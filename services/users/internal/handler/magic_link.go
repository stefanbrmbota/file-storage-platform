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
	maxMagicRequestBody = 1 << 10 // telo je {"email": "..."}
	maxMagicLoginBody   = 1 << 10 // telo je {"token": "..."}, token ima najviše 128 karaktera
	magicRequestTimeout = 15 * time.Second
	magicLoginTimeout   = 15 * time.Second

	// stepMagicLink je korak prijave magičnim linkom u logu pokušaja prijave (US-17).
	stepMagicLink = "magic-link"

	// magicRequestMessage je odgovor na svaki ispravan zahtev, bez obzira da li nalog postoji.
	magicRequestMessage = "Ako nalog sa tom adresom postoji, poslali smo link za prijavu."
)

// MagicLinkRequester traži slanje magičnog linka.
type MagicLinkRequester interface {
	Request(ctx context.Context, email string) error
}

// MagicLinkLoginer prijavljuje korisnika tokenom iz magičnog linka i vraća JWT.
type MagicLinkLoginer interface {
	Login(ctx context.Context, rawToken string) (string, error)
}

type magicRequestBody struct {
	Email string `json:"email"`
}

type magicLoginBody struct {
	Token string `json:"token"`
}

// MagicLinkRequestHandler obrađuje POST /magic-link/request (gateway:
// POST /api/auth/magic-link/request, US-17). Javna ruta. Svaki ispravan zahtev
// dobija isti odgovor 202, bez obzira da li nalog postoji, da se ne bi mogli
// proveravati tuđi nalozi. 400 samo za telo koje nije JSON sa emailom ili email
// lošeg oblika (oblik ne otkriva postojanje naloga); 500 samo za kvar servisa,
// bez detalja.
func MagicLinkRequestHandler(m MagicLinkRequester) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxMagicRequestBody)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()

		var req magicRequestBody
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "neispravan zahtev: telo mora biti JSON sa poljem email")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), magicRequestTimeout)
		defer cancel()

		err := m.Request(ctx, req.Email)
		switch {
		case err == nil:
			setJSONHeaders(w)
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": magicRequestMessage})
		case errors.Is(err, service.ErrInvalidEmail):
			writeError(w, http.StatusBadRequest, "email nije ispravan")
		default:
			slog.Error("zahtev za magicni link nije uspeo", "err", err)
			writeError(w, http.StatusInternalServerError, "interna greška")
		}
	}
}

// MagicLinkLoginHandler obrađuje POST /magic-link/login (gateway:
// POST /api/auth/magic-link/login, US-17). Javna ruta. Token stiže u telu (ne u
// URL-u) i nikad se ne loguje. Odgovori: 200 sa JWT-om; 401 link je nevažeći,
// istekao ili već iskorišćen (jedna poruka za sve slučajeve); 400 telo nije JSON
// sa poljem token; 500 bez detalja. Svaki pokušaj ulazi u log pokušaja prijave
// (korak "magic-link", IP i ishod; token se ne upisuje, a email nije poznat dok
// se token ne potroši).
func MagicLinkLoginHandler(m MagicLinkLoginer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxMagicLoginBody)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()

		var req magicLoginBody
		if err := dec.Decode(&req); err != nil || strings.TrimSpace(req.Token) == "" {
			writeError(w, http.StatusBadRequest, "neispravan zahtev: telo mora biti JSON sa poljem token")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), magicLoginTimeout)
		defer cancel()

		tok, err := m.Login(ctx, req.Token)
		switch {
		case err == nil:
			logMagicLinkAttempt(r, outcomeSuccess)
			setJSONHeaders(w)
			_ = json.NewEncoder(w).Encode(map[string]string{"token": tok})
		case errors.Is(err, service.ErrInvalidToken):
			logMagicLinkAttempt(r, outcomeFailure)
			writeError(w, http.StatusUnauthorized, "link je nevažeći, istekao ili je već iskorišćen")
		default:
			logMagicLinkAttempt(r, outcomeError)
			slog.Error("prijava magicnim linkom nije uspela", "err", err)
			writeError(w, http.StatusInternalServerError, "interna greška")
		}
	}
}

// logMagicLinkAttempt upisuje zapis o pokušaju prijave magičnim linkom. Isti je
// oblik kao u logLoginAttempt (poruka "pokusaj prijave"), ali bez emaila: kod
// prijave linkom korisnik se predstavlja samo tokenom, koji se nikad ne loguje.
// Razlog odbijanja se ne upisuje.
func logMagicLinkAttempt(r *http.Request, outcome string) {
	level := slog.LevelInfo
	if outcome != outcomeSuccess {
		level = slog.LevelWarn
	}
	slog.Log(r.Context(), level, "pokusaj prijave",
		"step", stepMagicLink,
		"ip", clientIP(r),
		"outcome", outcome,
	)
}
