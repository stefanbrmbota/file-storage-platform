package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testInternalToken = "test-internal-token-at-least-32-chars!"

func newInternalMux(t *testing.T, q *fakeReserver, r *fakeReleaser) *http.ServeMux {
	t.Helper()
	internalOnly, err := RequireInternalToken(testInternalToken)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("POST /internal/users/{id}/quota/reserve", internalOnly(ReserveQuotaHandler(q)))
	mux.Handle("POST /internal/users/{id}/quota/release", internalOnly(ReleaseQuotaHandler(r)))
	return mux
}

func postInternal(mux *http.ServeMux, path, token string, setHeader bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"bytes":10}`))
	if setHeader {
		req.Header.Set(InternalTokenHeader, token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestRequireInternalToken_WeakToken(t *testing.T) {
	for _, tok := range []string{"", "kratko", strings.Repeat("a", MinInternalTokenLen-1)} {
		if _, err := RequireInternalToken(tok); !errors.Is(err, ErrWeakInternalToken) {
			t.Errorf("token duzine %d: err = %v, ocekivano ErrWeakInternalToken", len(tok), err)
		}
	}
	if _, err := RequireInternalToken(strings.Repeat("a", MinInternalTokenLen)); err != nil {
		t.Errorf("token minimalne duzine: err = %v", err)
	}
}

func TestInternalRoutes_ValidToken(t *testing.T) {
	q, r := &fakeReserver{remaining: 90}, &fakeReleaser{used: 10}
	mux := newInternalMux(t, q, r)

	if rec := postInternal(mux, "/internal/users/u1/quota/reserve", testInternalToken, true); rec.Code != http.StatusOK {
		t.Errorf("reserve: status = %d, ocekivano 200", rec.Code)
	}
	if rec := postInternal(mux, "/internal/users/u1/quota/release", testInternalToken, true); rec.Code != http.StatusOK {
		t.Errorf("release: status = %d, ocekivano 200", rec.Code)
	}
	if q.calls != 1 || r.calls != 1 {
		t.Errorf("pozivi: reserve = %d, release = %d; ocekivano 1 i 1", q.calls, r.calls)
	}
}

func TestInternalRoutes_RejectBadToken(t *testing.T) {
	cases := []struct {
		name      string
		token     string
		setHeader bool
	}{
		{"bez zaglavlja", "", false},
		{"prazno zaglavlje", "", true},
		{"pogresan token", "pogresan-token-pogresan-token-pogresan", true},
		{"prefiks tokena", testInternalToken[:len(testInternalToken)-1], true},
		{"token sa nastavkom", testInternalToken + "x", true},
	}
	for _, path := range []string{"/internal/users/u1/quota/reserve", "/internal/users/u1/quota/release"} {
		for _, tc := range cases {
			t.Run(path+" "+tc.name, func(t *testing.T) {
				q, r := &fakeReserver{}, &fakeReleaser{}
				rec := postInternal(newInternalMux(t, q, r), path, tc.token, tc.setHeader)
				if rec.Code != http.StatusUnauthorized {
					t.Fatalf("status = %d, ocekivano 401", rec.Code)
				}
				if !strings.Contains(rec.Body.String(), `"error"`) {
					t.Errorf("odgovor mora biti JSON sa error: %s", rec.Body.String())
				}
				if strings.Contains(rec.Body.String(), testInternalToken) {
					t.Errorf("odgovor otkriva token: %s", rec.Body.String())
				}
				if q.calls != 0 || r.calls != 0 {
					t.Error("servis ne sme biti pozvan bez ispravnog tokena")
				}
			})
		}
	}
}
