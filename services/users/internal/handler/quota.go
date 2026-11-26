package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/service"
)

const (
	maxQuotaBody      = 1 << 10 // telo je {"bytes": N}
	quotaCheckTimeout = 5 * time.Second
)

// QuotaReserver rezervise prostor u kvoti korisnika.
type QuotaReserver interface {
	ReserveQuota(ctx context.Context, userID string, bytes int64) (int64, error)
}

// QuotaReleaser oslobadja prostor u kvoti korisnika.
type QuotaReleaser interface {
	ReleaseQuota(ctx context.Context, userID string, bytes int64) (int64, error)
}

type reserveQuotaRequest struct {
	Bytes int64 `json:"bytes"`
}

type releaseQuotaResponse struct {
	Released  int64 `json:"released"`
	UsedBytes int64 `json:"usedBytes"`
}

// ReleaseQuotaHandler obradjuje POST /internal/users/{id}/quota/release (S3, samo
// interna mreza). Odgovori: 200 oslobodjeno, 400 neispravan zahtev, 404 nepoznat
// korisnik, 503 greska baze.
func ReleaseQuotaHandler(q QuotaReleaser) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxQuotaBody)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()

		var req reserveQuotaRequest
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "neispravan zahtev: telo mora biti JSON sa poljem bytes")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), quotaCheckTimeout)
		defer cancel()

		used, err := q.ReleaseQuota(ctx, r.PathValue("id"), req.Bytes)
		switch {
		case err == nil:
			setJSONHeaders(w)
			_ = json.NewEncoder(w).Encode(releaseQuotaResponse{Released: req.Bytes, UsedBytes: used})
		case errors.Is(err, service.ErrInvalidQuotaRequest):
			writeError(w, http.StatusBadRequest, "neispravan zahtev: bytes mora biti pozitivan broj")
		case errors.Is(err, service.ErrUserNotFound):
			writeError(w, http.StatusNotFound, "korisnik nije pronadjen")
		default:
			slog.Error("oslobadjanje kvote nije uspelo", "err", err)
			writeError(w, http.StatusServiceUnavailable, "servis trenutno nije dostupan")
		}
	}
}

type reserveQuotaResponse struct {
	Reserved       int64 `json:"reserved"`
	RemainingBytes int64 `json:"remainingBytes"`
}

// ReserveQuotaHandler obradjuje POST /internal/users/{id}/quota/reserve (S3, samo
// interna mreza). Odgovori: 200 rezervisano, 400 neispravan zahtev, 404 nepoznat
// korisnik, 409 nema mesta, 503 greska baze.
func ReserveQuotaHandler(q QuotaReserver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxQuotaBody)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()

		var req reserveQuotaRequest
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "neispravan zahtev: telo mora biti JSON sa poljem bytes")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), quotaCheckTimeout)
		defer cancel()

		remaining, err := q.ReserveQuota(ctx, r.PathValue("id"), req.Bytes)
		switch {
		case err == nil:
			setJSONHeaders(w)
			_ = json.NewEncoder(w).Encode(reserveQuotaResponse{Reserved: req.Bytes, RemainingBytes: remaining})
		case errors.Is(err, service.ErrInvalidQuotaRequest):
			writeError(w, http.StatusBadRequest, "neispravan zahtev: bytes mora biti pozitivan broj")
		case errors.Is(err, service.ErrUserNotFound):
			writeError(w, http.StatusNotFound, "korisnik nije pronadjen")
		case errors.Is(err, service.ErrQuotaExceeded):
			writeError(w, http.StatusConflict, "nema dovoljno prostora u kvoti")
		default:
			slog.Error("rezervacija kvote nije uspela", "err", err)
			writeError(w, http.StatusServiceUnavailable, "servis trenutno nije dostupan")
		}
	}
}
