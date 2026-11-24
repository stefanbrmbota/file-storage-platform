package config

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func clearEnv(t *testing.T) {
	for _, k := range []string{"NOTIFICATIONS_PORT", "NOTIFICATIONS_CASSANDRA_HOSTS", "NOTIFICATIONS_CASSANDRA_KEYSPACE",
		"NOTIFICATIONS_CASSANDRA_REPLICATION_FACTOR", "NOTIFICATIONS_CASSANDRA_TIMEOUT", "NOTIFICATIONS_CASSANDRA_STARTUP_TIMEOUT"} {
		t.Setenv(k, "")
	}
}

func TestDefaults(t *testing.T) {
	clearEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := Cassandra{
		Hosts: []string{"localhost:9042"}, Keyspace: "notifications", ReplicationFactor: 1,
		Timeout: 5 * time.Second, StartupTimeout: 60 * time.Second,
	}
	if cfg.Port != "8083" || !reflect.DeepEqual(cfg.Cassandra, want) {
		t.Errorf("cfg = %+v", cfg)
	}
}

func TestHostsList(t *testing.T) {
	clearEnv(t)
	t.Setenv("NOTIFICATIONS_CASSANDRA_HOSTS", " cassandra-0:9042, ,cassandra-1:9042 ")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"cassandra-0:9042", "cassandra-1:9042"}; !reflect.DeepEqual(cfg.Cassandra.Hosts, want) {
		t.Errorf("hosts = %v", cfg.Cassandra.Hosts)
	}
}

func TestInvalidValues(t *testing.T) {
	cases := map[string]struct{ key, value string }{
		"prazni hostovi":          {"NOTIFICATIONS_CASSANDRA_HOSTS", " , "},
		"keyspace sa crticom":     {"NOTIFICATIONS_CASSANDRA_KEYSPACE", "moj-keyspace"},
		"keyspace sa CQL-om":      {"NOTIFICATIONS_CASSANDRA_KEYSPACE", "x; DROP KEYSPACE system"},
		"keyspace pocinje cifrom": {"NOTIFICATIONS_CASSANDRA_KEYSPACE", "1notif"},
		"faktor nula":             {"NOTIFICATIONS_CASSANDRA_REPLICATION_FACTOR", "0"},
		"faktor nije broj":        {"NOTIFICATIONS_CASSANDRA_REPLICATION_FACTOR", "tri"},
		"timeout":                 {"NOTIFICATIONS_CASSANDRA_TIMEOUT", "abc"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv(c.key, c.value)
			_, err := Load()
			if err == nil {
				t.Fatal("ocekivana greska")
			}
			if !strings.Contains(err.Error(), c.key) {
				t.Errorf("greska ne navodi %s: %v", c.key, err)
			}
		})
	}
}
