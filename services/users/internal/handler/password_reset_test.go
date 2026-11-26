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

type fakeRequester struct {
	err   error
	email string
	calls int
}

func (f *fakeRequester) Request(_ context.Context, email string) error {
	f.calls++
	f.email = email
	return f.err
}

type fakeResetter struct {
	err   error
	in    service.ResetPasswordInput
	calls int
}

func (f *fakeResetter) Reset(_ context.Context, in service.ResetPasswordInput) error {
	f.calls++
	f.in = in
	return f.err
}

func post(h http.HandlerFunc, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// ---- POST /password-reset/request ----

func TestResetRequestHandler_AlwaysSameAnswer(t *testing.T) {
	// Isti odgovor bez obzira da li servis nesto uradio (nalog postoji ili ne).
	a := post(PasswordResetRequestHandler(&fakeRequester{}), "/password-reset/request", `{"email":"ana@example.com"}`)
	b := post(PasswordResetRequestHandler(&fakeRequester{}), "/password-reset/request", `{"email":"niko@example.com"}`)
	for _, rec := range []*httptest.ResponseRecorder{a, b} {
		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d, ocekivano 202", rec.Code)
		}
	}
	if a.Body.String() != b.Body.String() {
		t.Errorf("odgovori se razlikuju:\n%s\n%s", a.Body.String(), b.Body.String())
	}
	if !strings.Contains(a.Body.String(), `"message"`) {
		t.Errorf("telo: %s", a.Body.String())
	}
	if ct := a.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestResetRequestHandler_PassesEmail(t *testing.T) {
	r := &fakeRequester{}
	post(PasswordResetRequestHandler(r), "/password-reset/request", `{"email":" Ana@Example.com "}`)
	if r.email != " Ana@Example.com " || r.calls != 1 {
		t.Errorf("prosledjeno %q (%d poziva); normalizacija je u servisu", r.email, r.calls)
	}
}

func TestResetRequestHandler_InvalidEmail(t *testing.T) {
	rec := post(PasswordResetRequestHandler(&fakeRequester{err: service.ErrInvalidEmail}), "/password-reset/request", `{"email":"nije-email"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"error"`) {
		t.Fatalf("status = %d, telo %s", rec.Code, rec.Body.String())
	}
}

func TestResetRequestHandler_BadBody(t *testing.T) {
	for name, body := range map[string]string{
		"prazno":          ``,
		"nije json":       `nije json`,
		"nepoznato polje": `{"email":"a@b.co","role":"ADMIN"}`,
		"preveliko":       `{"email":"` + strings.Repeat("a", maxResetRequestBody) + `"}`,
	} {
		r := &fakeRequester{}
		rec := post(PasswordResetRequestHandler(r), "/password-reset/request", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, ocekivano 400", name, rec.Code)
		}
		if r.calls != 0 {
			t.Errorf("%s: servis ne sme biti pozvan", name)
		}
	}
}

func TestResetRequestHandler_InternalError_NoDetails(t *testing.T) {
	rec := post(PasswordResetRequestHandler(&fakeRequester{err: errors.New("mongo: connection refused 10.0.0.5")}), "/password-reset/request", `{"email":"ana@example.com"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, ocekivano 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "mongo") || strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Errorf("odgovor otkriva interne detalje: %s", rec.Body.String())
	}
}

// ---- POST /password-reset/confirm ----

const confirmBody = `{"token":"abc-token","newPassword":"Nova-lozinka-2@","newPasswordConfirm":"Nova-lozinka-2@"}`

func TestResetConfirmHandler_Success(t *testing.T) {
	r := &fakeResetter{}
	rec := post(PasswordResetConfirmHandler(r), "/password-reset/confirm", confirmBody)
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("status = %d, telo %q; ocekivano prazno 204", rec.Code, rec.Body.String())
	}
	want := service.ResetPasswordInput{Token: "abc-token", NewPassword: "Nova-lozinka-2@", NewConfirm: "Nova-lozinka-2@"}
	if r.in != want {
		t.Errorf("prosledjeno %+v, ocekivano %+v", r.in, want)
	}
}

func TestResetConfirmHandler_BadBody(t *testing.T) {
	for name, body := range map[string]string{
		"prazno":          ``,
		"nije json":       `nije json`,
		"nepoznato polje": `{"token":"a","newPassword":"b","newPasswordConfirm":"b","userId":"u2"}`,
		"preveliko":       `{"token":"` + strings.Repeat("a", maxResetConfirmBody) + `"}`,
	} {
		r := &fakeResetter{}
		rec := post(PasswordResetConfirmHandler(r), "/password-reset/confirm", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, ocekivano 400", name, rec.Code)
		}
		if r.calls != 0 {
			t.Errorf("%s: servis ne sme biti pozvan", name)
		}
	}
}

func TestResetConfirmHandler_ErrorMapping(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		status   int
		contains string
	}{
		{"nevazeci link", service.ErrInvalidToken, http.StatusBadRequest, "link je nevažeći, istekao ili je već iskorišćen"},
		{"pravila lozinke", &service.PasswordRulesError{Problems: []string{"lozinka mora imati broj", "lozinka mora imati veliko slovo"}},
			http.StatusBadRequest, "lozinka mora imati broj; lozinka mora imati veliko slovo"},
		{"istovremena promena", service.ErrPasswordChangeConflict, http.StatusConflict, "pokušajte ponovo"},
	}
	for _, tc := range cases {
		rec := post(PasswordResetConfirmHandler(&fakeResetter{err: tc.err}), "/password-reset/confirm", confirmBody)
		if rec.Code != tc.status {
			t.Errorf("%s: status = %d, ocekivano %d", tc.name, rec.Code, tc.status)
		}
		if !strings.Contains(rec.Body.String(), tc.contains) || !strings.Contains(rec.Body.String(), `"error"`) {
			t.Errorf("%s: telo %q", tc.name, rec.Body.String())
		}
	}
}

func TestResetConfirmHandler_InternalError_NoDetails(t *testing.T) {
	rec := post(PasswordResetConfirmHandler(&fakeResetter{err: errors.New("mongo: connection refused 10.0.0.5")}), "/password-reset/confirm", confirmBody)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, ocekivano 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "mongo") || strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Errorf("odgovor otkriva interne detalje: %s", rec.Body.String())
	}
}

func TestResetConfirmHandler_ResponsesNeverEchoSecrets(t *testing.T) {
	for _, err := range []error{
		service.ErrInvalidToken,
		&service.PasswordRulesError{Problems: []string{"lozinka mora imati broj"}},
		service.ErrPasswordChangeConflict,
		errors.New("interna greska"),
	} {
		rec := post(PasswordResetConfirmHandler(&fakeResetter{err: err}), "/password-reset/confirm", confirmBody)
		for _, secret := range []string{"abc-token", "Nova-lozinka-2@"} {
			if strings.Contains(rec.Body.String(), secret) {
				t.Errorf("odgovor %d sadrzi tajnu: %s", rec.Code, rec.Body.String())
			}
		}
	}
}
