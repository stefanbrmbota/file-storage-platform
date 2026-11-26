package secheaders

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHSTS(t *testing.T) {
	rec := httptest.NewRecorder()
	HSTS(http.NotFoundHandler()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/users/me", nil))
	if got := rec.Header().Get("Strict-Transport-Security"); got != "max-age=31536000" {
		t.Errorf("Strict-Transport-Security = %q", got)
	}
}
