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

type fakeLogin struct {
	err             error
	email, password string
	calls           int
}

func (f *fakeLogin) Start(_ context.Context, email, password string) error {
	f.calls++
	f.email, f.password = email, password
	return f.err
}

func doLogin(l LoginStarter, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	rec := httptest.NewRecorder()
	LoginHandler(l).ServeHTTP(rec, req)
	return rec
}

func TestLoginHandler_Success(t *testing.T) {
	l := &fakeLogin{}
	rec := doLogin(l, `{"email":"pera@example.com","password":"tajna"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, ocekivano 200", rec.Code)
	}
	if l.email != "pera@example.com" || l.password != "tajna" {
		t.Errorf("prosledjeno: %q %q", l.email, l.password)
	}
	if strings.Contains(rec.Body.String(), "tajna") {
		t.Error("odgovor ne sme sadrzati lozinku")
	}
}

// Pogresna lozinka, nepostojeci email i neaktivan nalog u servisu daju istu
// gresku, pa odgovor mora biti identican.
func TestLoginHandler_InvalidCredentials_SameResponse(t *testing.T) {
	a := doLogin(&fakeLogin{err: service.ErrInvalidCredentials}, `{"email":"a@example.com","password":"x"}`)
	b := doLogin(&fakeLogin{err: service.ErrInvalidCredentials}, `{"email":"b@example.com","password":"y"}`)
	if a.Code != http.StatusUnauthorized || b.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d / %d, ocekivano 401", a.Code, b.Code)
	}
	if a.Body.String() != b.Body.String() {
		t.Errorf("odgovori se razlikuju: %q vs %q", a.Body.String(), b.Body.String())
	}
}

// Istekla lozinka (US-16) dobija 403 sa uputom za reset, razlicit od 401 za
// pogresne podatke, a odgovor ne sadrzi podatke o nalogu ni lozinku.
func TestLoginHandler_PasswordExpired(t *testing.T) {
	rec := doLogin(&fakeLogin{err: service.ErrPasswordExpired}, `{"email":"pera@example.com","password":"TajnaLozinka123!"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, ocekivano 403", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"error"`) || !strings.Contains(body, "lozinka je istekla") || !strings.Contains(body, "Zaboravio si lozinku?") {
		t.Errorf("poruka mora uputiti na reset lozinke: %s", body)
	}
	for _, leak := range []string{"TajnaLozinka123!", "pera@example.com"} {
		if strings.Contains(body, leak) {
			t.Errorf("odgovor sadrzi %q: %s", leak, body)
		}
	}
	invalid := doLogin(&fakeLogin{err: service.ErrInvalidCredentials}, `{"email":"pera@example.com","password":"x"}`)
	if invalid.Code == rec.Code {
		t.Error("istekla lozinka mora imati drugaciji status od pogresnih podataka")
	}
}

func TestLoginHandler_BadRequest(t *testing.T) {
	tests := map[string]string{
		"neispravan JSON": `{nije json`,
		"prazan email":    `{"email":"  ","password":"x"}`,
		"prazna lozinka":  `{"email":"a@b.c","password":""}`,
		"nepoznato polje": `{"email":"a@b.c","password":"x","role":"ADMIN"}`,
		"prevelik zahtev": `{"email":"a@b.c","password":"` + strings.Repeat("x", 2000) + `"}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			l := &fakeLogin{}
			rec := doLogin(l, body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, ocekivano 400", rec.Code)
			}
			if l.calls != 0 {
				t.Error("servis se ne sme pozivati za neispravan zahtev")
			}
		})
	}
}

func TestLoginHandler_InternalErrorHidesDetails(t *testing.T) {
	rec := doLogin(&fakeLogin{err: errors.New("smtp: connection refused 10.0.0.5")}, `{"email":"a@b.c","password":"x"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, ocekivano 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "smtp") || strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Errorf("odgovor otkriva interne detalje: %s", rec.Body.String())
	}
}
