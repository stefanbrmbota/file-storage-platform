package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/service"
)

const (
	maxQuotaRequestBody = 1 << 10 // telo je {"requestedBytes": N}
	quotaRequestTimeout = 5 * time.Second
)

// QuotaRequestSubmitter podnosi zahtev za povećanje kvote.
type QuotaRequestSubmitter interface {
	Submit(ctx context.Context, userID string, requestedBytes int64) (model.QuotaRequest, error)
}

// QuotaRequestReader vraća najnoviji zahtev korisnika.
type QuotaRequestReader interface {
	Latest(ctx context.Context, userID string) (model.QuotaRequest, error)
}

type quotaRequestBody struct {
	RequestedBytes int64 `json:"requestedBytes"`
}

// SubmitQuotaRequestHandler obrađuje POST /users/me/quota-requests (gateway:
// POST /api/users/me/quota-requests, US-21). Identitet je isključivo iz
// zaglavlja X-User-ID. Odgovori: 201 sa zahtevom (status PENDING); 400 telo nije
// JSON sa poljem requestedBytes, vrednost nije ispravna ili nije veća od
// trenutne kvote; 401 nema prijave; 404 nalog ne postoji; 409 korisnik već ima
// zahtev na čekanju; 500 bez detalja.
func SubmitQuotaRequestHandler(s QuotaRequestSubmitter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := strings.TrimSpace(r.Header.Get(headerUserID))
		if userID == "" || len(userID) > maxUserIDLen {
			writeError(w, http.StatusUnauthorized, "potrebna je prijava")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxQuotaRequestBody)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		var req quotaRequestBody
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "neispravan zahtev: telo mora biti JSON sa poljem requestedBytes")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), quotaRequestTimeout)
		defer cancel()

		created, err := s.Submit(ctx, userID, req.RequestedBytes)
		switch {
		case err == nil:
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(created)
		case errors.Is(err, service.ErrInvalidQuotaRequest):
			writeError(w, http.StatusBadRequest, "tražena kvota nije ispravna")
		case errors.Is(err, service.ErrQuotaNotIncreased):
			writeError(w, http.StatusBadRequest, "tražena kvota mora biti veća od trenutne")
		case errors.Is(err, service.ErrQuotaRequestPending):
			writeError(w, http.StatusConflict, "već imaš zahtev na čekanju")
		case errors.Is(err, service.ErrUserNotFound):
			writeError(w, http.StatusNotFound, "nalog nije pronađen")
		default:
			slog.Error("zahtev za kvotu nije podnet", "err", err)
			writeError(w, http.StatusInternalServerError, "interna greška")
		}
	}
}

// LatestQuotaRequestHandler obrađuje GET /users/me/quota-requests/latest
// (gateway: GET /api/users/me/quota-requests/latest, US-21): najnoviji zahtev
// prijavljenog korisnika sa statusom. Identitet je isključivo iz zaglavlja
// X-User-ID, pa korisnik vidi samo svoj zahtev. Odgovori: 200; 401 nema prijave;
// 404 korisnik još nije podneo zahtev; 500 bez detalja.
func LatestQuotaRequestHandler(s QuotaRequestReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := strings.TrimSpace(r.Header.Get(headerUserID))
		if userID == "" || len(userID) > maxUserIDLen {
			writeError(w, http.StatusUnauthorized, "potrebna je prijava")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), quotaRequestTimeout)
		defer cancel()

		req, err := s.Latest(ctx, userID)
		switch {
		case err == nil:
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(w).Encode(req)
		case errors.Is(err, service.ErrNoQuotaRequest), errors.Is(err, service.ErrUserNotFound):
			writeError(w, http.StatusNotFound, "nema podnetog zahteva za kvotu")
		default:
			slog.Error("zahtev za kvotu nije učitan", "err", err)
			writeError(w, http.StatusInternalServerError, "interna greška")
		}
	}
}
