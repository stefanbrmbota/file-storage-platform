package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
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
}

// Load cita konfiguraciju. Vraca gresku ako je podeseno samo jedno od
// ADMIN_EMAIL i ADMIN_PASSWORD, ili ako USERS_DEFAULT_QUOTA_BYTES nije
// pozitivan ceo broj.
func Load() (Config, error) {
	cfg := Config{
		Port:              getEnv("USERS_PORT", "8081"),
		MongoURI:          getEnv("USERS_MONGO_URI", "mongodb://localhost:27017"),
		MongoDB:           getEnv("USERS_MONGO_DB", "users"),
		AdminEmail:        os.Getenv("ADMIN_EMAIL"),
		AdminPassword:     os.Getenv("ADMIN_PASSWORD"),
		DefaultQuotaBytes: defaultQuotaBytes,
	}
	if (cfg.AdminEmail == "") != (cfg.AdminPassword == "") {
		return Config{}, errors.New("config: ADMIN_EMAIL i ADMIN_PASSWORD moraju biti postavljeni zajedno")
	}
	if v := os.Getenv("USERS_DEFAULT_QUOTA_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("config: USERS_DEFAULT_QUOTA_BYTES mora biti pozitivan ceo broj")
		}
		cfg.DefaultQuotaBytes = n
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
