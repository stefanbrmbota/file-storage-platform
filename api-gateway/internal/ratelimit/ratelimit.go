// Package ratelimit ogranicava broj zahteva ka gateway-u (US-63, T-63.1),
// kao zastitu od DoS napada i pogadjanja lozinki/OTP kodova.
//
// Algoritam je "kofa sa zetonima" (token bucket): svaki kljuc (korisnik ili
// IP adresa) ima kofu sa najvise N zetona. Svaki zahtev uzima jedan zeton, a
// kofa se ravnomerno puni brzinom N zetona u minuti. Prazna kofa → 429 Too
// Many Requests sa Retry-After (za koliko sekundi ce biti novog zetona).
//
// Tako korisnik moze kratko da posalje vise zahteva odjednom (npr. stranica
// ucita nekoliko stvari), ali ne moze dugo da salje vise od N u minuti.
//
// Stanje je u memoriji gateway-a. Sa vise instanci gateway-a svaka bi brojala
// za sebe; tada bi brojaci isli u zajednicko skladiste (npr. Redis).
package ratelimit

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/apierror"
	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/auth"
)

// sweepEvery je koliko cesto se iz memorije brisu kofe koje se vec napunile
// (korisnik odavno nije slao zahteve), da mapa ne bi rasla bez kraja.
const sweepEvery = time.Minute

type bucket struct {
	tokens float64
	last   time.Time
}

// Limiter broji zahteve po kljucu.
type Limiter struct {
	mu        sync.Mutex
	buckets   map[string]*bucket
	burst     float64 // najvise zetona u kofi
	perSecond float64 // brzina punjenja
	now       func() time.Time
	lastSweep time.Time
}

// New pravi Limiter koji dozvoljava perMinute zahteva u minuti po kljucu.
func New(perMinute int) *Limiter {
	return newWithClock(perMinute, time.Now)
}

func newWithClock(perMinute int, now func() time.Time) *Limiter {
	if perMinute < 1 {
		perMinute = 1
	}
	return &Limiter{
		buckets:   make(map[string]*bucket),
		burst:     float64(perMinute),
		perSecond: float64(perMinute) / 60,
		now:       now,
		lastSweep: now(),
	}
}

// Allow uzima zeton za kljuc. Ako ga nema, vraca false i vreme do sledeceg zetona.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	// Dopuni kofu za vreme koje je proslo od poslednjeg zahteva.
	b.tokens = math.Min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.perSecond)
	b.last = now

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := time.Duration((1 - b.tokens) / l.perSecond * float64(time.Second))
	return false, wait
}

// sweep brise kofe koje su se u medjuvremenu potpuno napunile; poziva se pod mu.
func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < sweepEvery {
		return
	}
	l.lastSweep = now
	full := time.Duration(l.burst / l.perSecond * float64(time.Second))
	for k, b := range l.buckets {
		if now.Sub(b.last) >= full {
			delete(l.buckets, k)
		}
	}
}

// KeyFunc odredjuje ciji je zahtev.
type KeyFunc func(*http.Request) string

// ByIP je kljuc za javne rute (prijava, registracija...): IP adresa klijenta.
// Gateway je jedina ulazna tacka, pa se koristi adresa TCP veze, a ne
// X-Forwarded-For, koji klijent moze sam da izmisli.
func ByIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return "ip:" + host
}

// ByUser je kljuc za zasticene rute: ID korisnika iz proverenog tokena, pa
// korisnik ne moze da zaobidje limit menjanjem adrese. Bez identiteta
// (ne bi trebalo da se desi iza auth-a) koristi se IP adresa.
func ByUser(r *http.Request) string {
	if id, ok := auth.FromContext(r.Context()); ok && id.UserID != "" {
		return "user:" + id.UserID
	}
	return ByIP(r)
}

// Middleware propusta zahtev ako kljuc ima zeton, a inace vraca 429 sa
// Retry-After (u celim sekundama, najmanje 1).
func (l *Limiter) Middleware(key KeyFunc, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, wait := l.Allow(key(r))
		if !ok {
			secs := int(math.Ceil(wait.Seconds()))
			if secs < 1 {
				secs = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(secs))
			apierror.WriteStatus(w, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
