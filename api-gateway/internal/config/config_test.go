package config

import (
	"strings"
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	for _, k := range []string{"GATEWAY_PORT", "GATEWAY_UPSTREAM_TIMEOUT", "USERS_SERVICE_URL",
		"FILES_SERVICE_URL", "NOTIFICATIONS_SERVICE_URL", "SHARING_SERVICE_URL", "ANALYTICS_SERVICE_URL"} {
		t.Setenv(k, "")
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "8080" || cfg.UpstreamTimeout != 10*time.Second {
		t.Errorf("port=%q timeout=%v", cfg.Port, cfg.UpstreamTimeout)
	}
	if cfg.UsersURL.String() != "http://localhost:8081" || cfg.AnalyticsURL.String() != "http://localhost:8085" {
		t.Errorf("users=%s analytics=%s", cfg.UsersURL, cfg.AnalyticsURL)
	}
}

func TestServiceURLFromEnv(t *testing.T) {
	t.Setenv("USERS_SERVICE_URL", "http://users:8081/")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UsersURL.String() != "http://users:8081" {
		t.Errorf("users=%s", cfg.UsersURL)
	}
}

func TestInvalidServiceURL(t *testing.T) {
	for _, raw := range []string{"users:8081", "ftp://users:8081", "http://", "http://users:8081/api", "http://u:p@users:8081"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("FILES_SERVICE_URL", raw)
			_, err := Load()
			if err == nil {
				t.Fatal("ocekivana greska")
			}
			if !strings.Contains(err.Error(), "FILES_SERVICE_URL") {
				t.Errorf("greska ne navodi promenljivu: %v", err)
			}
			if strings.Contains(err.Error(), "p@") {
				t.Errorf("greska otkriva vrednost: %v", err)
			}
		})
	}
}

func TestInvalidTimeout(t *testing.T) {
	t.Setenv("GATEWAY_UPSTREAM_TIMEOUT", "abc")
	if _, err := Load(); err == nil {
		t.Fatal("ocekivana greska")
	}
}
