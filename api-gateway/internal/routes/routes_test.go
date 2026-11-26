package routes

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/proxy"
)

// hit belezi koji servis je dobio zahtev i sa kojom putanjom.
type hit struct {
	service Service
	method  string
	path    string
	query   string
}

// headerGuard je lazni guard: propusta zahtev i belezi nivo pristupa u
// headeru X-Test-Access, da test vidi koja provera je primenjena.
type headerGuard struct{}

func mark(level string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test-Access", level)
		next.ServeHTTP(w, r)
	})
}

func (headerGuard) Require(next http.Handler) http.Handler      { return mark("user", next) }
func (headerGuard) RequireAdmin(next http.Handler) http.Handler { return mark("admin", next) }

// newMux registruje rute sa laznim servisima koji samo belezi zahtev.
func newMux(t *testing.T) (*http.ServeMux, *[]hit) {
	t.Helper()
	var hits []hit
	upstreams := map[Service]http.Handler{}
	for _, s := range []Service{Users, Files, Sharing, Notifications, Analytics} {
		upstreams[s] = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits = append(hits, hit{s, r.Method, r.URL.Path, r.URL.RawQuery})
			w.WriteHeader(http.StatusOK)
		})
	}
	mux := http.NewServeMux()
	Register(mux, upstreams, headerGuard{})
	return mux, &hits
}

func TestRoutesReachServiceWithoutPrefix(t *testing.T) {
	cases := []struct {
		method, path string
		service      Service
		wantPath     string
	}{
		{"POST", "/api/auth/register", Users, "/register"},
		{"POST", "/api/auth/confirm", Users, "/confirm"},
		{"POST", "/api/auth/login", Users, "/login"},
		{"POST", "/api/auth/login/verify", Users, "/login/verify"},
		{"POST", "/api/auth/password-reset/request", Users, "/password-reset/request"},
		{"POST", "/api/auth/password-reset/confirm", Users, "/password-reset/confirm"},
		{"POST", "/api/auth/magic-link/request", Users, "/magic-link/request"},
		{"POST", "/api/auth/magic-link/login", Users, "/magic-link/login"},
		{"POST", "/api/auth/logout", Users, "/logout"},
		{"GET", "/api/users/me", Users, "/users/me"},
		{"POST", "/api/users/me/password", Users, "/users/me/password"},
		{"POST", "/api/users/me/quota-requests", Users, "/users/me/quota-requests"},
		{"GET", "/api/users/me/quota-requests/latest", Users, "/users/me/quota-requests/latest"},
		{"GET", "/api/admin/users", Users, "/admin/users"},
		{"GET", "/api/admin/quota-requests", Users, "/admin/quota-requests"},
		{"POST", "/api/folders", Files, "/folders"},
		{"GET", "/api/folders/f1/contents", Files, "/folders/f1/contents"},
		{"PATCH", "/api/folders/f1", Files, "/folders/f1"},
		{"DELETE", "/api/folders/f1", Files, "/folders/f1"},
		{"POST", "/api/folders/f1/files", Files, "/folders/f1/files"},
		{"GET", "/api/files/d1/content", Files, "/files/d1/content"},
		{"PUT", "/api/files/d1/content", Files, "/files/d1/content"},
		{"PATCH", "/api/files/d1", Files, "/files/d1"},
		{"DELETE", "/api/files/d1", Files, "/files/d1"},
		{"GET", "/api/search", Files, "/search"},
		{"POST", "/api/shares", Sharing, "/shares"},
		{"DELETE", "/api/shares/r1/u2", Sharing, "/shares/r1/u2"},
		{"GET", "/api/shares/with-me", Sharing, "/shares/with-me"},
		{"GET", "/api/shares/r1", Sharing, "/shares/r1"},
		{"GET", "/api/notifications", Notifications, "/notifications"},
		{"GET", "/api/notifications/unread-count", Notifications, "/notifications/unread-count"},
		{"POST", "/api/notifications/n1/read", Notifications, "/notifications/n1/read"},
		{"POST", "/api/notifications/read-all", Notifications, "/notifications/read-all"},
		{"GET", "/api/analytics/me", Analytics, "/analytics/me"},
		{"GET", "/api/analytics/me/events", Analytics, "/analytics/me/events"},
	}
	if len(cases) != len(All) {
		t.Fatalf("test pokriva %d ruta, a lista ima %d", len(cases), len(All))
	}
	for _, c := range cases {
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			mux, hits := newMux(t)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, nil))

			if rec.Code != http.StatusOK || len(*hits) != 1 {
				t.Fatalf("status = %d, pogodaka = %d", rec.Code, len(*hits))
			}
			got := (*hits)[0]
			if got.service != c.service || got.method != c.method || got.path != c.wantPath {
				t.Errorf("stiglo %s %s %s, ocekivano %s %s %s",
					got.service, got.method, got.path, c.service, c.method, c.wantPath)
			}
		})
	}
}

