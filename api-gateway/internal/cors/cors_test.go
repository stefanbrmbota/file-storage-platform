package cors

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

const client = "http://localhost:3000"

// handler belezi da li je zahtev stigao dalje.
func newHandler(reached *bool) http.Handler {
	return Middleware([]string{client}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*reached = true
		w.WriteHeader(http.StatusOK)
	}))
}

func TestPreflightAllowedOrigin(t *testing.T) {
	var reached bool
	req := httptest.NewRequest(http.MethodOptions, "/api/folders", nil)
	req.Header.Set("Origin", client)
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "authorization, content-type")
	rec := httptest.NewRecorder()
	newHandler(&reached).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, ocekivano 204", rec.Code)
	}
	if reached {
		t.Error("preflight ne sme do servisa")
	}
	h := rec.Header()
	if h.Get("Access-Control-Allow-Origin") != client {
		t.Errorf("Allow-Origin = %q", h.Get("Access-Control-Allow-Origin"))
	}
	if h.Get("Access-Control-Allow-Methods") != allowMethods || h.Get("Access-Control-Allow-Headers") != allowHeaders {
		t.Errorf("Allow-Methods = %q, Allow-Headers = %q",
			h.Get("Access-Control-Allow-Methods"), h.Get("Access-Control-Allow-Headers"))
	}
	if h.Get("Access-Control-Allow-Credentials") != "" {
		t.Error("Allow-Credentials ne treba, token ide u headeru")
	}
}

func TestPreflightUnknownOrigin(t *testing.T) {
	var reached bool
	req := httptest.NewRequest(http.MethodOptions, "/api/folders", nil)
	req.Header.Set("Origin", "http://evil.example")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	newHandler(&reached).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden || reached {
		t.Fatalf("status = %d, stiglo dalje = %v", rec.Code, reached)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("nepoznat origin ne sme dobiti Allow-Origin")
	}
}

func TestSimpleRequestAllowedOrigin(t *testing.T) {
	var reached bool
	req := httptest.NewRequest(http.MethodGet, "/api/folders/x/contents", nil)
	req.Header.Set("Origin", client)
	rec := httptest.NewRecorder()
	newHandler(&reached).ServeHTTP(rec, req)

	if !reached || rec.Code != http.StatusOK {
		t.Fatalf("status = %d, stiglo dalje = %v", rec.Code, reached)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != client {
		t.Errorf("Allow-Origin = %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
	if rec.Header().Get("Vary") != "Origin" {
		t.Errorf("Vary = %q", rec.Header().Get("Vary"))
	}
}

func TestSimpleRequestUnknownOrigin(t *testing.T) {
	var reached bool
	req := httptest.NewRequest(http.MethodGet, "/api/folders/x/contents", nil)
	req.Header.Set("Origin", "http://evil.example")
	rec := httptest.NewRecorder()
	newHandler(&reached).ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("nepoznat origin ne sme dobiti Allow-Origin")
	}
}

func TestNoOriginPassesThrough(t *testing.T) {
	var reached bool
	rec := httptest.NewRecorder()
	newHandler(&reached).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if !reached || rec.Header().Get("Access-Control-Allow-Origin") != "" || rec.Header().Get("Vary") != "" {
		t.Errorf("stiglo dalje = %v, headeri = %v", reached, rec.Header())
	}
}

func TestParseOrigins(t *testing.T) {
	got := ParseOrigins(" http://localhost:3000/ , ,https://app.example.com")
	want := []string{"http://localhost:3000", "https://app.example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
