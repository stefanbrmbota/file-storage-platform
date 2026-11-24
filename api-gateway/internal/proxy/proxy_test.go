package proxy

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/auth"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func errorBody(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, ocekivano application/json", ct)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("telo nije JSON: %v", err)
	}
	return body["error"]
}

func TestForwardsPathQueryAndBody(t *testing.T) {
	var gotMethod, gotPath, gotQuery, gotBody, gotHost string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotQuery, gotHost = r.Method, r.URL.Path, r.URL.RawQuery, r.Host
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	target := mustURL(t, upstream.URL)
	h := New(target, time.Second, discardLogger())
	req := httptest.NewRequest(http.MethodPost, "/api/files/folders?parent=abc", strings.NewReader(`{"name":"x"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, ocekivano 201", rec.Code)
	}
	if rec.Body.String() != `{"ok":true}` {
		t.Errorf("telo = %q", rec.Body.String())
	}
	if gotMethod != http.MethodPost || gotPath != "/api/files/folders" || gotQuery != "parent=abc" {
		t.Errorf("servis dobio %s %s?%s", gotMethod, gotPath, gotQuery)
	}
	if gotBody != `{"name":"x"}` {
		t.Errorf("servis dobio telo %q", gotBody)
	}
	if gotHost != target.Host {
		t.Errorf("Host = %q, ocekivano %q", gotHost, target.Host)
	}
}

func TestStripsClientIdentityHeaders(t *testing.T) {
	var gotID, gotRole string
	var hasID, hasRole bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID, gotRole = r.Header.Get(HeaderUserID), r.Header.Get(HeaderUserRole)
		_, hasID = r.Header[http.CanonicalHeaderKey(HeaderUserID)]
		_, hasRole = r.Header[http.CanonicalHeaderKey(HeaderUserRole)]
	}))
	defer upstream.Close()

	h := New(mustURL(t, upstream.URL), time.Second, discardLogger())
	req := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
	req.Header.Set(HeaderUserID, "tudji-id")
	req.Header.Set(HeaderUserRole, "ADMIN")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if hasID || hasRole {
		t.Errorf("podmetnuti headeri stigli do servisa: %s=%q, %s=%q", HeaderUserID, gotID, HeaderUserRole, gotRole)
	}
}

func TestUnavailableServiceReturns502(t *testing.T) {
	// Server se odmah zatvara, pa na adresi niko ne slusa.
	upstream := httptest.NewServer(http.NotFoundHandler())
	target := mustURL(t, upstream.URL)
	upstream.Close()

	h := New(target, time.Second, discardLogger())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/users/me", nil))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, ocekivano 502", rec.Code)
	}
	if msg := errorBody(t, rec); msg != "service unavailable" {
		t.Errorf("poruka = %q", msg)
	}
}

func TestSlowServiceReturns504(t *testing.T) {
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer upstream.Close()
	defer close(release)

	h := New(mustURL(t, upstream.URL), 50*time.Millisecond, discardLogger())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/files/x", nil))

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, ocekivano 504", rec.Code)
	}
	if msg := errorBody(t, rec); msg != "service timeout" {
		t.Errorf("poruka = %q", msg)
	}
}

func TestUpstreamErrorStatusIsPassedThrough(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"folder not found"}`))
	}))
	defer upstream.Close()

	h := New(mustURL(t, upstream.URL), time.Second, discardLogger())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/files/x", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, ocekivano 404", rec.Code)
	}
	if msg := errorBody(t, rec); msg != "folder not found" {
		t.Errorf("poruka = %q", msg)
	}
}

func TestPlainTextErrorBecomesJSON(t *testing.T) {
	// net/http za nepoznatu rutu vraca "404 page not found" kao tekst.
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()

	h := New(mustURL(t, upstream.URL), time.Second, discardLogger())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/notifications", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, ocekivano 404", rec.Code)
	}
	if msg := errorBody(t, rec); msg != "not found" {
		t.Errorf("poruka = %q", msg)
	}
}

func TestServerErrorDetailsAreHidden(t *testing.T) {
	for _, ct := range []string{"application/json", "text/plain"} {
		t.Run(ct, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", ct)
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"mongo: connection refused at 10.0.0.5"}`))
			}))
			defer upstream.Close()

			h := New(mustURL(t, upstream.URL), time.Second, discardLogger())
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/folders/x", nil))

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, ocekivano 500", rec.Code)
			}
			if msg := errorBody(t, rec); msg != "internal error" {
				t.Errorf("poruka = %q, detalji servisa ne smeju do klijenta", msg)
			}
		})
	}
}

func TestSuccessBodyIsUntouched(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("sadrzaj datoteke"))
	}))
	defer upstream.Close()

	h := New(mustURL(t, upstream.URL), time.Second, discardLogger())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/files/x/content", nil))

	if rec.Code != http.StatusOK || rec.Body.String() != "sadrzaj datoteke" {
		t.Errorf("status = %d, telo = %q", rec.Code, rec.Body.String())
	}
}

func TestIdentityFromAuthIsForwarded(t *testing.T) {
	var gotID, gotRole string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID, gotRole = r.Header.Get(HeaderUserID), r.Header.Get(HeaderUserRole)
	}))
	defer upstream.Close()

	h := New(mustURL(t, upstream.URL), time.Second, discardLogger())
	req := httptest.NewRequest(http.MethodGet, "/folders/x/contents", nil)
	// Klijent pokusava da podmetne tudji identitet; vazi samo onaj iz tokena.
	req.Header.Set(HeaderUserID, "tudji-id")
	req.Header.Set(HeaderUserRole, "ADMIN")
	req = req.WithContext(auth.ContextWithIdentity(context.Background(), auth.Identity{UserID: "u1", Role: "USER"}))
	h.ServeHTTP(httptest.NewRecorder(), req)

	if gotID != "u1" || gotRole != "USER" {
		t.Errorf("servis dobio %s=%q, %s=%q", HeaderUserID, gotID, HeaderUserRole, gotRole)
	}
}
