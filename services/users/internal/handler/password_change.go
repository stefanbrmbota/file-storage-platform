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
	maxChangePasswordBody = 4 << 10 // tri lozinke do 128 karaktera staju sa velikom rezervom
	changePasswordTimeout = 15 * time.Second
)

// PasswordChanger menja lozinku prijavljenog korisnika.
type PasswordChanger interface {
	Change(ctx context.Context, in service.ChangePasswordInput) error
}

type changePasswordRequest struct {
	CurrentPassword    string `json:"currentPassword"`
	NewPassword        string `json:"newPassword"`
	NewPasswordConfirm string `json:"newPasswordConfirm"`
}

// ChangePasswordHandler obrađuje POST /users/me/password (gateway: POST
// /api/users/me/password, US-14). Identitet je iz zaglavlja X-User-ID, a token
// kojim je zahtev poslat (Authorization: Bearer) mora pripadati istom korisniku
// i biva opozvan posle uspeha. Odgovori: 204 uspeh; 400 nova lozinka nije
// prihvatljiva; 401 nema prijave ili je token neispravan; 403 pogrešna trenutna
// lozinka (namerno nije 401, da klijent ne odjavi korisnika); 404 nalog ne
// postoji; 409 lozinka je nedavno promenjena ili se menja istovremeno; 500 bez
// detalja. Lozinke se nikad ne loguju i ne vraćaju.
func ChangePasswordHandler(c PasswordChanger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := strings.TrimSpace(r.Header.Get(headerUserID))
		tok, hasToken := bearerToken(r)
		if userID == "" || len(userID) > maxUserIDLen || !hasToken {
			writeError(w, http.StatusUnauthorized, "potrebna je prijava")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxChangePasswordBody)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		var req changePasswordRequest
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "neispravan zahtev: telo mora biti JSON sa poljima currentPassword, newPassword i newPasswordConfirm")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), changePasswordTimeout)
		defer cancel()

		err := c.Change(ctx, service.ChangePasswordInput{
			UserID:          userID,
			Token:           tok,
			CurrentPassword: req.CurrentPassword,
			NewPassword:     req.NewPassword,
			NewConfirm:      req.NewPasswordConfirm,
		})

		var rules *service.PasswordRulesError
		var tooNew *service.PasswordTooNewError
		switch {
		case err == nil:
			w.WriteHeader(http.StatusNoContent)
		case errors.Is(err, service.ErrInvalidAuthToken):
			writeError(w, http.StatusUnauthorized, "neispravan ili istekao token")
		case errors.As(err, &rules):
			writeError(w, http.StatusBadRequest, "neispravan zahtev: "+strings.Join(rules.Problems, "; "))
		case errors.Is(err, service.ErrWrongPassword):
			writeError(w, http.StatusForbidden, "pogrešna trenutna lozinka")
		case errors.As(err, &tooNew):
			writeError(w, http.StatusConflict, "lozinka je nedavno promenjena; sledeća promena je moguća najranije "+tooNew.AllowedAt.UTC().Format("02.01.2006. u 15:04 (UTC)"))
		case errors.Is(err, service.ErrPasswordChangeConflict):
			writeError(w, http.StatusConflict, "lozinka je upravo promenjena, pokušajte ponovo")
		case errors.Is(err, service.ErrUserNotFound):
			writeError(w, http.StatusNotFound, "nalog nije pronađen")
		default:
			slog.Error("promena lozinke nije uspela", "err", err)
			writeError(w, http.StatusInternalServerError, "interna greška")
		}
	}
}
