package config

import (
	"errors"
	"os"

	"github.com/stefanbrmbota/file-storage-platform/pkg/envconfig"
)

// TLS su podesavanja HTTPS-a gateway-a (US-67, T-67.2).
//
// Kad su postavljeni GATEWAY_TLS_CERT_FILE i GATEWAY_TLS_KEY_FILE, gateway
// prima samo HTTPS. Bez njih radi preko HTTP-a (lokalni rad bez Docker-a i
// testovi). Postavlja se ili oba ili nijedan.
type TLS struct {
	CertFile string
	KeyFile  string
	// HealthPort je HTTP port samo za GET /health na 127.0.0.1 (healthcheck
	// u kontejneru ne zna za nas sertifikat). Nije dostupan spolja.
	HealthPort string
}

// Enabled kaze da li gateway radi preko HTTPS-a.
func (t TLS) Enabled() bool { return t.CertFile != "" }

// LoadTLS cita TLS podesavanja i proverava da fajlovi postoje, da bi gateway
// pao odmah pri pokretanju, a ne na prvom zahtevu.
func LoadTLS() (TLS, error) {
	env := envconfig.New()
	t := TLS{
		CertFile:   env.String("GATEWAY_TLS_CERT_FILE", ""),
		KeyFile:    env.String("GATEWAY_TLS_KEY_FILE", ""),
		HealthPort: env.String("GATEWAY_HEALTH_PORT", "8079"),
	}
	if err := env.Err(); err != nil {
		return TLS{}, err
	}
	if (t.CertFile == "") != (t.KeyFile == "") {
		return TLS{}, errors.New("config: GATEWAY_TLS_CERT_FILE i GATEWAY_TLS_KEY_FILE se postavljaju zajedno")
	}
	if t.Enabled() {
		for _, f := range []string{t.CertFile, t.KeyFile} {
			if _, err := os.Stat(f); err != nil {
				return TLS{}, errors.New("config: sertifikat ili kljuc za TLS ne postoji (pokreni go run ./cmd/devcert)")
			}
		}
	}
	return t, nil
}
