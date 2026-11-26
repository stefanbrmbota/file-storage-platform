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
	maxConfirmBody = 1 << 10 // 1 KB je dovoljno za token
	confirmTimeout = 15 * time.Second
)

// Confirmer potvrđuje registraciju na osnovu jednokratnog tokena.
type Confirmer interface {
	Confirm(ctx context.Context, token string) error
}

type confirmRequest struct {
	Token string `json:"token"`
}

// ConfirmHandler aktivira nalog. Token stiže u telu zahteva (ne u URL-u) i
// nikad se ne loguje. Nevažeći, istekao ili već iskorišćen token daje 400;
// ako zavisni servis nije dostupan, 502 (token ostaje iskoristiv).
func ConfirmHandler(c Confirmer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxConfirmBody)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()

		var req confirmRequest
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "neispravan zahtev: telo mora biti JSON sa poljem token")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), confirmTimeout)
		defer cancel()

		err := c.Confirm(ctx, req.Token)
		switch {
		case err == nil:
			setJSONHeaders(w)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "nalog je aktiviran"})
		case errors.Is(err, service.ErrInvalidToken):
			writeError(w, http.StatusBadRequest, "link je nevažeći, istekao ili je već iskorišćen")
		case errors.Is(err, service.ErrUpstream):
			writeError(w, http.StatusBadGateway, "servis je privremeno nedostupan, pokušajte ponovo")
		default:
			slog.Error("potvrda registracije nije uspela", "err", err)
			writeError(w, http.StatusInternalServerError, "interna greška")
		}
	}
}
