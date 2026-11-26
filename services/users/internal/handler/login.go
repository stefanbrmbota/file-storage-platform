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
	maxLoginBody = 1 << 10 // 1 KB je dovoljno za email i lozinku
	loginTimeout = 15 * time.Second

	// passwordExpiredMessage je odgovor (403) oba koraka prijave kad je lozinka istekla (US-16).
	// Upućuje na reset lozinke (US-15); ne sadrži nikakve podatke o nalogu.
	passwordExpiredMessage = "lozinka je istekla; postavi novu pomoću opcije „Zaboravio si lozinku?“ na stranici za prijavu"
)

// LoginStarter izvodi prvi korak prijave (provera lozinke i slanje OTP-a).
type LoginStarter interface {
	Start(ctx context.Context, email, password string) error
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginHandler je prvi korak prijave. Pogrešan email, pogrešna lozinka i
// neaktivan nalog dobijaju istu poruku (401), pa se ne otkriva da li nalog
// postoji. Istekla lozinka (US-16) dobija 403 sa uputom za reset, ali tek posle
// tačne lozinke. Uspeh vraća 200 bez ikakvih podataka o nalogu. Lozinka se nikad ne loguje.
func LoginHandler(l LoginStarter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxLoginBody)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()

		var req loginRequest
		if err := dec.Decode(&req); err != nil || strings.TrimSpace(req.Email) == "" || req.Password == "" {
			writeError(w, http.StatusBadRequest, "neispravan zahtev: telo mora biti JSON sa poljima email i password")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), loginTimeout)
		defer cancel()

		err := l.Start(ctx, req.Email, req.Password)
		switch {
		case err == nil:
			logLoginAttempt(r, stepLogin, req.Email, outcomeSuccess)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "kod za prijavu je poslat na vaš mejl"})
		case errors.Is(err, service.ErrInvalidCredentials):
			logLoginAttempt(r, stepLogin, req.Email, outcomeFailure)
			writeError(w, http.StatusUnauthorized, "pogrešan email ili lozinka")
		case errors.Is(err, service.ErrPasswordExpired):
			logLoginAttempt(r, stepLogin, req.Email, outcomeExpired)
			writeError(w, http.StatusForbidden, passwordExpiredMessage)
		default:
			logLoginAttempt(r, stepLogin, req.Email, outcomeError)
			slog.Error("prijava (prvi korak) nije uspela", "err", err)
			writeError(w, http.StatusInternalServerError, "interna greška")
		}
	}
}
