package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/service"
)

type fakePasswordChanger struct {
	err   error
	in    service.ChangePasswordInput
	calls int
}

func (f *fakePasswordChanger) Change(_ context.Context, in service.ChangePasswordInput) error {
	f.calls++
	f.in = in
	return f.err
}

const validPasswordBody = `{"currentPassword":"Stara-lozinka-1!","newPassword":"Nova-lozinka-2@","newPasswordConfirm":"Nova-lozinka-2@"}`

func doChangePassword(c PasswordChanger, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/users/me/password", strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	ChangePasswordHandler(c).ServeHTTP(rec, req)
	return rec
}

func authed() map[string]string {
	return map[string]string{"X-User-ID": "u1", "Authorization": "Bearer jwt-token"}
}

func TestChangePasswordHandler_Success(t *testing.T) {
	c := &fakePasswordChanger{}
	rec := doChangePassword(c, validPasswordBody, authed())
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, ocekivano 204", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("204 ne sme imati telo: %q", rec.Body.String())
	}
	want := service.ChangePasswordInput{
		UserID: "u1", Token: "jwt-token",
		CurrentPassword: "Stara-lozinka-1!", NewPassword: "Nova-lozinka-2@", NewConfirm: "Nova-lozinka-2@",
	}
	if c.in != want {
		t.Errorf("prosledjeno %+v, ocekivano %+v", c.in, want)
	}
}

func TestChangePasswordHandler_NotAuthenticated(t *testing.T) {
	long := strings.Repeat("a", maxUserIDLen+1)
	for name, headers := range map[string]map[string]string{
		"bez zaglavlja":       nil,
		"bez tokena":          {"X-User-ID": "u1"},
		"bez X-User-ID":       {"Authorization": "Bearer jwt-token"},
		"prazan token":        {"X-User-ID": "u1", "Authorization": "Bearer   "},
		"neispravna sema":     {"X-User-ID": "u1", "Authorization": "Basic abc"},
		"predugacak korisnik": {"X-User-ID": long, "Authorization": "Bearer jwt-token"},
	} {
		c := &fakePasswordChanger{}
		rec := doChangePassword(c, validPasswordBody, headers)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, ocekivano 401", name, rec.Code)
		}
		if c.calls != 0 {
			t.Errorf("%s: servis ne sme biti pozvan", name)
		}
	}
}

func TestChangePasswordHandler_BadBody(t *testing.T) {
	for name, body := range map[string]string{
		"prazno":          ``,
		"nije json":       `nije json`,
		"nepoznato polje": `{"currentPassword":"a","newPassword":"b","newPasswordConfirm":"b","role":"ADMIN"}`,
		"preveliko":       `{"currentPassword":"` + strings.Repeat("a", maxChangePasswordBody) + `"}`,
	} {
		c := &fakePasswordChanger{}
		rec := doChangePassword(c, body, authed())
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, ocekivano 400", name, rec.Code)
		}
		if c.calls != 0 {
			t.Errorf("%s: servis ne sme biti pozvan", name)
		}
	}
}

func TestChangePasswordHandler_ErrorMapping(t *testing.T) {
	allowedAt := time.Date(2026, 11, 27, 9, 30, 0, 0, time.UTC)
	cases := []struct {
		name     string
		err      error
		status   int
		contains string
	}{
		{"nova lozinka ne zadovoljava pravila", &service.PasswordRulesError{Problems: []string{"lozinka mora imati broj", "lozinka mora imati veliko slovo"}},
			http.StatusBadRequest, "lozinka mora imati broj; lozinka mora imati veliko slovo"},
		{"neispravan token", service.ErrInvalidAuthToken, http.StatusUnauthorized, "token"},
		{"pogresna trenutna lozinka", service.ErrWrongPassword, http.StatusForbidden, "pogrešna trenutna lozinka"},
		{"prerano menjanje", &service.PasswordTooNewError{AllowedAt: allowedAt}, http.StatusConflict, "27.11.2026. u 09:30 (UTC)"},
		{"istovremena promena", service.ErrPasswordChangeConflict, http.StatusConflict, "pokušajte ponovo"},
		{"nalog ne postoji", service.ErrUserNotFound, http.StatusNotFound, "nalog nije pronađen"},
	}
	for _, tc := range cases {
		c := &fakePasswordChanger{err: tc.err}
		rec := doChangePassword(c, validPasswordBody, authed())
		if rec.Code != tc.status {
			t.Errorf("%s: status = %d, ocekivano %d", tc.name, rec.Code, tc.status)
		}
		if !strings.Contains(rec.Body.String(), tc.contains) {
			t.Errorf("%s: telo %q ne sadrzi %q", tc.name, rec.Body.String(), tc.contains)
		}
		if !strings.Contains(rec.Body.String(), `"error"`) {
			t.Errorf("%s: telo nije JSON greska: %s", tc.name, rec.Body.String())
		}
	}
}

func TestChangePasswordHandler_WrongCurrentPasswordIsNot401(t *testing.T) {
	// Klijentski interceptor na 401 brise token i odjavljuje korisnika.
	rec := doChangePassword(&fakePasswordChanger{err: service.ErrWrongPassword}, validPasswordBody, authed())
	if rec.Code == http.StatusUnauthorized {
		t.Fatal("pogresna trenutna lozinka ne sme biti 401")
	}
}

func TestChangePasswordHandler_InternalError_NoDetails(t *testing.T) {
	c := &fakePasswordChanger{err: errors.New("mongo: connection refused 10.0.0.5")}
	rec := doChangePassword(c, validPasswordBody, authed())
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, ocekivano 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "mongo") || strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Errorf("odgovor otkriva interne detalje: %s", rec.Body.String())
	}
}

func TestChangePasswordHandler_ResponsesNeverEchoPasswords(t *testing.T) {
	for _, err := range []error{
		&service.PasswordRulesError{Problems: []string{"lozinka mora imati broj"}},
		service.ErrWrongPassword,
		service.ErrPasswordChangeConflict,
		errors.New("interna greska"),
	} {
		rec := doChangePassword(&fakePasswordChanger{err: err}, validPasswordBody, authed())
		for _, secret := range []string{"Stara-lozinka-1!", "Nova-lozinka-2@"} {
			if strings.Contains(rec.Body.String(), secret) {
				t.Errorf("odgovor %d sadrzi lozinku: %s", rec.Code, rec.Body.String())
			}
		}
	}
}
