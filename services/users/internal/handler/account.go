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

const (
	// headerUserID postavlja samo gateway iz JWT-a (design D6); klijentska vrednost se briše.
	headerUserID   = "X-User-ID"
	maxUserIDLen   = 128
	accountTimeout = 5 * time.Second
)

// AccountGetter vraća podatke o nalogu sa kvotom.
type AccountGetter interface {
	Get(ctx context.Context, userID string) (service.Account, error)
}

// MeHandler obrađuje GET /users/me (gateway: GET /api/users/me, US-06): podaci o
// nalogu prijavljenog korisnika, kvota, zauzeće i preostali prostor. Identitet
// dolazi isključivo iz zaglavlja X-User-ID, nikad iz URL-a ni tela. Bez
// zaglavlja 401, nepoznat nalog 404, greška baze 500 bez detalja.
func MeHandler(a AccountGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get(headerUserID))
		if id == "" || len(id) > maxUserIDLen {
			writeError(w, http.StatusUnauthorized, "potrebna je prijava")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), accountTimeout)
		defer cancel()

		acc, err := a.Get(ctx, id)
		switch {
		case err == nil:
			setJSONHeaders(w)
			w.Header().Set("Cache-Control", "no-store") // lični podaci
			_ = json.NewEncoder(w).Encode(acc)
		case errors.Is(err, service.ErrUserNotFound):
			writeError(w, http.StatusNotFound, "nalog nije pronađen")
		default:
			slog.Error("podaci o nalogu nisu učitani", "err", err)
			writeError(w, http.StatusInternalServerError, "interna greška")
		}
	}
}
