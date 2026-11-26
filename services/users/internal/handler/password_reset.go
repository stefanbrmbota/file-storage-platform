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
	maxResetRequestBody = 1 << 10 // telo je {"email": "..."}
	maxResetConfirmBody = 4 << 10 // token i dve lozinke do 128 karaktera
	resetRequestTimeout = 15 * time.Second
	resetConfirmTimeout = 15 * time.Second

	// resetRequestMessage je odgovor na svaki ispravan zahtev, bez obzira da li nalog postoji.
	resetRequestMessage = "Ako nalog sa tom adresom postoji, poslali smo link za reset lozinke."
)

// PasswordResetRequester traži slanje linka za reset lozinke.
type PasswordResetRequester interface {
	Request(ctx context.Context, email string) error
}

// PasswordResetter postavlja novu lozinku na osnovu tokena iz mejla.
type PasswordResetter interface {
	Reset(ctx context.Context, in service.ResetPasswordInput) error
}

type resetRequestBody struct {
	Email string `json:"email"`
}

type resetConfirmBody struct {
	Token              string `json:"token"`
	NewPassword        string `json:"newPassword"`
	NewPasswordConfirm string `json:"newPasswordConfirm"`
}

// PasswordResetRequestHandler obrađuje POST /password-reset/request (gateway:
// POST /api/auth/password-reset/request, US-15). Javna ruta. Svaki ispravan
// zahtev dobija isti odgovor 202, bez obzira da li nalog postoji, da se ne bi
// mogli proveravati tuđi nalozi. 400 samo za telo koje nije JSON sa emailom ili
// email lošeg oblika (oblik ne otkriva postojanje naloga); 500 samo za kvar
// servisa, bez detalja.
func PasswordResetRequestHandler(p PasswordResetRequester) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxResetRequestBody)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()

		var req resetRequestBody
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "neispravan zahtev: telo mora biti JSON sa poljem email")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), resetRequestTimeout)
		defer cancel()

		err := p.Request(ctx, req.Email)
		switch {
		case err == nil:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": resetRequestMessage})
		case errors.Is(err, service.ErrInvalidEmail):
			writeError(w, http.StatusBadRequest, "email nije ispravan")
		default:
			slog.Error("zahtev za reset lozinke nije uspeo", "err", err)
			writeError(w, http.StatusInternalServerError, "interna greška")
		}
	}
}

// PasswordResetConfirmHandler obrađuje POST /password-reset/confirm (gateway:
// POST /api/auth/password-reset/confirm, US-15). Javna ruta. Token stiže u
// telu (ne u URL-u) i nikad se ne loguje. Odgovori: 204 lozinka promenjena; 400
// nova lozinka nije prihvatljiva ili je link nevažeći, istekao ili već
// iskorišćen (jedna poruka za sva tri slučaja); 409 lozinka se istovremeno
// menjala (link ostaje upotrebljiv); 500 bez detalja. Lozinke se ne loguju ni ne vraćaju.
func PasswordResetConfirmHandler(p PasswordResetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxResetConfirmBody)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()

		var req resetConfirmBody
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "neispravan zahtev: telo mora biti JSON sa poljima token, newPassword i newPasswordConfirm")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), resetConfirmTimeout)
		defer cancel()

		err := p.Reset(ctx, service.ResetPasswordInput{
			Token:       req.Token,
			NewPassword: req.NewPassword,
			NewConfirm:  req.NewPasswordConfirm,
		})

		var rules *service.PasswordRulesError
		switch {
		case err == nil:
			w.WriteHeader(http.StatusNoContent)
		case errors.Is(err, service.ErrInvalidToken):
			writeError(w, http.StatusBadRequest, "link je nevažeći, istekao ili je već iskorišćen")
		case errors.As(err, &rules):
			writeError(w, http.StatusBadRequest, "neispravan zahtev: "+strings.Join(rules.Problems, "; "))
		case errors.Is(err, service.ErrPasswordChangeConflict):
			writeError(w, http.StatusConflict, "lozinka je upravo promenjena, pokušajte ponovo")
		default:
			slog.Error("reset lozinke nije uspeo", "err", err)
			writeError(w, http.StatusInternalServerError, "interna greška")
		}
	}
}
