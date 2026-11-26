package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// US-66: JSON odgovori nose nosniff, a korisnički podaci u njima su enkodovani.
func TestJSONResponsesAreEncoded(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, map[string]string{"name": `<script>alert(1)</script>`})

	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	if strings.ContainsAny(rec.Body.String(), "<>") || !strings.Contains(rec.Body.String(), `\u003cscript`) {
		t.Errorf("HTML u JSON-u nije enkodovan: %s", rec.Body.String())
	}
}
