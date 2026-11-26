package handler

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"unicode"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/service"
)

// Koraci prijave i ishodi pokušaja koji se upisuju u log (US-05).
const (
	stepLogin  = "login"  // prvi korak: email i lozinka
	stepVerify = "verify" // drugi korak: OTP

	outcomeSuccess = "success"
	outcomeFailure = "failure" // odbijen pokušaj (pogrešni podaci, pogrešan kod)
	outcomeError   = "error"   // interna greška, pokušaj nije mogao biti obrađen
	outcomeExpired = "expired" // lozinka je istekla (US-16): prijava je onemogućena do reseta lozinke

	maxLoggedEmailLen = 254
)

// logLoginAttempt upisuje jedan zapis o pokušaju prijave: korak, email, IP
// adresu i ishod (vreme dodaje slog). Namerno ne prima lozinku, OTP ni token,
// pa ne mogu završiti u logu. Razlog odbijanja se ne upisuje: log ne sme da
// otkrije ono što odgovor klijentu skriva (npr. da nalog postoji). Izuzetak je
// ishod "expired" (US-16): do njega se stiže tek posle tačne lozinke odnosno OTP-a,
// pa ne otkriva ništa novo, a to je događaj koji treba da bude vidljiv u reviziji.
func logLoginAttempt(r *http.Request, step, email, outcome string) {
	level := slog.LevelInfo
	if outcome != outcomeSuccess {
		level = slog.LevelWarn
	}
	slog.Log(r.Context(), level, "pokusaj prijave",
		"step", step,
		"email", loggableEmail(email),
		"ip", clientIP(r),
		"outcome", outcome,
	)
}

// loggableEmail normalizuje email kao servis i uklanja kontrolne znakove
// (sprečava ubacivanje lažnih redova u log), pa ograničava dužinu.
func loggableEmail(email string) string {
	e := service.NormalizeEmail(email)
	e = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, e)
	if len(e) > maxLoggedEmailLen {
		e = e[:maxLoggedEmailLen]
	}
	return e
}

// clientIP vraća IP klijenta. Servisu se pristupa preko API gateway-a, pa je
// RemoteAddr adresa gateway-a; prvi element X-Forwarded-For je adresa klijenta,
// ali samo ako ga postavlja gateway kao jedina ulazna tačka. Vrednost koja
// nije ispravna IP adresa se odbacuje (nema slobodnog teksta u logu).
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first, _, _ := strings.Cut(xff, ",")
		if ip := net.ParseIP(strings.TrimSpace(first)); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return "unknown"
}
