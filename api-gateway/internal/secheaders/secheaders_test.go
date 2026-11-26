package secheaders

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHeadersOnEveryResponse(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusUnauthorized, http.StatusNotFound} {
		h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/users/me", nil))

		want := map[string]string{
			"Cache-Control":          "no-store",
			"X-Content-Type-Options": "nosniff",
			"X-Frame-Options":        "DENY",
			"Referrer-Policy":        "no-referrer",
		}
		for k, v := range want {
			if got := rec.Header().Get(k); got != v {
				t.Errorf("status %d: %s = %q, ocekivano %q", status, k, got, v)
			}
		}
	}
}

func TestServiceCanOverrideCacheControl(t *testing.T) {
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "private, max-age=60")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/files/1/content", nil))
	if got := rec.Header().Get("Cache-Control"); got != "private, max-age=60" {
		t.Errorf("Cache-Control = %q", got)
	}
}
