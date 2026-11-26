package routes

// Ogranicenje broja zahteva kroz rute (US-63, T-63.1).

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/auth"
	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/ratelimit"
)

// identityGuard je lazni guard koji "prijavljuje" korisnika iz X-Test-User.
type identityGuard struct{}

func withUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := auth.Identity{UserID: r.Header.Get("X-Test-User"), Role: auth.RoleAdmin}
		next.ServeHTTP(w, r.WithContext(auth.ContextWithIdentity(r.Context(), id)))
	})
}

func (identityGuard) Require(next http.Handler) http.Handler      { return withUser(next) }
func (identityGuard) RequireAdmin(next http.Handler) http.Handler { return withUser(next) }

func newLimitedMux(publicPerMin, userPerMin int) *http.ServeMux {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	public, user := ratelimit.New(publicPerMin), ratelimit.New(userPerMin)
	mux := http.NewServeMux()
	Register(mux, map[Service]http.Handler{Users: ok, Files: ok, Sharing: ok, Notifications: ok, Analytics: ok},
		identityGuard{}, Limits{
			Public: func(h http.Handler) http.Handler { return public.Middleware(ratelimit.ByIP, h) },
			User:   func(h http.Handler) http.Handler { return user.Middleware(ratelimit.ByUser, h) },
		})
	return mux
}

func send(mux http.Handler, method, path, ip, user string) int {
	req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	req.RemoteAddr = ip + ":40000"
	if user != "" {
		req.Header.Set("X-Test-User", user)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Code
}

// Pogadjanje lozinke: posle limita prijava sa iste adrese dobija 429, a druga adresa i dalje radi.
func TestPublicRoutesLimitedPerIP(t *testing.T) {
	mux := newLimitedMux(3, 100)
	for i := 1; i <= 3; i++ {
		if c := send(mux, "POST", "/api/auth/login", "10.0.0.1", ""); c != http.StatusOK {
			t.Fatalf("prijava %d: status %d", i, c)
		}
	}
	if c := send(mux, "POST", "/api/auth/login/verify", "10.0.0.1", ""); c != http.StatusTooManyRequests {
		t.Errorf("4. javni zahtev sa iste adrese: status %d, ocekivano 429", c)
	}
	if c := send(mux, "POST", "/api/auth/login", "10.0.0.2", ""); c != http.StatusOK {
		t.Errorf("druga adresa: status %d, ocekivano 200", c)
	}
}

// Zasticene rute broje po korisniku: ista adresa, razliciti korisnici imaju odvojene limite.
func TestProtectedRoutesLimitedPerUser(t *testing.T) {
	mux := newLimitedMux(100, 2)
	send(mux, "GET", "/api/users/me", "10.0.0.1", "ana")
	send(mux, "GET", "/api/folders/root/contents", "10.0.0.1", "ana")
	if c := send(mux, "GET", "/api/admin/users", "10.0.0.9", "ana"); c != http.StatusTooManyRequests {
		t.Errorf("ana preko limita (i sa druge adrese): status %d, ocekivano 429", c)
	}
	if c := send(mux, "GET", "/api/users/me", "10.0.0.1", "marko"); c != http.StatusOK {
		t.Errorf("marko sa iste adrese: status %d, ocekivano 200", c)
	}
}

// Bez Limits rute rade kao ranije (postojeci pozivi Register).
func TestNoLimitsByDefault(t *testing.T) {
	mux, _ := newMux(t)
	for i := 0; i < 50; i++ {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/auth/login", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("zahtev %d: status %d", i+1, rec.Code)
		}
	}
}
