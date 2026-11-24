package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSessionReturnsIdentity(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	req = req.WithContext(ContextWithIdentity(context.Background(), Identity{UserID: "u1", Role: RoleUser}))
	rec := httptest.NewRecorder()
	SessionHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != `{"role":"USER","userId":"u1"}` {
		t.Errorf("telo = %s", body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("odgovor sa identitetom ne sme da se kesira")
	}
}

func TestSessionWithoutIdentityIs401(t *testing.T) {
	rec := httptest.NewRecorder()
	SessionHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/auth/session", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

// Kroz Require: opozvan token ne dobija sesiju.
func TestSessionBehindRequire(t *testing.T) {
	users := &fakeUsers{revoked: map[string]bool{"jti-1": true}}
	a := newAuth(t, users)
	h := a.Require(SessionHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	req.Header.Set("Authorization", "Bearer "+validToken(t, RoleUser))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("opozvan token: status = %d, ocekivano 401", rec.Code)
	}

	users.revoked = nil
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"userId":"u1"`) {
		t.Fatalf("vazeci token: status = %d, telo = %s", rec.Code, rec.Body)
	}
}
