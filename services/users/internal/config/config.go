package config

import (
	"errors"
	"os"
)

// Config sadrzi podesavanja Users servisa. Vrednosti dolaze iskljucivo iz
// env promenljivih, bez zakucanih adresa i lozinki.
type Config struct {
	Port          string
	MongoURI      string
	MongoDB       string
	AdminEmail    string
	AdminPassword string // nikad se ne loguje
}

// Load cita konfiguraciju. Vraca gresku ako je podeseno samo jedno od
// ADMIN_EMAIL i ADMIN_PASSWORD.
func Load() (Config, error) {
	cfg := Config{
		Port:          getEnv("USERS_PORT", "8081"),
		MongoURI:      getEnv("USERS_MONGO_URI", "mongodb://localhost:27017"),
		MongoDB:       getEnv("USERS_MONGO_DB", "users"),
		AdminEmail:    os.Getenv("ADMIN_EMAIL"),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),
	}
	if (cfg.AdminEmail == "") != (cfg.AdminPassword == "") {
		return Config{}, errors.New("config: ADMIN_EMAIL i ADMIN_PASSWORD moraju biti postavljeni zajedno")
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
