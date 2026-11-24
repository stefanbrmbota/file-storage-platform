package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func TestHealthOK(t *testing.T) {
	rec := httptest.NewRecorder()
	Health(fakePinger{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok"`) {
		t.Errorf("status = %d, telo = %q", rec.Code, rec.Body.String())
	}
}

func TestHealthDatabaseDown(t *testing.T) {
	rec := httptest.NewRecorder()
	Health(fakePinger{err: errors.New("dial tcp 10.0.0.5:9042: connection refused")}).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, ocekivano 503", rec.Code)
	}
	if body := rec.Body.String(); strings.Contains(body, "10.0.0.5") || !strings.Contains(body, `"error"`) {
		t.Errorf("telo = %q; ocekivan opsti JSON bez internih detalja", body)
	}
}
