package routes

// Izolacija obavestenja kroz gateway (US-08, T-08.1): pravi gateway (rute,
// provera JWT-a, proxy) ispred laznog Notifications servisa. Proverava da
// korisnik A nikad ne dobija obavestenja korisnika B, ni kad podmetne
// X-User-ID, userId u upitu ili ID u putanji.

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/auth"
	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/proxy"
)

const isolationSecret = "test-secret-test-secret-test-secret-123"

// fakeNotifications vraca obavestenja korisnika iz X-User-ID, kao pravi servis
// (koji ID cita samo iz tog headera), i belezi sta je dobio.
type fakeNotifications struct {
	byUser  map[string][]string
	calls   int
	gotUser string
	gotRole string
}

func (f *fakeNotifications) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.calls++
	f.gotUser = r.Header.Get(proxy.HeaderUserID)
	f.gotRole = r.Header.Get(proxy.HeaderUserRole)
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/notifications":
		_ = json.NewEncoder(w).Encode(map[string]any{"items": f.byUser[f.gotUser]})
	case "/notifications/unread-count":
		_ = json.NewEncoder(w).Encode(map[string]int{"count": len(f.byUser[f.gotUser])})
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}
}

func signedToken(t *testing.T, secret, sub, jti string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": sub, "role": "USER", "jti": jti, "exp": time.Now().Add(15 * time.Minute).Unix(),
	})
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// newIsolationGateway sastavlja gateway kao main.go: rute + auth + proxy.
func newIsolationGateway(t *testing.T, notif http.Handler) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Lazni Users: nijedan token nije opozvan.
	users := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"revoked":false}`))
	}))
	t.Cleanup(users.Close)
	notifSrv := httptest.NewServer(notif)
	t.Cleanup(notifSrv.Close)

	usersURL, _ := url.Parse(users.URL)
	notifURL, _ := url.Parse(notifSrv.URL)
	other := http.NotFoundHandler()

	mux := http.NewServeMux()
	Register(mux, map[Service]http.Handler{
		Users: other, Files: other, Sharing: other, Analytics: other,
		Notifications: proxy.New(notifURL, time.Second, logger),
	}, auth.New(isolationSecret, usersURL, time.Second, logger))
	return mux
}

func get(h http.Handler, target, token string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestNotificationsIsolationThroughGateway(t *testing.T) {
	notif := &fakeNotifications{byUser: map[string][]string{
		"user-a": {"A: Marko je podelio folder"},
		"user-b": {"B: tajna 1", "B: tajna 2"},
	}}
	gw := newIsolationGateway(t, notif)
	tokenA := signedToken(t, isolationSecret, "user-a", "jti-a")

	items := func(rec *httptest.ResponseRecorder) []string {
		t.Helper()
		var body struct{ Items []string }
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("telo nije JSON: %v", err)
		}
		return body.Items
	}

	t.Run("A dobija samo svoja", func(t *testing.T) {
		rec := get(gw, "/api/notifications", tokenA, nil)
		got := items(rec)
		if rec.Code != http.StatusOK || len(got) != 1 || got[0] != "A: Marko je podelio folder" {
			t.Fatalf("status = %d, items = %v", rec.Code, got)
		}
		if notif.gotUser != "user-a" || notif.gotRole != "USER" {
			t.Errorf("servis dobio X-User-ID=%q X-User-Role=%q", notif.gotUser, notif.gotRole)
		}
	})

	t.Run("podmetnut X-User-ID ne menja identitet", func(t *testing.T) {
		rec := get(gw, "/api/notifications", tokenA, map[string]string{
			proxy.HeaderUserID: "user-b", proxy.HeaderUserRole: "ADMIN",
		})
		if got := items(rec); notif.gotUser != "user-a" || notif.gotRole != "USER" || len(got) != 1 {
			t.Fatalf("servis dobio %q/%q, items = %v", notif.gotUser, notif.gotRole, got)
		}
	})

	t.Run("userId u upitu ne menja identitet", func(t *testing.T) {
		rec := get(gw, "/api/notifications?userId=user-b&user_id=user-b", tokenA, nil)
		if got := items(rec); notif.gotUser != "user-a" || len(got) != 1 {
			t.Fatalf("servis dobio %q, items = %v", notif.gotUser, got)
		}
	})

	t.Run("broj neprocitanih je za A", func(t *testing.T) {
		rec := get(gw, "/api/notifications/unread-count", tokenA, map[string]string{proxy.HeaderUserID: "user-b"})
		var body struct{ Count int }
		_ = json.NewDecoder(rec.Body).Decode(&body)
		if rec.Code != http.StatusOK || body.Count != 1 || notif.gotUser != "user-a" {
			t.Fatalf("status = %d, count = %d, user = %q", rec.Code, body.Count, notif.gotUser)
		}
	})

	t.Run("ID drugog korisnika u putanji vraca 404", func(t *testing.T) {
		before := notif.calls
		for _, target := range []string{"/api/notifications/user-b", "/api/users/user-b/notifications"} {
			if rec := get(gw, target, tokenA, nil); rec.Code != http.StatusNotFound {
				t.Errorf("%s: status = %d, ocekivano 404", target, rec.Code)
			}
		}
		if notif.calls != before {
			t.Error("zahtev sa tudjim ID-jem u putanji stigao do servisa")
		}
	})

	t.Run("bez tokena 401, i sa podmetnutim X-User-ID", func(t *testing.T) {
		before := notif.calls
		rec := get(gw, "/api/notifications", "", map[string]string{proxy.HeaderUserID: "user-b"})
		if rec.Code != http.StatusUnauthorized || notif.calls != before {
			t.Fatalf("status = %d, servis pozvan = %v", rec.Code, notif.calls != before)
		}
	})

	t.Run("token za B potpisan tudjom tajnom se odbija", func(t *testing.T) {
		before := notif.calls
		forged := signedToken(t, "druga-tajna-druga-tajna-druga-tajna-1", "user-b", "jti-x")
		rec := get(gw, "/api/notifications", forged, nil)
		if rec.Code != http.StatusUnauthorized || notif.calls != before {
			t.Fatalf("status = %d, servis pozvan = %v", rec.Code, notif.calls != before)
		}
	})
}
