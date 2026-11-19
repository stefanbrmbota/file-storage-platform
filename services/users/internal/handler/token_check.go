package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	maxTokenCheckBody = 1 << 10 // jti je kratak; 1 KB je dovoljno
	maxJTILen         = 128
	tokenCheckTimeout = 5 * time.Second
)

// RevocationChecker proverava da li je token (po jti) opozvan odjavom.
type RevocationChecker interface {
	IsRevoked(ctx context.Context, jti string) (bool, error)
}

type tokenCheckRequest struct {
	JTI string `json:"jti"`
}

// TokenCheckHandler obrađuje POST /internal/tokens/check (S6, samo interna
// mreža): za jti vraća {"revoked": true|false}. jti ide u telo, ne u URL.
// Pri grešci baze vraća 503, da pozivalac (gateway) odbije zahtev (fail closed).
func TokenCheckHandler(c RevocationChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxTokenCheckBody)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()

		var req tokenCheckRequest
		if err := dec.Decode(&req); err != nil || strings.TrimSpace(req.JTI) == "" || len(req.JTI) > maxJTILen {
			writeError(w, http.StatusBadRequest, "neispravan zahtev: telo mora biti JSON sa poljem jti")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), tokenCheckTimeout)
		defer cancel()

		revoked, err := c.IsRevoked(ctx, req.JTI)
		if err != nil {
			slog.Error("provera opozvanog tokena nije uspela", "err", err)
			writeError(w, http.StatusServiceUnavailable, "servis trenutno nije dostupan")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"revoked": revoked})
	}
}
