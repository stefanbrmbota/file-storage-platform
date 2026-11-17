package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// defaultQuotaBytes je podrazumevana kvota novog korisnika (1 GiB).
const defaultQuotaBytes int64 = 1 << 30

// Config sadrzi podesavanja Users servisa. Vrednosti dolaze iskljucivo iz
// env promenljivih, bez zakucanih adresa i lozinki.
type Config struct {
	Port              string
	MongoURI          string
	MongoDB           string
	AdminEmail        string
	AdminPassword     string // nikad se ne loguje
	DefaultQuotaBytes int64  // kvota koju dobija novi nalog

	SMTPHost     string
	SMTPPort     string
	SMTPFrom     string
	SMTPUser     string
	SMTPPassword string // nikad se ne loguje

	ConfirmURLBase  string        // klijentska stranica za potvrdu; token ide u #fragment
	ConfirmationTTL time.Duration // rok vazenja linka za potvrdu
	FilesServiceURL string
	RequestTimeout  time.Duration // timeout za pozive drugih servisa i SMTP-a
}

// Load cita konfiguraciju. Vraca gresku ako je podeseno samo jedno od
// ADMIN_EMAIL i ADMIN_PASSWORD (isto za SMTP_USER i SMTP_PASSWORD), ili ako
// neka numericka vrednost / trajanje nije ispravno.
func Load() (Config, error) {
	cfg := Config{
		Port:              getEnv("USERS_PORT", "8081"),
		MongoURI:          getEnv("USERS_MONGO_URI", "mongodb://localhost:27017"),
		MongoDB:           getEnv("USERS_MONGO_DB", "users"),
		AdminEmail:        os.Getenv("ADMIN_EMAIL"),
		AdminPassword:     os.Getenv("ADMIN_PASSWORD"),
		DefaultQuotaBytes: defaultQuotaBytes,
		SMTPHost:          getEnv("SMTP_HOST", "localhost"),
		SMTPPort:          getEnv("SMTP_PORT", "1025"),
		SMTPFrom:          getEnv("SMTP_FROM", "no-reply@file-storage.local"),
		SMTPUser:          os.Getenv("SMTP_USER"),
		SMTPPassword:      os.Getenv("SMTP_PASSWORD"),
		ConfirmURLBase:    getEnv("CONFIRM_URL_BASE", "http://localhost:3000/confirm"),
		FilesServiceURL:   getEnv("FILES_SERVICE_URL", "http://localhost:8082"),
	}
	if (cfg.AdminEmail == "") != (cfg.AdminPassword == "") {
		return Config{}, errors.New("config: ADMIN_EMAIL i ADMIN_PASSWORD moraju biti postavljeni zajedno")
	}
	if (cfg.SMTPUser == "") != (cfg.SMTPPassword == "") {
		return Config{}, errors.New("config: SMTP_USER i SMTP_PASSWORD moraju biti postavljeni zajedno")
	}
	if v := os.Getenv("USERS_DEFAULT_QUOTA_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return Config{}, errors.New("config: USERS_DEFAULT_QUOTA_BYTES mora biti pozitivan ceo broj")
		}
		cfg.DefaultQuotaBytes = n
	}

	var err error
	if cfg.ConfirmationTTL, err = getDuration("CONFIRMATION_TTL", 24*time.Hour); err != nil {
		return Config{}, err
	}
	if cfg.RequestTimeout, err = getDuration("USERS_REQUEST_TIMEOUT", 5*time.Second); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// AdminConfigured javlja da su podaci za inicijalnog admina zadati.
func (c Config) AdminConfigured() bool {
	return c.AdminEmail != "" && c.AdminPassword != ""
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("config: %s mora biti pozitivno trajanje (npr. 24h)", key)
	}
	return d, nil
}
