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
// postoji. Uspeh vraća 200 bez ikakvih podataka o nalogu. Lozinka se nikad ne loguje.
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
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "kod za prijavu je poslat na vaš mejl"})
		case errors.Is(err, service.ErrInvalidCredentials):
			writeError(w, http.StatusUnauthorized, "pogrešan email ili lozinka")
		default:
			slog.Error("prijava (prvi korak) nije uspela", "err", err)
			writeError(w, http.StatusInternalServerError, "interna greška")
		}
	}
}
