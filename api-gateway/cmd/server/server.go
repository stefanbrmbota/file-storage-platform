package main

import (
	"net/http"

	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/config"
)

// newServer pravi HTTP server gateway-a (T-30.6).
//
// ReadTimeout i WriteTimeout namerno nisu postavljeni (0 = bez limita): oni
// ogranicavaju ceo zahtev, ukljucujuci prenos tela, pa bi prekinuli
// otpremanje ili preuzimanje velike datoteke koje traje duze od limita.
// Trajanje prenosa ogranicavaju sami servisi (npr. Files ima svoj
// ReadTimeout za otpremanje), a gateway prekida zahtev cim servis zatvori vezu.
//
// Zastita od sporih klijenata ostaje:
//   - ReadHeaderTimeout: klijent mora da posalje zaglavlja na vreme (slowloris);
//   - IdleTimeout: neaktivna keep-alive veza se zatvara;
//   - GATEWAY_UPSTREAM_TIMEOUT (proxy): najduze cekanje na odgovor servisa.
func newServer(cfg config.Config, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           h,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}
}

// maxHeaderBytes je granica velicine zaglavlja; JWT i ostali headeri su
// daleko manji, pa 64 KB ostavlja dovoljno mesta i sece preterane zahteve.
const maxHeaderBytes = 64 << 10
