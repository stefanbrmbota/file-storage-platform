package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeChecker struct {
	revoked bool
	err     error
	jti     string
	calls   int
}

func (f *fakeChecker) IsRevoked(_ context.Context, jti string) (bool, error) {
	f.calls++
	f.jti = jti
	return f.revoked, f.err
}

func doTokenCheck(c RevocationChecker, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/internal/tokens/check", strings.NewReader(body))
	rec := httptest.NewRecorder()
	TokenCheckHandler(c).ServeHTTP(rec, req)
	return rec
}

func TestTokenCheckHandler_Revoked(t *testing.T) {
	c := &fakeChecker{revoked: true}
	rec := doTokenCheck(c, `{"jti":"abc123"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, ocekivano 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"revoked":true`) {
		t.Errorf("odgovor: %s", rec.Body.String())
	}
	if c.jti != "abc123" {
		t.Errorf("prosledjen jti = %q", c.jti)
	}
}

func TestTokenCheckHandler_NotRevoked(t *testing.T) {
	rec := doTokenCheck(&fakeChecker{}, `{"jti":"abc123"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"revoked":false`) {
		t.Fatalf("status = %d, odgovor: %s", rec.Code, rec.Body.String())
	}
}

func TestTokenCheckHandler_BadRequest(t *testing.T) {
	long := strings.Repeat("a", maxJTILen+1)
	for _, body := range []string{``, `nije json`, `{}`, `{"jti":""}`, `{"jti":"  "}`, `{"jti":"a","extra":1}`, `{"jti":"` + long + `"}`} {
		c := &fakeChecker{}
		rec := doTokenCheck(c, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("telo %.20q: status = %d, ocekivano 400", body, rec.Code)
		}
		if c.calls != 0 {
			t.Errorf("telo %.20q: servis ne sme biti pozvan", body)
		}
	}
}

func TestTokenCheckHandler_FailClosed_NoDetails(t *testing.T) {
	c := &fakeChecker{err: errors.New("mongo: connection refused 10.0.0.5")}
	rec := doTokenCheck(c, `{"jti":"abc123"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, ocekivano 503", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "mongo") || strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Errorf("odgovor otkriva interne detalje: %s", rec.Body.String())
	}
}
