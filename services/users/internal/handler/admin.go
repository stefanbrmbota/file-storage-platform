package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/service"
)

const adminTimeout = 10 * time.Second

// AdminLister vraća administratorski pregled korisnika i zahteva za kvotu.
type AdminLister interface {
	ListUsers(ctx context.Context, adminID string) ([]service.AdminUser, error)
	ListQuotaRequests(ctx context.Context, adminID string) ([]service.AdminQuotaRequest, error)
}

// AdminUsersHandler obrađuje GET /admin/users (gateway: GET /api/admin/users,
// US-22): lista korisnika sa imenom, emailom, kvotom i zauzećem. Identitet je
// isključivo iz zaglavlja X-User-ID, a servis uvek proverava u bazi da li je
// pozivalac administrator. Odgovori: 200 sa {"users": [...]}; 401 nema prijave;
// 403 pozivalac nije administrator; 500 bez detalja.
func AdminUsersHandler(a AdminLister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminID, ok := adminCaller(w, r)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), adminTimeout)
		defer cancel()

		users, err := a.ListUsers(ctx, adminID)
		if !adminResult(w, err, "lista korisnika nije učitana") {
			return
		}
		writeAdminJSON(w, map[string]any{"users": users})
	}
}

// AdminQuotaRequestsHandler obrađuje GET /admin/quota-requests (gateway:
// GET /api/admin/quota-requests, US-22): zahtevi za kvotu sa podnosiocem, njegovom
// trenutnom kvotom, traženom kvotom i statusom; prvo oni na čekanju. Odgovori isti
// kao za listu korisnika, telo je {"requests": [...]}.
func AdminQuotaRequestsHandler(a AdminLister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminID, ok := adminCaller(w, r)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), adminTimeout)
		defer cancel()

		reqs, err := a.ListQuotaRequests(ctx, adminID)
		if !adminResult(w, err, "lista zahteva za kvotu nije učitana") {
			return
		}
		writeAdminJSON(w, map[string]any{"requests": reqs})
	}
}

// adminCaller vraća ID pozivaoca iz zaglavlja ili odgovara 401.
func adminCaller(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := strings.TrimSpace(r.Header.Get(headerUserID))
	if id == "" || len(id) > maxUserIDLen {
		writeError(w, http.StatusUnauthorized, "potrebna je prijava")
		return "", false
	}
	return id, true
}

// adminResult odgovara greškom (403 ili 500) i vraća false, ili vraća true kad je sve u redu.
func adminResult(w http.ResponseWriter, err error, logMsg string) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, service.ErrForbidden):
		writeError(w, http.StatusForbidden, "zabranjen pristup")
	default:
		slog.Error(logMsg, "err", err)
		writeError(w, http.StatusInternalServerError, "interna greška")
	}
	return false
}

func writeAdminJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store") // lični podaci
	_ = json.NewEncoder(w).Encode(body)
}
