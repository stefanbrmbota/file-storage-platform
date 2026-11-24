// Package config ucitava konfiguraciju notifications servisa iz env promenljivih (US-31).
package config

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/pkg/envconfig"
)

// Config su podesavanja servisa. Ostale promenljive iz .env.example se dodaju
// ovde kad ih servis bude koristio.
type Config struct {
	Port string

	Cassandra Cassandra
}

// Cassandra su podesavanja konekcije na Cassandru (design 3.2, T-07.1).
type Cassandra struct {
	// Hosts su adrese cvorova (host:port).
	Hosts []string
	// Keyspace je ime keyspace-a; servis ga pravi pri pokretanju ako ne postoji.
	Keyspace string
	// ReplicationFactor je faktor replikacije keyspace-a (lokalno 1, u K8s 3).
	ReplicationFactor int
	// Timeout je najduze cekanje na jedan upit ka bazi.
	Timeout time.Duration
	// StartupTimeout je koliko dugo servis pri pokretanju pokusava da se
	// poveze, jer Cassandra se sporo podize.
	StartupTimeout time.Duration
}

// keyspacePattern je dozvoljeno ime keyspace-a. Ime ne moze biti parametar
// CQL upita, pa se sme koristiti u DDL-u samo ako prodje ovu proveru.
var keyspacePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,47}$`)

// Load cita konfiguraciju; vraca gresku ako neka vrednost nije ispravna.
func Load() (Config, error) {
	env := envconfig.New()
	cfg := Config{
		Port: env.String("NOTIFICATIONS_PORT", "8083"),
		Cassandra: Cassandra{
			Hosts:             splitHosts(env.String("NOTIFICATIONS_CASSANDRA_HOSTS", "localhost:9042")),
			Keyspace:          env.String("NOTIFICATIONS_CASSANDRA_KEYSPACE", "notifications"),
			ReplicationFactor: env.Int("NOTIFICATIONS_CASSANDRA_REPLICATION_FACTOR", 1),
			Timeout:           env.Duration("NOTIFICATIONS_CASSANDRA_TIMEOUT", 5*time.Second),
			StartupTimeout:    env.Duration("NOTIFICATIONS_CASSANDRA_STARTUP_TIMEOUT", 60*time.Second),
		},
	}

	errs := []error{env.Err()}
	if len(cfg.Cassandra.Hosts) == 0 {
		errs = append(errs, errors.New("config: NOTIFICATIONS_CASSANDRA_HOSTS mora sadrzati bar jednu adresu"))
	}
	if !keyspacePattern.MatchString(cfg.Cassandra.Keyspace) {
		errs = append(errs, errors.New("config: NOTIFICATIONS_CASSANDRA_KEYSPACE sme sadrzati samo mala slova, cifre i _, i mora pocinjati slovom"))
	}
	if cfg.Cassandra.ReplicationFactor < 1 {
		errs = append(errs, errors.New("config: NOTIFICATIONS_CASSANDRA_REPLICATION_FACTOR mora biti bar 1"))
	}
	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// splitHosts deli listu adresa razdvojenu zarezima i uklanja prazne stavke.
func splitHosts(raw string) []string {
	var out []string
	for _, h := range strings.Split(raw, ",") {
		if h = strings.TrimSpace(h); h != "" {
			out = append(out, h)
		}
	}
	return out
}
