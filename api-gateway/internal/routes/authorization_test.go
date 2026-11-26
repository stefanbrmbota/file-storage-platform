package routes

// Testovi autorizacije svakog zahteva (US-62, T-62.1) sa PRAVIM proveravacem
// tokena (auth.Authenticator) i proxy-jem, a laznim Users servisom za proveru
// opoziva. Prolaze kroz celu listu ruta, pa nova ruta automatski ulazi u proveru.

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/auth"
	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/proxy"
)

const testSecret = "test-secret-at-least-32-characters-long!"

// gotRequest je ono sto je servis video: identitet iz headera.
type gotRequest struct {
	path, userID, role string
}

// newGateway pravi gateway sa pravim auth-om i proxy-jem. Svi servisi su
// jedan lazni server koji belezi zahteve.
func newGateway(t *testing.T) (http.Handler, *[]gotRequest) {
	t.Helper()
	var got []gotRequest
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/internal/tokens/check" {
			_ = json.NewEncoder(w).Encode(map[string]bool{"revoked": false})
			return
		}
		got = append(got, gotRequest{r.URL.Path, r.Header.Get(proxy.HeaderUserID), r.Header.Get(proxy.HeaderUserRole)})
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	target, _ := url.Parse(backend.URL)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	p := proxy.New(target, time.Second, logger)
	mux := http.NewServeMux()
	Register(mux, map[Service]http.Handler{
		Users: p, Files: p, Sharing: p, Notifications: p, Analytics: p,
	}, auth.New(testSecret, target, time.Second, logger))
	return mux, &got
}

func token(t *testing.T, userID, role string) string {
	t.Helper()
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID, "role": role, "jti": "jti-" + userID,
		"exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// concretePath pretvara obrazac rute u stvarnu putanju.
func concretePath(pattern string) (method, path string) {
	method, path, _ = strings.Cut(pattern, " ")
	return method, strings.NewReplacer("{id}", "x", "{resourceId}", "r", "{granteeId}", "g").Replace(path)
}

// Bez tokena (ili sa pokvarenim tokenom) nijedna zasticena ruta ne stize do servisa.
func TestEveryProtectedRouteRejectsMissingOrBadToken(t *testing.T) {
	valid := token(t, "u1", auth.RoleUser)
	forged, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "u1", "role": auth.RoleAdmin, "jti": "j", "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte("neka-druga-tajna-koju-napadac-zna-123"))
	bad := map[string]string{
		"bez tokena":      "",
		"tudji potpis":    "Bearer " + forged,
		"izmenjen potpis": "Bearer " + valid[:len(valid)-4] + "AAAA",
		"nije JWT":        "Bearer abc",
		"pogresna sema":   "Basic " + valid,
	}
	for _, rt := range All {
		if rt.Access == Public {
			continue
		}
		for name, authz := range bad {
			t.Run(rt.Pattern+"/"+name, func(t *testing.T) {
				gw, got := newGateway(t)
				method, path := concretePath(rt.Pattern)
				req := httptest.NewRequest(method, path, nil)
				if authz != "" {
					req.Header.Set("Authorization", authz)
				}
				// Lazni identitet ne sme da pomogne.
				req.Header.Set(proxy.HeaderUserID, "admin-id")
				req.Header.Set(proxy.HeaderUserRole, auth.RoleAdmin)
				rec := httptest.NewRecorder()
				gw.ServeHTTP(rec, req)

				if rec.Code != http.StatusUnauthorized {
					t.Errorf("status = %d, ocekivano 401", rec.Code)
				}
				if len(*got) != 0 {
					t.Errorf("zahtev stigao do servisa: %+v", *got)
				}
			})
		}
	}
}

// Obican korisnik ne prolazi ni jednu admin rutu.
func TestUserTokenCannotReachAdminRoutes(t *testing.T) {
	for _, rt := range All {
		if rt.Access != Admin {
			continue
		}
		t.Run(rt.Pattern, func(t *testing.T) {
			gw, got := newGateway(t)
			method, path := concretePath(rt.Pattern)
			req := httptest.NewRequest(method, path, nil)
			req.Header.Set("Authorization", "Bearer "+token(t, "u1", auth.RoleUser))
			req.Header.Set(proxy.HeaderUserRole, auth.RoleAdmin)
			rec := httptest.NewRecorder()
			gw.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden || len(*got) != 0 {
				t.Errorf("status = %d, do servisa stiglo %d zahteva; ocekivano 403 i 0", rec.Code, len(*got))
			}
		})
	}
}

// Servis dobija identitet iz tokena, a ne onaj koji je klijent poslao.
func TestServiceGetsIdentityOnlyFromToken(t *testing.T) {
	gw, got := newGateway(t)
	req := httptest.NewRequest(http.MethodGet, "/api/folders/x/contents", nil)
	req.Header.Set("Authorization", "Bearer "+token(t, "ana", auth.RoleUser))
	req.Header.Set(proxy.HeaderUserID, "marko")
	req.Header.Set(proxy.HeaderUserRole, auth.RoleAdmin)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || len(*got) != 1 {
		t.Fatalf("status = %d, zahteva = %d", rec.Code, len(*got))
	}
	if g := (*got)[0]; g.userID != "ana" || g.role != auth.RoleUser {
		t.Errorf("servis dobio korisnika %q sa ulogom %q, ocekivano ana/USER", g.userID, g.role)
	}
}

// Na javnoj ruti (prijava) lazni identitet se brise pre servisa.
func TestPublicRouteDropsSpoofedIdentity(t *testing.T) {
	gw, got := newGateway(t)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{}`))
	req.Header.Set(proxy.HeaderUserID, "admin-id")
	req.Header.Set(proxy.HeaderUserRole, auth.RoleAdmin)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if len(*got) != 1 {
		t.Fatalf("zahteva = %d", len(*got))
	}
	if g := (*got)[0]; g.userID != "" || g.role != "" {
		t.Errorf("servis dobio lazni identitet %q/%q", g.userID, g.role)
	}
}

// Lista javnih ruta je tacno ona iz design 5.2: nova javna ruta mora svesno da se doda ovde.
func TestOnlyKnownRoutesArePublic(t *testing.T) {
	allowed := map[string]bool{
		"POST /api/auth/register": true, "POST /api/auth/confirm": true,
		"POST /api/auth/login": true, "POST /api/auth/login/verify": true,
		"POST /api/auth/password-reset/request": true, "POST /api/auth/password-reset/confirm": true,
		"POST /api/auth/magic-link/request": true, "POST /api/auth/magic-link/login": true,
	}
	public := 0
	for _, rt := range All {
		if rt.Access == Public {
			public++
			if !allowed[rt.Pattern] {
				t.Errorf("ruta %s je javna, a nije na listi dozvoljenih javnih ruta", rt.Pattern)
			}
		}
	}
	if public != len(allowed) {
		t.Errorf("javnih ruta je %d, ocekivano %d", public, len(allowed))
	}
}
