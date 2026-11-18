package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/service"
)

type fakeLogouter struct {
	err   error
	token string
	calls int
}

func (f *fakeLogouter) Logout(_ context.Context, tok string) error {
	f.calls++
	f.token = tok
	return f.err
}

func doLogout(l Logouter, authHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	LogoutHandler(l).ServeHTTP(rec, req)
	return rec
}

func TestLogoutHandler_Success(t *testing.T) {
	l := &fakeLogouter{}
	rec := doLogout(l, "Bearer jwt-token")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, ocekivano 204", rec.Code)
	}
	if l.token != "jwt-token" {
		t.Errorf("prosledjen token = %q", l.token)
	}
}

func TestLogoutHandler_BearerCaseInsensitive(t *testing.T) {
	l := &fakeLogouter{}
	if rec := doLogout(l, "bearer jwt-token"); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, ocekivano 204", rec.Code)
	}
}

func TestLogoutHandler_MissingOrMalformedHeader(t *testing.T) {
	for _, h := range []string{"", "jwt-token", "Basic abc", "Bearer", "Bearer   "} {
		l := &fakeLogouter{}
		rec := doLogout(l, h)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("zaglavlje %q: status = %d, ocekivano 401", h, rec.Code)
		}
		if l.calls != 0 {
			t.Errorf("zaglavlje %q: servis ne sme biti pozvan", h)
		}
	}
}

func TestLogoutHandler_InvalidToken(t *testing.T) {
	l := &fakeLogouter{err: service.ErrInvalidAuthToken}
	rec := doLogout(l, "Bearer lose")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, ocekivano 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"error"`) {
		t.Errorf("telo nije JSON greska: %s", rec.Body.String())
	}
}

func TestLogoutHandler_InternalError_NoDetails(t *testing.T) {
	l := &fakeLogouter{err: errors.New("mongo: connection refused 10.0.0.5")}
	rec := doLogout(l, "Bearer jwt-token")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, ocekivano 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "mongo") || strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Errorf("odgovor otkriva interne detalje: %s", rec.Body.String())
	}
}
