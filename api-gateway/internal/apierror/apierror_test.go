package apierror

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWrite(t *testing.T) {
	rec := httptest.NewRecorder()
	Write(rec, http.StatusConflict, "folder already exists")

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	var b Body
	if err := json.NewDecoder(rec.Body).Decode(&b); err != nil || b.Error != "folder already exists" {
		t.Errorf("telo = %+v, err = %v", b, err)
	}
}

func TestNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	NotFound(rec, httptest.NewRequest("GET", "/x", nil))
	if rec.Code != http.StatusNotFound || rec.Body.String() != "{\"error\":\"not found\"}\n" {
		t.Errorf("status = %d, telo = %q", rec.Code, rec.Body.String())
	}
}

func TestMessageNeverEmpty(t *testing.T) {
	for status := 400; status < 600; status++ {
		if Message(status) == "" {
			t.Errorf("prazna poruka za %d", status)
		}
	}
	if Message(500) != "internal error" || Message(502) != "service unavailable" || Message(504) != "service timeout" {
		t.Error("pogresne poruke za 5xx")
	}
}
