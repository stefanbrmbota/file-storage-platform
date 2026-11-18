package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/service"
)

const logoutTimeout = 10 * time.Second

// Logouter opoziva token pri odjavi.
type Logouter interface {
	Logout(ctx context.Context, tokenString string) error
}

// LogoutHandler obrađuje POST /logout. Token se čita iz zaglavlja
// Authorization: Bearer <token>; ispravna odjava vraća 204. Neispravan,
// istekao ili nedostajući token dobija istu poruku (401). Token se ne loguje.
func LogoutHandler(l Logouter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok, ok := bearerToken(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "neispravan ili istekao token")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), logoutTimeout)
		defer cancel()

		err := l.Logout(ctx, tok)
		switch {
		case err == nil:
			w.WriteHeader(http.StatusNoContent)
		case errors.Is(err, service.ErrInvalidAuthToken):
			writeError(w, http.StatusUnauthorized, "neispravan ili istekao token")
		default:
			slog.Error("odjava nije uspela", "err", err)
			writeError(w, http.StatusInternalServerError, "interna greška")
		}
	}
}

// bearerToken vraća token iz zaglavlja Authorization (šema Bearer, bez obzira na velika slova).
func bearerToken(r *http.Request) (string, bool) {
	const prefix = "bearer "
	h := r.Header.Get("Authorization")
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	tok := strings.TrimSpace(h[len(prefix):])
	return tok, tok != ""
}
