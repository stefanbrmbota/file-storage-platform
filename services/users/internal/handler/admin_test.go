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

type fakeAdminLister struct {
	users    []service.AdminUser
	requests []service.AdminQuotaRequest
	err      error
	caller   string
	calls    int
}

func (f *fakeAdminLister) ListUsers(_ context.Context, adminID string) ([]service.AdminUser, error) {
	f.calls++
	f.caller = adminID
	return f.users, f.err
}

func (f *fakeAdminLister) ListQuotaRequests(_ context.Context, adminID string) ([]service.AdminQuotaRequest, error) {
	f.calls++
	f.caller = adminID
	return f.requests, f.err
}

func adminGet(h http.HandlerFunc, path, userID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAdminUsersHandler_OK(t *testing.T) {
	f := &fakeAdminLister{users: []service.AdminUser{
		{ID: "u1", FirstName: "Ana", LastName: "Anić", Email: "ana@example.com", Role: model.RoleUser, Active: true, QuotaBytes: 1 << 30, UsedBytes: 5},
	}}
	rec := adminGet(AdminUsersHandler(f), "/admin/users", "admin")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, telo %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Users []map[string]any `json:"users"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Users) != 1 {
		t.Fatalf("telo: %s", rec.Body.String())
	}
	u := body.Users[0]
	if u["firstName"] != "Ana" || u["email"] != "ana@example.com" || u["quotaBytes"] != float64(1<<30) || u["usedBytes"] != float64(5) {
		t.Errorf("korisnik: %v", u)
	}
	if f.caller != "admin" {
		t.Errorf("servisu prosledjen pozivalac %q", f.caller)
	}
	if rec.Header().Get("Content-Type") != "application/json" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("zaglavlja: %v", rec.Header())
	}
}

func TestAdminUsersHandler_EmptyListIsArray(t *testing.T) {
	rec := adminGet(AdminUsersHandler(&fakeAdminLister{users: []service.AdminUser{}}), "/admin/users", "admin")
	if !strings.Contains(rec.Body.String(), `"users":[]`) {
		t.Errorf("telo: %s", rec.Body.String())
	}
}

func TestAdminQuotaRequestsHandler_OK(t *testing.T) {
	f := &fakeAdminLister{requests: []service.AdminQuotaRequest{
		{ID: "r1", UserID: "u1", FirstName: "Ana", LastName: "Anić", Email: "ana@example.com", CurrentQuotaBytes: 1 << 30, RequestedBytes: 5 << 30, Status: model.QuotaRequestPending},
	}}
	rec := adminGet(AdminQuotaRequestsHandler(f), "/admin/quota-requests", "admin")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, telo %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Requests []map[string]any `json:"requests"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Requests) != 1 {
		t.Fatalf("telo: %s", rec.Body.String())
	}
	r := body.Requests[0]
	if r["id"] != "r1" || r["email"] != "ana@example.com" || r["currentQuotaBytes"] != float64(1<<30) ||
		r["requestedBytes"] != float64(5<<30) || r["status"] != "PENDING" {
		t.Errorf("zahtev: %v", r)
	}
}

func TestAdminHandlers_Unauthenticated(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"users":    AdminUsersHandler(&fakeAdminLister{}),
		"requests": AdminQuotaRequestsHandler(&fakeAdminLister{}),
	} {
		for label, id := range map[string]string{"bez zaglavlja": "", "predug id": strings.Repeat("a", maxUserIDLen+1)} {
			if rec := adminGet(h, "/admin/x", id); rec.Code != http.StatusUnauthorized {
				t.Errorf("%s/%s: status = %d, ocekivano 401", name, label, rec.Code)
			}
		}
	}
	f := &fakeAdminLister{}
	adminGet(AdminUsersHandler(f), "/admin/users", "")
	if f.calls != 0 {
		t.Error("bez prijave servis se ne zove")
	}
}

func TestAdminHandlers_ForbiddenForNonAdmin(t *testing.T) {
	f := &fakeAdminLister{err: service.ErrForbidden, users: []service.AdminUser{{ID: "tajna"}}}
	for name, h := range map[string]http.HandlerFunc{"users": AdminUsersHandler(f), "requests": AdminQuotaRequestsHandler(f)} {
		rec := adminGet(h, "/admin/x", "user")
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"error"`) {
			t.Errorf("%s: status = %d, telo %s", name, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "tajna") {
			t.Errorf("%s: odgovor 403 ne sme sadrzati podatke: %s", name, rec.Body.String())
		}
	}
}

func TestAdminHandlers_InternalErrorHidesDetails(t *testing.T) {
	f := &fakeAdminLister{err: errors.New("mongo: connection refused 10.0.0.5")}
	for name, h := range map[string]http.HandlerFunc{"users": AdminUsersHandler(f), "requests": AdminQuotaRequestsHandler(f)} {
		rec := adminGet(h, "/admin/x", "admin")
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("%s: status = %d", name, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "mongo") || strings.Contains(rec.Body.String(), "10.0.0.5") {
			t.Errorf("%s: telo otkriva interne detalje: %s", name, rec.Body.String())
		}
	}
}
