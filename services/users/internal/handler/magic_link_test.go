package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/service"
)

type fakeMagicRequester struct {
	err   error
	email string
	calls int
}

func (f *fakeMagicRequester) Request(_ context.Context, email string) error {
	f.calls++
	f.email = email
	return f.err
}

type fakeMagicLoginer struct {
	token string
	err   error
	raw   string
	calls int
}

func (f *fakeMagicLoginer) Login(_ context.Context, raw string) (string, error) {
	f.calls++
	f.raw = raw
	return f.token, f.err
}

// ---- POST /magic-link/request ----

func TestMagicRequestHandler_AlwaysSameAnswer(t *testing.T) {
	a := post(MagicLinkRequestHandler(&fakeMagicRequester{}), "/magic-link/request", `{"email":"ana@example.com"}`)
	b := post(MagicLinkRequestHandler(&fakeMagicRequester{}), "/magic-link/request", `{"email":"niko@example.com"}`)
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

func TestMagicRequestHandler_PassesEmail(t *testing.T) {
	r := &fakeMagicRequester{}
	post(MagicLinkRequestHandler(r), "/magic-link/request", `{"email":" Ana@Example.com "}`)
	if r.email != " Ana@Example.com " || r.calls != 1 {
		t.Errorf("prosledjeno %q (%d poziva); normalizacija je u servisu", r.email, r.calls)
	}
}

func TestMagicRequestHandler_InvalidEmail(t *testing.T) {
	rec := post(MagicLinkRequestHandler(&fakeMagicRequester{err: service.ErrInvalidEmail}), "/magic-link/request", `{"email":"nije-email"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"error"`) {
		t.Fatalf("status = %d, telo %s", rec.Code, rec.Body.String())
	}
}

func TestMagicRequestHandler_MalformedBody(t *testing.T) {
	for name, body := range map[string]string{
		"prazno":          ``,
		"nije json":       `email=ana@example.com`,
		"nepoznato polje": `{"email":"ana@example.com","extra":1}`,
		"pogresan tip":    `{"email":123}`,
		"prevelik":        `{"email":"` + strings.Repeat("a", 2000) + `"}`,
	} {
		r := &fakeMagicRequester{}
		rec := post(MagicLinkRequestHandler(r), "/magic-link/request", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, ocekivano 400", name, rec.Code)
		}
		if r.calls != 0 {
			t.Errorf("%s: servis se ne sme zvati", name)
		}
	}
}

func TestMagicRequestHandler_InternalErrorHidesDetails(t *testing.T) {
	rec := post(MagicLinkRequestHandler(&fakeMagicRequester{err: errors.New("mongo: connection refused 10.0.0.5")}), "/magic-link/request", `{"email":"ana@example.com"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "mongo") || strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Errorf("telo otkriva interne detalje: %s", rec.Body.String())
	}
}

// ---- POST /magic-link/login ----

func TestMagicLoginHandler_Success(t *testing.T) {
	buf := captureLog(t)
	l := &fakeMagicLoginer{token: "jwt-vrednost"}

	rec := post(MagicLinkLoginHandler(l), "/magic-link/login", `{"token":"tajni-token-iz-linka"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, telo %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp["token"] != "jwt-vrednost" {
		t.Fatalf("telo: %s", rec.Body.String())
	}
	if l.raw != "tajni-token-iz-linka" || l.calls != 1 {
		t.Errorf("servisu prosledjeno %q (%d poziva)", l.raw, l.calls)
	}
	logged := buf.String()
	if !strings.Contains(logged, `"step":"magic-link"`) || !strings.Contains(logged, `"outcome":"success"`) {
		t.Errorf("nedostaje zapis o prijavi: %s", logged)
	}
	if strings.Contains(logged, "tajni-token-iz-linka") || strings.Contains(logged, "jwt-vrednost") {
		t.Errorf("token ne sme u log: %s", logged)
	}
}

func TestMagicLoginHandler_InvalidToken(t *testing.T) {
	buf := captureLog(t)
	rec := post(MagicLinkLoginHandler(&fakeMagicLoginer{err: service.ErrInvalidToken}), "/magic-link/login", `{"token":"stari-token"}`)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), `"error"`) {
		t.Fatalf("status = %d, telo %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"token"`) {
		t.Errorf("neuspeh ne sme vratiti token: %s", rec.Body.String())
	}
	logged := buf.String()
	if !strings.Contains(logged, `"outcome":"failure"`) || !strings.Contains(logged, `"level":"WARN"`) {
		t.Errorf("neuspeh mora biti zabelezen kao WARN: %s", logged)
	}
	if strings.Contains(logged, "stari-token") {
		t.Errorf("token ne sme u log: %s", logged)
	}
}

func TestMagicLoginHandler_MalformedBody(t *testing.T) {
	for name, body := range map[string]string{
		"prazno":          ``,
		"nije json":       `token=abc`,
		"prazan token":    `{"token":""}`,
		"samo razmaci":    `{"token":"   "}`,
		"nepoznato polje": `{"token":"abc","email":"ana@example.com"}`,
		"prevelik":        `{"token":"` + strings.Repeat("a", 2000) + `"}`,
	} {
		l := &fakeMagicLoginer{}
		rec := post(MagicLinkLoginHandler(l), "/magic-link/login", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, ocekivano 400", name, rec.Code)
		}
		if l.calls != 0 {
			t.Errorf("%s: servis se ne sme zvati", name)
		}
	}
}

func TestMagicLoginHandler_InternalErrorHidesDetails(t *testing.T) {
	buf := captureLog(t)
	rec := post(MagicLinkLoginHandler(&fakeMagicLoginer{err: errors.New("mongo: connection refused 10.0.0.5")}), "/magic-link/login", `{"token":"abc"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "mongo") || strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Errorf("telo otkriva interne detalje: %s", rec.Body.String())
	}
	if !strings.Contains(buf.String(), `"outcome":"error"`) {
		t.Errorf("kvar mora biti zabelezen: %s", buf.String())
	}
}

func TestMagicLoginHandler_LogsClientIP(t *testing.T) {
	buf := captureLog(t)
	req := httptest.NewRequest(http.MethodPost, "/magic-link/login", strings.NewReader(`{"token":"abc"}`))
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	MagicLinkLoginHandler(&fakeMagicLoginer{err: service.ErrInvalidToken}).ServeHTTP(httptest.NewRecorder(), req)

	if !strings.Contains(buf.String(), `"ip":"203.0.113.7"`) {
		t.Errorf("IP klijenta mora biti u logu: %s", buf.String())
	}
}
