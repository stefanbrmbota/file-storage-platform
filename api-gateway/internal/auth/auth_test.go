package auth

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const secret = "test-secret-test-secret-test-secret-123"

// fakeUsers je lazni Users servis za POST /internal/tokens/check.
type fakeUsers struct {
	revoked map[string]bool
	status  int // ako nije 0, vraca se umesto 200
	calls   atomic.Int32
	gotPath string
	gotJTI  string
}

func (f *fakeUsers) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.calls.Add(1)
	f.gotPath = r.URL.Path
	var req struct {
		JTI string `json:"jti"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	f.gotJTI = req.JTI
	if f.status != 0 {
		w.WriteHeader(f.status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"revoked": f.revoked[req.JTI]})
}

func newAuth(t *testing.T, users http.Handler) *Authenticator {
	t.Helper()
	srv := httptest.NewServer(users)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	return New(secret, u, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

type tokenOpts struct {
	sub, role, jti string
	exp            time.Duration
	noExp          bool
	method         jwt.SigningMethod
	key            any
}

func makeToken(t *testing.T, o tokenOpts) string {
	t.Helper()
	if o.method == nil {
		o.method = jwt.SigningMethodHS256
	}
	if o.key == nil {
		o.key = []byte(secret)
	}
	if o.exp == 0 {
		o.exp = 15 * time.Minute
	}
	rc := jwt.RegisteredClaims{Subject: o.sub, ID: o.jti, IssuedAt: jwt.NewNumericDate(time.Now())}
	if !o.noExp {
		rc.ExpiresAt = jwt.NewNumericDate(time.Now().Add(o.exp))
	}
	s, err := jwt.NewWithClaims(o.method, claims{Role: o.role, RegisteredClaims: rc}).SignedString(o.key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func validToken(t *testing.T, role string) string {
	return makeToken(t, tokenOpts{sub: "u1", role: role, jti: "jti-1"})
}

// serve salje zahtev kroz handler i vraca odgovor i identitet koji je stigao dalje.
func serve(h func(http.Handler) http.Handler, authz string) (*httptest.ResponseRecorder, *Identity) {
	var got *Identity
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, ok := FromContext(r.Context()); ok {
			got = &id
		}
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/folders/x/contents", nil)
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	rec := httptest.NewRecorder()
	h(next).ServeHTTP(rec, req)
	return rec, got
}

func errMsg(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var b map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&b); err != nil {
		t.Fatalf("telo nije JSON: %v", err)
	}
	return b["error"]
}

func TestValidTokenPasses(t *testing.T) {
	users := &fakeUsers{}
	a := newAuth(t, users)
	rec, id := serve(a.Require, "Bearer "+validToken(t, RoleUser))

	if rec.Code != http.StatusOK || id == nil {
		t.Fatalf("status = %d, identitet = %v", rec.Code, id)
	}
	if id.UserID != "u1" || id.Role != RoleUser {
		t.Errorf("identitet = %+v", *id)
	}
	if users.gotPath != tokenCheckPath || users.gotJTI != "jti-1" {
		t.Errorf("Users dobio %s jti=%q", users.gotPath, users.gotJTI)
	}
}

func TestBearerIsCaseInsensitive(t *testing.T) {
	a := newAuth(t, &fakeUsers{})
	if rec, _ := serve(a.Require, "bearer "+validToken(t, RoleUser)); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestInvalidTokensReturn401(t *testing.T) {
	cases := map[string]func(t *testing.T) string{
		"bez headera":     func(t *testing.T) string { return "" },
		"bez Bearer seme": func(t *testing.T) string { return validToken(t, RoleUser) },
		"Basic sema":      func(t *testing.T) string { return "Basic dXNlcjpwYXNz" },
		"prazan Bearer":   func(t *testing.T) string { return "Bearer " },
		"smece":           func(t *testing.T) string { return "Bearer abc.def.ghi" },
		"istekao": func(t *testing.T) string {
			return "Bearer " + makeToken(t, tokenOpts{sub: "u1", role: RoleUser, jti: "j", exp: -time.Minute})
		},
		"bez exp": func(t *testing.T) string {
			return "Bearer " + makeToken(t, tokenOpts{sub: "u1", role: RoleUser, jti: "j", noExp: true})
		},
		"pogresna tajna": func(t *testing.T) string {
			return "Bearer " + makeToken(t, tokenOpts{sub: "u1", role: RoleUser, jti: "j", key: []byte("druga-tajna-druga-tajna-druga-tajna")})
		},
		"HS512": func(t *testing.T) string {
			return "Bearer " + makeToken(t, tokenOpts{sub: "u1", role: RoleUser, jti: "j", method: jwt.SigningMethodHS512})
		},
		"alg none": func(t *testing.T) string {
			return "Bearer " + makeToken(t, tokenOpts{sub: "u1", role: RoleUser, jti: "j", method: jwt.SigningMethodNone, key: jwt.UnsafeAllowNoneSignatureType})
		},
		"bez sub": func(t *testing.T) string { return "Bearer " + makeToken(t, tokenOpts{role: RoleUser, jti: "j"}) },
		"bez jti": func(t *testing.T) string { return "Bearer " + makeToken(t, tokenOpts{sub: "u1", role: RoleUser}) },
		"predugacak jti": func(t *testing.T) string {
			return "Bearer " + makeToken(t, tokenOpts{sub: "u1", role: RoleUser, jti: strings.Repeat("a", maxJTILen+1)})
		},
		"nepoznata uloga": func(t *testing.T) string {
			return "Bearer " + makeToken(t, tokenOpts{sub: "u1", role: "SUPERUSER", jti: "j"})
		},
		"izmenjen payload": func(t *testing.T) string { return "Bearer " + tamper(validToken(t, RoleUser)) },
	}
	for name, mk := range cases {
		t.Run(name, func(t *testing.T) {
			users := &fakeUsers{}
			a := newAuth(t, users)
			rec, id := serve(a.Require, mk(t))

			if rec.Code != http.StatusUnauthorized || id != nil {
				t.Fatalf("status = %d, stiglo dalje = %v", rec.Code, id != nil)
			}
			if msg := errMsg(t, rec); msg != "unauthorized" {
				t.Errorf("poruka = %q", msg)
			}
			if users.calls.Load() != 0 {
				t.Error("neispravan token ne treba da ide do Users servisa")
			}
		})
	}
}

// tamper menja ulogu u payload-u bez novog potpisa.
func tamper(tok string) string {
	parts := strings.Split(tok, ".")
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	payload = []byte(strings.Replace(string(payload), `"USER"`, `"ADMIN"`, 1))
	parts[1] = base64.RawURLEncoding.EncodeToString(payload)
	return strings.Join(parts, ".")
}

func TestRevokedTokenReturns401(t *testing.T) {
	a := newAuth(t, &fakeUsers{revoked: map[string]bool{"jti-1": true}})
	rec, id := serve(a.Require, "Bearer "+validToken(t, RoleUser))

	if rec.Code != http.StatusUnauthorized || id != nil {
		t.Fatalf("status = %d, stiglo dalje = %v", rec.Code, id != nil)
	}
}

func TestUsersFailureReturns503(t *testing.T) {
	cases := map[string]http.Handler{
		"500": &fakeUsers{status: http.StatusInternalServerError},
		"503": &fakeUsers{status: http.StatusServiceUnavailable},
		"neispravan odgovor": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"nesto":"drugo"}`))
		}),
	}
	for name, users := range cases {
		t.Run(name, func(t *testing.T) {
			a := newAuth(t, users)
			rec, id := serve(a.Require, "Bearer "+validToken(t, RoleUser))
			if rec.Code != http.StatusServiceUnavailable || id != nil {
				t.Fatalf("status = %d, stiglo dalje = %v", rec.Code, id != nil)
			}
			if msg := errMsg(t, rec); msg != "service unavailable" {
				t.Errorf("poruka = %q", msg)
			}
		})
	}
}

