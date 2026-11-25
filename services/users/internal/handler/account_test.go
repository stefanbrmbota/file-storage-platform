package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/service"
)

type fakeAccountGetter struct {
	acc   service.Account
	err   error
	id    string
	calls int
}

func (f *fakeAccountGetter) Get(_ context.Context, id string) (service.Account, error) {
	f.calls++
	f.id = id
	return f.acc, f.err
}

func doMe(a AccountGetter, target string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	MeHandler(a).ServeHTTP(rec, req)
	return rec
}

func sampleAccount() service.Account {
	return service.Account{
		ID: "u1", FirstName: "Ana", LastName: "Anić", Email: "ana@example.com",
		Role: model.RoleUser, QuotaBytes: 1000, UsedBytes: 250, RemainingBytes: 750,
	}
}

func TestMeHandler_Success(t *testing.T) {
	g := &fakeAccountGetter{acc: sampleAccount()}
	rec := doMe(g, "/users/me", map[string]string{"X-User-ID": "u1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, ocekivano 200", rec.Code)
	}
	if g.id != "u1" {
		t.Errorf("prosledjen id = %q", g.id)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, ocekivano no-store", cc)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"id": "u1", "firstName": "Ana", "lastName": "Anić", "email": "ana@example.com", "role": "USER",
		"quotaBytes": float64(1000), "usedBytes": float64(250), "remainingBytes": float64(750),
	}
	if len(body) != len(want) {
		t.Errorf("polja = %v, ocekivano tacno %d", body, len(want))
	}
	for k, v := range want {
		if body[k] != v {
			t.Errorf("%s = %v, ocekivano %v", k, body[k], v)
		}
	}
}

func TestMeHandler_MissingOrBadHeader(t *testing.T) {
	for name, headers := range map[string]map[string]string{
		"bez zaglavlja": nil,
		"prazno":        {"X-User-ID": ""},
		"samo razmaci":  {"X-User-ID": "   "},
		"predugacak id": {"X-User-ID": strings.Repeat("a", maxUserIDLen+1)},
	} {
		g := &fakeAccountGetter{acc: sampleAccount()}
		rec := doMe(g, "/users/me", headers)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, ocekivano 401", name, rec.Code)
		}
		if g.calls != 0 {
			t.Errorf("%s: servis ne sme biti pozvan", name)
		}
		if !strings.Contains(rec.Body.String(), `"error"`) {
			t.Errorf("%s: telo nije JSON greska: %s", name, rec.Body.String())
		}
	}
}

func TestMeHandler_IdentityOnlyFromHeader(t *testing.T) {
	g := &fakeAccountGetter{acc: sampleAccount()}
	rec := doMe(g, "/users/me?id=u2&userId=u2", map[string]string{"X-User-ID": "u1"})
	if rec.Code != http.StatusOK || g.id != "u1" {
		t.Fatalf("status = %d, id = %q; ocekivano id iz zaglavlja (u1)", rec.Code, g.id)
	}
}

func TestMeHandler_NotFound(t *testing.T) {
	g := &fakeAccountGetter{err: service.ErrUserNotFound}
	rec := doMe(g, "/users/me", map[string]string{"X-User-ID": "u1"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, ocekivano 404", rec.Code)
	}
}

func TestMeHandler_InternalError_NoDetails(t *testing.T) {
	g := &fakeAccountGetter{err: errors.New("mongo: connection refused 10.0.0.5")}
	rec := doMe(g, "/users/me", map[string]string{"X-User-ID": "u1"})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, ocekivano 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "mongo") || strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Errorf("odgovor otkriva interne detalje: %s", rec.Body.String())
	}
}
