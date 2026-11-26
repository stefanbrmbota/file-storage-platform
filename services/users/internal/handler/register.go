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
	maxRegisterBody = 1 << 20 // 1 MB
	registerTimeout = 15 * time.Second
)

// Registrar kreira nalog iz validiranog zahteva (heširanje lozinke i upis korisnika).
type Registrar interface {
	Register(ctx context.Context, in service.RegistrationInput) (model.User, error)
}

// RegisterHandler prima zahtev za registraciju: neispravan zahtev dobija 400 sa
// porukom šta nije u redu, zauzet email 409, a uspeh 201 sa podacima naloga
// (bez lozinke i heša).
func RegisterHandler(reg Registrar) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRegisterBody)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()

		var in service.RegistrationInput
		if err := dec.Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, "neispravan zahtev: telo mora biti ispravan JSON sa poljima firstName, lastName, email, password i passwordConfirm")
			return
		}

		if problems := service.ValidateRegistration(in); len(problems) > 0 {
			writeError(w, http.StatusBadRequest, "neispravan zahtev: "+strings.Join(problems, "; "))
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), registerTimeout)
		defer cancel()

		u, err := reg.Register(ctx, in)
		if errors.Is(err, service.ErrEmailTaken) {
			writeError(w, http.StatusConflict, "email je već zauzet")
			return
		}
		if err != nil {
			// Bez internih detalja ka klijentu; lozinka se nikad ne loguje.
			slog.Error("registracija nije uspela", "err", err)
			writeError(w, http.StatusInternalServerError, "interna greška")
			return
		}
		setJSONHeaders(w)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(u)
	}
}