func TestAccessLevels(t *testing.T) {
	public := map[string]bool{
		"POST /api/auth/register": true, "POST /api/auth/confirm": true,
		"POST /api/auth/login": true, "POST /api/auth/login/verify": true,
		"POST /api/auth/password-reset/request": true, "POST /api/auth/password-reset/confirm": true,
		"POST /api/auth/magic-link/request": true, "POST /api/auth/magic-link/login": true,
	}
	admin := map[string]bool{"GET /api/admin/users": true, "GET /api/admin/quota-requests": true}

	for _, rt := range All {
		t.Run(rt.Pattern, func(t *testing.T) {
			want := "user"
			switch {
			case public[rt.Pattern]:
				want = ""
			case admin[rt.Pattern]:
				want = "admin"
			}
			method, path, _ := strings.Cut(rt.Pattern, " ")
			path = strings.NewReplacer("{id}", "x", "{resourceId}", "r", "{granteeId}", "g").Replace(path)

			mux, _ := newMux(t)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
			if got := rec.Header().Get("X-Test-Access"); got != want {
				t.Errorf("provera = %q, ocekivano %q", got, want)
			}
		})
	}
}

func TestRegisterPanicsWithoutGuard(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("ocekivan panic bez guard-a")
		}
	}()
	h := http.NotFoundHandler()
	Register(http.NewServeMux(), map[Service]http.Handler{
		Users: h, Files: h, Sharing: h, Notifications: h, Analytics: h,
	}, nil)
}

func TestQueryIsForwarded(t *testing.T) {
	mux, hits := newMux(t)
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/search?q=izvestaj+2026", nil))
	if len(*hits) != 1 || (*hits)[0].query != "q=izvestaj+2026" {
		t.Fatalf("pogoci = %+v", *hits)
	}
}

func TestUnknownAndInternalRoutesReturn404(t *testing.T) {
	cases := []struct{ method, path string }{
		{"GET", "/"},
		{"GET", "/api"},
		{"GET", "/api/unknown"},
		{"POST", "/internal/tokens/check"},
		{"POST", "/api/internal/tokens/check"},
		{"POST", "/api/internal/folders/root"},
		{"POST", "/api/auth/internal/tokens/check"},
		{"GET", "/folders/f1/contents"}, // bez /api
		{"GET", "/api/auth/register"},   // pogresna metoda
		{"DELETE", "/api/folders"},      // pogresna metoda
	}
	for _, c := range cases {
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			mux, hits := newMux(t)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, nil))

			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, ocekivano 404", rec.Code)
			}
			if len(*hits) != 0 {
				t.Errorf("zahtev stigao do servisa: %+v", *hits)
			}
			if body := rec.Body.String(); body != "{\"error\":\"not found\"}\n" {
				t.Errorf("telo = %q", body)
			}
		})
	}
}

func TestPathTraversalDoesNotReachInternal(t *testing.T) {
	mux, hits := newMux(t)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/folders/../internal/folders/root", nil))
	for _, h := range *hits {
		if strings.HasPrefix(h.path, "/internal") {
			t.Fatalf("zahtev stigao do interne rute: %+v", h)
		}
	}
}

func TestRegisterPanicsWithoutUpstream(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("ocekivan panic kad servis nema handler")
		}
	}()
	Register(http.NewServeMux(), map[Service]http.Handler{Users: http.NotFoundHandler()}, headerGuard{})
}

// TestThroughProxy proverava ceo put: ruta → proxy → servis.
func TestThroughProxy(t *testing.T) {
	var gotPath, gotQuery string
	users := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.WriteHeader(http.StatusAccepted)
	}))
	defer users.Close()

	target, _ := url.Parse(users.URL)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	p := proxy.New(target, time.Second, logger)
	mux := http.NewServeMux()
	Register(mux, map[Service]http.Handler{
		Users: p, Files: p, Sharing: p, Notifications: p, Analytics: p,
	}, headerGuard{})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/auth/login/verify?x=1", strings.NewReader(`{}`)))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, ocekivano 202", rec.Code)
	}
	if gotPath != "/login/verify" || gotQuery != "x=1" {
		t.Errorf("servis dobio %s?%s", gotPath, gotQuery)
	}
}