func TestUsersDownReturns503(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	u, _ := url.Parse(srv.URL)
	srv.Close()
	a := New(secret, u, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if rec, _ := serve(a.Require, "Bearer "+validToken(t, RoleUser)); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, ocekivano 503", rec.Code)
	}
}

func TestUsersSlowReturns503(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)
	u, _ := url.Parse(srv.URL)
	a := New(secret, u, 50*time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if rec, _ := serve(a.Require, "Bearer "+validToken(t, RoleUser)); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, ocekivano 503", rec.Code)
	}
}

func TestAdminRoute(t *testing.T) {
	a := newAuth(t, &fakeUsers{})

	rec, id := serve(a.RequireAdmin, "Bearer "+validToken(t, RoleUser))
	if rec.Code != http.StatusForbidden || id != nil {
		t.Fatalf("USER na admin ruti: status = %d", rec.Code)
	}
	if msg := errMsg(t, rec); msg != "forbidden" {
		t.Errorf("poruka = %q", msg)
	}

	rec, id = serve(a.RequireAdmin, "Bearer "+validToken(t, RoleAdmin))
	if rec.Code != http.StatusOK || id == nil || id.Role != RoleAdmin {
		t.Fatalf("ADMIN na admin ruti: status = %d, identitet = %v", rec.Code, id)
	}
}
