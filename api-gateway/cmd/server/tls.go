package main

import (
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/config"
)

// tlsConfig je TLS gateway-a (US-67): najmanje TLS 1.2, a za TLS 1.2 samo
// sifre sa forward secrecy (ECDHE) i AEAD (GCM/ChaCha20). TLS 1.3 sifre Go
// bira sam i sve su bezbedne.
func tlsConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
		},
	}
}

// listen pokrece server: preko HTTPS-a ako je TLS podesen, inace HTTP.
func listen(srv *http.Server, t config.TLS) error {
	if !t.Enabled() {
		return srv.ListenAndServe()
	}
	srv.TLSConfig = tlsConfig()
	return srv.ListenAndServeTLS(t.CertFile, t.KeyFile)
}

// startHealthServer pokrece HTTP server samo za GET /health na 127.0.0.1,
// za healthcheck u kontejneru dok glavni port radi preko TLS-a. Slusa samo
// na loopback adresi, pa nije dostupan ni iz Docker mreze ni spolja.
func startHealthServer(port string, logger *slog.Logger) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	srv := &http.Server{
		Addr:              net.JoinHostPort("127.0.0.1", port),
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("health server failed", "err", err)
		}
	}()
	return srv
}
