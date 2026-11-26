package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/auth"
)

// clock je lazni sat koji test pomera rucno.
type clock struct{ t time.Time }

func (c *clock) now() time.Time      { return c.t }
func (c *clock) add(d time.Duration) { c.t = c.t.Add(d) }
func newClock() *clock               { return &clock{t: time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)} }
func okHandler() http.Handler        { return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}) }

func TestAllowsUpToLimitThenBlocks(t *testing.T) {
	c := newClock()
	l := newWithClock(5, c.now)
	for i := 1; i <= 5; i++ {
		if ok, _ := l.Allow("ana"); !ok {
			t.Fatalf("zahtev %d odbijen, a limit je 5", i)
		}
	}
	ok, wait := l.Allow("ana")
	if ok {
		t.Fatal("6. zahtev je prosao")
	}
	// 5 u minuti = jedan zeton na 12 s.
	if wait != 12*time.Second {
		t.Errorf("Retry-After = %v, ocekivano 12s", wait)
	}
}

func TestRefillsOverTime(t *testing.T) {
	c := newClock()
	l := newWithClock(60, c.now) // jedan zeton u sekundi
	for i := 0; i < 60; i++ {
		l.Allow("ana")
	}
	if ok, _ := l.Allow("ana"); ok {
		t.Fatal("prazna kofa je propustila zahtev")
	}
	c.add(time.Second)
	if ok, _ := l.Allow("ana"); !ok {
		t.Fatal("posle 1 s treba da postoji novi zeton")
	}
	c.add(time.Minute)
	for i := 0; i < 60; i++ {
		if ok, _ := l.Allow("ana"); !ok {
			t.Fatalf("posle minuta kofa treba da je puna; zahtev %d odbijen", i+1)
		}
	}
}

func TestKeysAreIndependent(t *testing.T) {
	c := newClock()
	l := newWithClock(1, c.now)
	l.Allow("ana")
	if ok, _ := l.Allow("ana"); ok {
		t.Fatal("ana je presla limit")
	}
	if ok, _ := l.Allow("marko"); !ok {
		t.Fatal("limit jednog korisnika ne sme da blokira drugog")
	}
}

func TestSweepRemovesIdleBuckets(t *testing.T) {
	c := newClock()
	l := newWithClock(10, c.now)
	l.Allow("ana")
	c.add(2 * time.Minute)
	l.Allow("marko")
	if _, ok := l.buckets["ana"]; ok {
		t.Error("napunjena kofa nije obrisana iz memorije")
	}
}

func TestMiddlewareReturns429WithRetryAfter(t *testing.T) {
	c := newClock()
	l := newWithClock(2, c.now)
	h := l.Middleware(ByIP, okHandler())

	codes := []int{}
	var last *httptest.ResponseRecorder
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
		req.RemoteAddr = "10.0.0.7:5555"
		last = httptest.NewRecorder()
		h.ServeHTTP(last, req)
		codes = append(codes, last.Code)
	}
	if codes[0] != 200 || codes[1] != 200 || codes[2] != http.StatusTooManyRequests {
		t.Fatalf("statusi = %v, ocekivano [200 200 429]", codes)
	}
	if got := last.Header().Get("Retry-After"); got != "30" {
		t.Errorf("Retry-After = %q, ocekivano 30", got)
	}
	if body := last.Body.String(); body != "{\"error\":\"too many requests\"}\n" {
		t.Errorf("telo = %q", body)
	}
}

func TestByIPIgnoresForwardedHeaderAndPort(t *testing.T) {
	a := httptest.NewRequest(http.MethodGet, "/", nil)
	a.RemoteAddr = "10.0.0.7:1111"
	a.Header.Set("X-Forwarded-For", "1.2.3.4")
	b := httptest.NewRequest(http.MethodGet, "/", nil)
	b.RemoteAddr = "10.0.0.7:2222"
	if ByIP(a) != ByIP(b) || ByIP(a) != "ip:10.0.0.7" {
		t.Errorf("ByIP = %q i %q, ocekivano ip:10.0.0.7", ByIP(a), ByIP(b))
	}
}

func TestByUserUsesTokenIdentity(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.7:1111"
	if ByUser(r) != "ip:10.0.0.7" {
		t.Errorf("bez identiteta: %q", ByUser(r))
	}
	r = r.WithContext(auth.ContextWithIdentity(r.Context(), auth.Identity{UserID: "ana", Role: auth.RoleUser}))
	if ByUser(r) != "user:ana" {
		t.Errorf("sa identitetom: %q", ByUser(r))
	}
}
