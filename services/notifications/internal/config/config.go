// Package config ucitava konfiguraciju notifications servisa iz env promenljivih (US-31).
package config

import "github.com/stefanbrmbota/file-storage-platform/pkg/envconfig"

// Config su podesavanja servisa. Ostale promenljive iz .env.example se dodaju
// ovde kad ih servis bude koristio.
type Config struct {
	Port string
}

// Load cita konfiguraciju; vraca gresku ako neka vrednost nije ispravna.
func Load() (Config, error) {
	env := envconfig.New()
	cfg := Config{
		Port: env.String("NOTIFICATIONS_PORT", "8083"),
	}
	return cfg, env.Err()
}
