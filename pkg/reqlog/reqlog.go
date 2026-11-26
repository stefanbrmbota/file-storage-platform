// Package reqlog beleži zahteve koje je servis odbio zbog neispravnog ulaza
// (US-64): 400, 413, 415 i 422.
//
// Middleware se postavlja oko celog rutera servisa, pa hvata svaku neuspelu
// validaciju bez izmene pojedinačnih handlera:
//
//	srv := &http.Server{Handler: reqlog.RejectedInput(logger, mux)}
//
// U log idu metoda, ruta, status, razlog (poruka greške koju servis vraća
// klijentu), ID korisnika i IP adresa. Telo zahteva, query string i
// zaglavlja se NE loguju, pa lozinke, tokeni i OTP nikad ne stižu u log.
package reqlog

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
)

// maxCapturedBody ograničava koliko se odgovora čuva da bi se pročitao razlog.
const maxCapturedBody = 512

// maxReasonLen ograničava dužinu razloga u logu.
const maxReasonLen = 200

// rejected su statusi koji znače da ulaz nije prošao validaciju.
var rejected = map[int]bool{
	http.StatusBadRequest:            true,
	http.StatusRequestEntityTooLarge: true,
	http.StatusUnsupportedMediaType:  true,
	http.StatusUnprocessableEntity:   true,
}

// RejectedInput loguje (nivo WARN) svaki odgovor sa statusom neuspele validacije.
func RejectedInput(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &recorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if !rejected[rec.status] {
			return
		}
		route := r.Pattern
		if route == "" {
			route = r.Method + " " + r.URL.Path
		}
		logger.Warn("neuspela validacija ulaza",
			"route", route,
			"status", rec.status,
			"reason", reason(rec.body.Bytes()),
			"userId", truncate(r.Header.Get("X-User-ID"), 128),
			"ip", clientIP(r),
		)
	})
}

// recorder pamti status i početak tela odgovora, a sve prosleđuje dalje.
type recorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	body        bytes.Buffer
}

func (r *recorder) WriteHeader(status int) {
	if !r.wroteHeader {
		r.status = status
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *recorder) Write(p []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	if rejected[r.status] && r.body.Len() < maxCapturedBody {
		n := min(len(p), maxCapturedBody-r.body.Len())
		r.body.Write(p[:n])
	}
	return r.ResponseWriter.Write(p)
}

// Unwrap omogućava http.ResponseController-u da dođe do pravog writer-a.
func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// reason čita poruku iz {"error": "..."}; servisi ne stavljaju vrednosti
// polja u poruku, pa je bezbedna za log.
func reason(body []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &e) != nil || e.Error == "" {
		return "nepoznat razlog"
	}
	return truncate(e.Error, maxReasonLen)
}

// clientIP uzima prvu adresu iz X-Forwarded-For (postavlja je gateway),
// a bez nje adresu konekcije.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first, _, _ := strings.Cut(xff, ",")
		return truncate(strings.TrimSpace(first), 64)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func truncate(s string, n int) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1 // bez prelaza u novi red: zapis u logu se ne može lažirati
		}
		return r
	}, s)
	if len(s) > n {
		return s[:n]
	}
	return s
}
