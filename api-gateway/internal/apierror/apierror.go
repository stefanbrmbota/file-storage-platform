// Package apierror je jedinstven format greske koju gateway vraca klijentu
// (US-30, T-30.3): JSON {"error": "..."} sa odgovarajucim HTTP statusom,
// bez stack trace-a i internih detalja (IB zahtev).
package apierror

import (
	"encoding/json"
	"net/http"
)

// Body je telo svakog odgovora sa greskom.
type Body struct {
	Error string `json:"error"`
}

// Write upisuje gresku u jedinstvenom formatu.
func Write(w http.ResponseWriter, status int, msg string) {
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Body{Error: msg})
}

// WriteStatus upisuje gresku sa opstom porukom za dati status.
func WriteStatus(w http.ResponseWriter, status int) {
	Write(w, status, Message(status))
}

// Message je opsta poruka za status, za slucaj kad stvarni razlog ne sme ili
// ne moze da se prenese klijentu.
func Message(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "bad request"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not found"
	case http.StatusMethodNotAllowed:
		return "method not allowed"
	case http.StatusRequestEntityTooLarge:
		return "request too large"
	case http.StatusTooManyRequests:
		return "too many requests"
	case http.StatusBadGateway:
		return "service unavailable"
	case http.StatusServiceUnavailable:
		return "service unavailable"
	case http.StatusGatewayTimeout:
		return "service timeout"
	}
	if status >= 500 {
		return "internal error"
	}
	return "request failed"
}

// NotFound je handler za sve sto nije na listi ruta.
func NotFound(w http.ResponseWriter, _ *http.Request) {
	WriteStatus(w, http.StatusNotFound)
}
