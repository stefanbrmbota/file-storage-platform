package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

const testSecret = "test-secret-test-secret-test-secret-123"

// TestMain postavlja obaveznu tajnu za sve testove; pojedinacni testovi je menjaju preko t.Setenv.
func TestMain(m *testing.M) {
	_ = os.Setenv("JWT_SECRET", testSecret)
	os.Exit(m.Run())
}

func TestDefaults(t *testing.T) {
	for _, k := range []string{"GATEWAY_PORT", "GATEWAY_UPSTREAM_TIMEOUT", "USERS_SERVICE_URL",
		"FILES_SERVICE_URL", "NOTIFICATIONS_SERVICE_URL", "SHARING_SERVICE_URL", "ANALYTICS_SERVICE_URL",
		"CORS_ALLOWED_ORIGINS"} {
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
	if len(cfg.CORSAllowedOrigins) != 1 || cfg.CORSAllowedOrigins[0] != "http://localhost:3000" {
		t.Errorf("cors=%v", cfg.CORSAllowedOrigins)
	}
	if cfg.JWTSecret != testSecret || cfg.AuthTimeout != 5*time.Second {
		t.Errorf("auth timeout=%v", cfg.AuthTimeout)
	}
	if cfg.ReadHeaderTimeout != 5*time.Second || cfg.IdleTimeout != 60*time.Second {
		t.Errorf("read header=%v idle=%v", cfg.ReadHeaderTimeout, cfg.IdleTimeout)
	}
}

func TestJWTSecretRequired(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("ocekivana greska za JWT_SECRET, dobijeno %v", err)
	}
}

func TestJWTSecretTooShort(t *testing.T) {
	t.Setenv("JWT_SECRET", "kratka-tajna")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("ocekivana greska za JWT_SECRET, dobijeno %v", err)
	}
	if strings.Contains(err.Error(), "kratka-tajna") {
		t.Errorf("greska otkriva tajnu: %v", err)
	}
}

func TestCORSOrigins(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:3000, https://app.example.com/")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.CORSAllowedOrigins) != 2 || cfg.CORSAllowedOrigins[1] != "https://app.example.com" {
		t.Errorf("cors=%v", cfg.CORSAllowedOrigins)
	}
}

func TestInvalidCORSOrigin(t *testing.T) {
	for _, raw := range []string{"*", "localhost:3000", "http://localhost:3000/app", "https://*.example.com"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("CORS_ALLOWED_ORIGINS", raw)
			if _, err := Load(); err == nil {
				t.Fatal("ocekivana greska")
			}
		})
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

func TestServerTimeoutsFromEnv(t *testing.T) {
	t.Setenv("GATEWAY_READ_HEADER_TIMEOUT", "2s")
	t.Setenv("GATEWAY_IDLE_TIMEOUT", "2m")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ReadHeaderTimeout != 2*time.Second || cfg.IdleTimeout != 2*time.Minute {
		t.Errorf("read header=%v idle=%v", cfg.ReadHeaderTimeout, cfg.IdleTimeout)
	}
}

func TestInvalidServerTimeout(t *testing.T) {
	for _, k := range []string{"GATEWAY_READ_HEADER_TIMEOUT", "GATEWAY_IDLE_TIMEOUT"} {
		t.Run(k, func(t *testing.T) {
			t.Setenv(k, "0s")
			if _, err := Load(); err == nil {
				t.Fatal("ocekivana greska za nepozitivan timeout")
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
