package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/service"
)

const (
	maxLoginVerifyBody = 1 << 10 // 1 KB je dovoljno za email i kod
	loginVerifyTimeout = 15 * time.Second
)

var otpFormat = regexp.MustCompile(`^[0-9]{6}$`)

// LoginFinisher je drugi korak prijave: proverava OTP i izdaje JWT.
type LoginFinisher interface {
	Finish(ctx context.Context, email, code string) (string, error)
}

type loginVerifyRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

// LoginVerifyHandler proverava OTP i vraća JWT. Pogrešan, istekao, već
// iskorišćen kod, nepostojeći i neaktivan nalog dobijaju istu poruku (401).
// Ako je lozinka istekla (US-16), a kod je ispravan, odgovor je 403 sa uputom za
// reset i token se ne izdaje. Kod i token se nikad ne loguju.
func LoginVerifyHandler(l LoginFinisher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxLoginVerifyBody)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()

		var req loginVerifyRequest
		if err := dec.Decode(&req); err != nil || strings.TrimSpace(req.Email) == "" || !otpFormat.MatchString(req.Code) {
			writeError(w, http.StatusBadRequest, "neispravan zahtev: telo mora biti JSON sa poljem email i kodom od 6 cifara (code)")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), loginVerifyTimeout)
		defer cancel()

		tok, err := l.Finish(ctx, req.Email, req.Code)
		switch {
		case err == nil:
			logLoginAttempt(r, stepVerify, req.Email, outcomeSuccess)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"token": tok})
		case errors.Is(err, service.ErrInvalidOTP):
			logLoginAttempt(r, stepVerify, req.Email, outcomeFailure)
			writeError(w, http.StatusUnauthorized, "neispravan ili istekao kod")
		case errors.Is(err, service.ErrPasswordExpired):
			logLoginAttempt(r, stepVerify, req.Email, outcomeExpired)
			writeError(w, http.StatusForbidden, passwordExpiredMessage)
		default:
			logLoginAttempt(r, stepVerify, req.Email, outcomeError)
			slog.Error("prijava (drugi korak) nije uspela", "err", err)
			writeError(w, http.StatusInternalServerError, "interna greška")
		}
	}
}
