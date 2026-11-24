// Package handler su HTTP handleri Notifications servisa.
package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
)

// Pinger proverava da je baza dostupna.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Health obradjuje GET /health: 200 ako baza odgovara, inace 503 (T-07.1).
// Docker healthcheck zove ovu rutu, pa servis bez baze nije "healthy".
// Razlog greske ide samo u log, ne klijentu.
func Health(p Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := p.Ping(r.Context()); err != nil {
			slog.Warn("health: baza nije dostupna", "err", err)
			writeError(w, http.StatusServiceUnavailable, "service unavailable")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
