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

type fakeFinisher struct {
	err         error
	email, code string
	calls       int
}

func (f *fakeFinisher) Finish(_ context.Context, email, code string) (string, error) {
	f.calls++
	f.email, f.code = email, code
	if f.err != nil {
		return "", f.err
	}
	return "jwt-token", nil
}

func doLoginVerify(l LoginFinisher, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/login/verify", strings.NewReader(body))
	rec := httptest.NewRecorder()
	LoginVerifyHandler(l).ServeHTTP(rec, req)
	return rec
}

func TestLoginVerifyHandler_Success(t *testing.T) {
	l := &fakeFinisher{}
	rec := doLoginVerify(l, `{"email":"pera@example.com","code":"042517"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, ocekivano 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"token":"jwt-token"`) {
		t.Errorf("odgovor ne sadrzi token: %s", rec.Body.String())
	}
	if l.email != "pera@example.com" || l.code != "042517" {
		t.Errorf("prosledjeno: %q %q", l.email, l.code)
	}
}

func TestLoginVerifyHandler_InvalidOTP_SameResponse(t *testing.T) {
	a := doLoginVerify(&fakeFinisher{err: service.ErrInvalidOTP}, `{"email":"a@example.com","code":"111111"}`)
	b := doLoginVerify(&fakeFinisher{err: service.ErrInvalidOTP}, `{"email":"b@example.com","code":"222222"}`)
	if a.Code != http.StatusUnauthorized || b.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d / %d, ocekivano 401", a.Code, b.Code)
	}
	if a.Body.String() != b.Body.String() {
		t.Errorf("odgovori se razlikuju: %q vs %q", a.Body.String(), b.Body.String())
	}
	if strings.Contains(a.Body.String(), "token") {
		t.Error("greska ne sme sadrzati token")
	}
}

// Istekla lozinka (US-16) u drugom koraku: 403 sa uputom za reset, bez tokena.
func TestLoginVerifyHandler_PasswordExpired(t *testing.T) {
	rec := doLoginVerify(&fakeFinisher{err: service.ErrPasswordExpired}, `{"email":"pera@example.com","code":"482915"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, ocekivano 403", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "lozinka je istekla") || !strings.Contains(body, "Zaboravio si lozinku?") {
		t.Errorf("poruka mora uputiti na reset lozinke: %s", body)
	}
	if strings.Contains(body, `"token"`) || strings.Contains(body, "482915") {
		t.Errorf("odgovor ne sme sadrzati token ni kod: %s", body)
	}
}

func TestLoginVerifyHandler_BadRequest(t *testing.T) {
	tests := map[string]string{
		"neispravan JSON": `{nije json`,
		"prazan email":    `{"email":" ","code":"123456"}`,
		"prazan kod":      `{"email":"a@b.c","code":""}`,
		"kratak kod":      `{"email":"a@b.c","code":"12345"}`,
		"predug kod":      `{"email":"a@b.c","code":"1234567"}`,
		"slova u kodu":    `{"email":"a@b.c","code":"12345a"}`,
		"nepoznato polje": `{"email":"a@b.c","code":"123456","role":"ADMIN"}`,
		"prevelik zahtev": `{"email":"a@b.c","code":"123456","x":"` + strings.Repeat("x", 2000) + `"}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			l := &fakeFinisher{}
			rec := doLoginVerify(l, body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, ocekivano 400", rec.Code)
			}
			if l.calls != 0 {
				t.Error("servis se ne sme pozivati za neispravan zahtev")
			}
		})
	}
}

func TestLoginVerifyHandler_InternalErrorHidesDetails(t *testing.T) {
	rec := doLoginVerify(&fakeFinisher{err: errors.New("mongo: connection refused 10.0.0.5")}, `{"email":"a@b.c","code":"123456"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, ocekivano 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "mongo") || strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Errorf("odgovor otkriva interne detalje: %s", rec.Body.String())
	}
}
