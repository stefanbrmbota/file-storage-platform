package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
)

func TestRequireRoleAdmin(t *testing.T) {
	tests := []struct {
		name       string
		role       model.Role
		setRole    bool
		wantStatus int
		wantCalled bool
	}{
		{"admin prolazi", model.RoleAdmin, true, http.StatusOK, true},
		{"user dobija 403", model.RoleUser, true, http.StatusForbidden, false},
		{"bez uloge dobija 403", "", false, http.StatusForbidden, false},
		{"prazna uloga dobija 403", "", true, http.StatusForbidden, false},
		{"nepoznata uloga dobija 403", model.Role("SUPERADMIN"), true, http.StatusForbidden, false},
		{"mala slova nisu admin", model.Role("admin"), true, http.StatusForbidden, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})
			h := RequireRole(model.RoleAdmin)(next)

			req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
			if tt.setRole {
				req = req.WithContext(ContextWithRole(req.Context(), tt.role))
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, očekivano %d", rec.Code, tt.wantStatus)
			}
			if called != tt.wantCalled {
				t.Fatalf("next pozvan = %v, očekivano %v", called, tt.wantCalled)
			}
		})
	}
}

func TestRequireRoleForbiddenBody(t *testing.T) {
	h := RequireRole(model.RoleAdmin)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	req = req.WithContext(ContextWithRole(req.Context(), model.RoleUser))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("telo nije JSON: %v", err)
	}
	if body["error"] == "" {
		t.Fatalf("očekivano polje error, dobijeno %v", body)
	}
}

func TestRequireRoleMultipleRoles(t *testing.T) {
	h := RequireRole(model.RoleAdmin, model.RoleUser)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req = req.WithContext(ContextWithRole(req.Context(), model.RoleUser))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, očekivano 200", rec.Code)
	}
}
