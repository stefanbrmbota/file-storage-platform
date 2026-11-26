package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/service"
)

// QuotaDecider odobrava i odbija zahteve za povećanje kvote.
type QuotaDecider interface {
	Approve(ctx context.Context, adminID, requestID string) (model.QuotaRequest, error)
	Reject(ctx context.Context, adminID, requestID string) (model.QuotaRequest, error)
}

// ApproveQuotaRequestHandler obrađuje POST /admin/quota-requests/{id}/approve
// (gateway: isto pod /api, US-23): kvota korisnika se postavlja na traženu vrednost.
// Odgovori: 200 sa ažuriranim zahtevom; 401 nema prijave; 403 pozivalac nije
// administrator; 404 zahtev (ili njegov korisnik) ne postoji; 409 zahtev je već
// odlučen; 500 bez detalja.
func ApproveQuotaRequestHandler(d QuotaDecider) http.HandlerFunc {
	return quotaDecisionHandler(d.Approve, "odobravanje zahteva za kvotu nije uspelo")
}

// RejectQuotaRequestHandler obrađuje POST /admin/quota-requests/{id}/reject
// (US-23): zahtev dobija status REJECTED, a kvota korisnika ostaje ista. Odgovori
// su isti kao za odobravanje.
func RejectQuotaRequestHandler(d QuotaDecider) http.HandlerFunc {
	return quotaDecisionHandler(d.Reject, "odbijanje zahteva za kvotu nije uspelo")
}

func quotaDecisionHandler(decide func(ctx context.Context, adminID, requestID string) (model.QuotaRequest, error), logMsg string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminID, ok := adminCaller(w, r)
		if !ok {
			return
		}
		requestID, ok := pathID(w, r)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), adminTimeout)
		defer cancel()

		req, err := decide(ctx, adminID, requestID)
		switch {
		case err == nil:
			setJSONHeaders(w)
			w.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(w).Encode(req)
		case errors.Is(err, service.ErrForbidden):
			writeError(w, http.StatusForbidden, "zabranjen pristup")
		case errors.Is(err, service.ErrQuotaRequestNotFound):
			writeError(w, http.StatusNotFound, "zahtev za kvotu nije pronađen")
		case errors.Is(err, service.ErrUserNotFound):
			writeError(w, http.StatusNotFound, "korisnik zahteva više ne postoji")
		case errors.Is(err, service.ErrQuotaRequestDecided):
			writeError(w, http.StatusConflict, "zahtev je već odlučen")
		default:
			slog.Error(logMsg, "err", err)
			writeError(w, http.StatusInternalServerError, "interna greška")
		}
	}
}
