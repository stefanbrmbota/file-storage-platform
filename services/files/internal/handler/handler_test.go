package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	New(nil).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "{\"status\":\"ok\"}\n" {
		t.Fatalf("body = %q", got)
	}
}

func TestCreateFolderRequiresUser(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/folders", strings.NewReader(`{"name":"a"}`))
	rec := httptest.NewRecorder()

	New(nil).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestCreateFolderRejectsBadJSON(t *testing.T) {
	for _, body := range []string{`not json`, `{"name":"a","extra":1}`} {
		req := httptest.NewRequest(http.MethodPost, "/folders", strings.NewReader(body))
		req.Header.Set("X-User-ID", "ana")
		rec := httptest.NewRecorder()

		New(nil).Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %q: status = %d, want %d", body, rec.Code, http.StatusBadRequest)
		}
	}
}
