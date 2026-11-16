package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/service"
)

const maxRegisterBody = 1 << 20 // 1 MB

// Registrar upisuje validiran zahtev za registraciju (heširanje lozinke i upis
// korisnika, US-04 T3).
type Registrar interface {
	Register(ctx context.Context, in service.RegistrationInput) error
}

// RegisterHandler prima zahtev za registraciju: neispravan zahtev dobija 400 sa
// porukom šta nije u redu, a ispravan se prosleđuje Registrar-u.
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

		if err := reg.Register(r.Context(), in); err != nil {
			// Bez internih detalja ka klijentu; lozinka se nikad ne loguje.
			slog.Error("registracija nije uspela", "err", err)
			writeError(w, http.StatusInternalServerError, "interna greška")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("{}"))
	}
}
