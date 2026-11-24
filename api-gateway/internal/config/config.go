// Package config ucitava konfiguraciju gateway servisa iz env promenljivih (US-31).
package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/cors"
	"github.com/stefanbrmbota/file-storage-platform/pkg/envconfig"
)

// Config su podesavanja servisa. Ostale promenljive iz .env.example se dodaju
// ovde kad ih servis bude koristio.
type Config struct {
	Port string

	// UpstreamTimeout je najduze vreme cekanja odgovora servisa (design 5.3, tacka 5).
	UpstreamTimeout time.Duration

	// ReadHeaderTimeout je rok da klijent posalje zaglavlja zahteva, a
	// IdleTimeout koliko neaktivna keep-alive veza ostaje otvorena (T-30.6).
	// Ukupno trajanje zahteva nije ograniceno, da se ne bi prekidao prenos
	// velikih datoteka.
	ReadHeaderTimeout time.Duration
	IdleTimeout       time.Duration

	// Adrese servisa u internoj mrezi (design 5.3, tacka 6).
	UsersURL         *url.URL
	FilesURL         *url.URL
	NotificationsURL *url.URL
	SharingURL       *url.URL
	AnalyticsURL     *url.URL

	// CORSAllowedOrigins su origin-i klijentske aplikacije kojima pretrazivac
	// sme da salje zahteve (T-30.3), npr. http://localhost:3000.
	CORSAllowedOrigins []string

	// JWTSecret je ista tajna kao JWT_SECRET u Users servisu (HS256, T-30.5).
	// Nikad se ne loguje.
	JWTSecret string
	// AuthTimeout je najduze cekanje na Users servis pri proveri opozvanog tokena.
	AuthTimeout time.Duration
}

// MinJWTSecretLen je najmanja duzina tajne (HS256 trazi bar 256 bita), ista kao u Users servisu.
const MinJWTSecretLen = 32

// Load cita konfiguraciju; vraca gresku ako neka vrednost nije ispravna.
// Podrazumevane adrese su iste kao u .env.example (lokalni rad bez Docker-a).
func Load() (Config, error) {
	env := envconfig.New()
	cfg := Config{
		Port:              env.String("GATEWAY_PORT", "8080"),
		UpstreamTimeout:   env.Duration("GATEWAY_UPSTREAM_TIMEOUT", 10*time.Second),
		ReadHeaderTimeout: env.Duration("GATEWAY_READ_HEADER_TIMEOUT", 5*time.Second),
		IdleTimeout:       env.Duration("GATEWAY_IDLE_TIMEOUT", 60*time.Second),
		JWTSecret:         env.Required("JWT_SECRET"),
		AuthTimeout:       env.Duration("GATEWAY_AUTH_TIMEOUT", 5*time.Second),
	}

	var errs []error
	parse := func(key, fallback string) *url.URL {
		u, err := parseServiceURL(key, env.String(key, fallback))
		if err != nil {
			errs = append(errs, err)
		}
		return u
	}
	cfg.UsersURL = parse("USERS_SERVICE_URL", "http://localhost:8081")
	cfg.FilesURL = parse("FILES_SERVICE_URL", "http://localhost:8082")
	cfg.NotificationsURL = parse("NOTIFICATIONS_SERVICE_URL", "http://localhost:8083")
	cfg.SharingURL = parse("SHARING_SERVICE_URL", "http://localhost:8084")
	cfg.AnalyticsURL = parse("ANALYTICS_SERVICE_URL", "http://localhost:8085")

	cfg.CORSAllowedOrigins = cors.ParseOrigins(env.String("CORS_ALLOWED_ORIGINS", "http://localhost:3000"))
	for _, o := range cfg.CORSAllowedOrigins {
		if !validOrigin(o) {
			errs = append(errs, errors.New("config: CORS_ALLOWED_ORIGINS mora biti lista adresa oblika http://host:port, bez *"))
			break
		}
	}

	if cfg.JWTSecret != "" && len(cfg.JWTSecret) < MinJWTSecretLen {
		errs = append(errs, fmt.Errorf("config: JWT_SECRET mora imati bar %d karaktera", MinJWTSecretLen))
	}

	if err := errors.Join(append([]error{env.Err()}, errs...)...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// parseServiceURL prihvata samo apsolutnu http(s) adresu bez putanje, npr.
// http://users:8081. Poruka o gresci sadrzi samo ime promenljive.
func parseServiceURL(key, raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.User != nil {
		return nil, fmt.Errorf("config: %s mora biti adresa oblika http://host:port", key)
	}
	u.Path = ""
	return u, nil
}

// validOrigin prihvata origin oblika scheme://host[:port], bez putanje i bez "*".
func validOrigin(o string) bool {
	u, err := url.Parse(o)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" &&
		u.Path == "" && u.RawQuery == "" && u.User == nil && !strings.Contains(o, "*")
}
