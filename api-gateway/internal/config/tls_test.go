package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTLSDisabledByDefault(t *testing.T) {
	t.Setenv("GATEWAY_TLS_CERT_FILE", "")
	t.Setenv("GATEWAY_TLS_KEY_FILE", "")
	t.Setenv("GATEWAY_HEALTH_PORT", "")
	tc, err := LoadTLS()
	if err != nil || tc.Enabled() || tc.HealthPort != "8079" {
		t.Fatalf("err=%v enabled=%v health=%q", err, tc.Enabled(), tc.HealthPort)
	}
}

func TestTLSNeedsBothFiles(t *testing.T) {
	t.Setenv("GATEWAY_TLS_CERT_FILE", "/certs/localhost.crt")
	t.Setenv("GATEWAY_TLS_KEY_FILE", "")
	if _, err := LoadTLS(); err == nil || !strings.Contains(err.Error(), "zajedno") {
		t.Fatalf("ocekivana greska, dobijeno %v", err)
	}
}

func TestTLSFilesMustExist(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GATEWAY_TLS_CERT_FILE", filepath.Join(dir, "nema.crt"))
	t.Setenv("GATEWAY_TLS_KEY_FILE", filepath.Join(dir, "nema.key"))
	if _, err := LoadTLS(); err == nil {
		t.Fatal("ocekivana greska za nepostojece fajlove")
	}

	crt, key := filepath.Join(dir, "a.crt"), filepath.Join(dir, "a.key")
	_ = os.WriteFile(crt, []byte("x"), 0o644)
	_ = os.WriteFile(key, []byte("x"), 0o644)
	t.Setenv("GATEWAY_TLS_CERT_FILE", crt)
	t.Setenv("GATEWAY_TLS_KEY_FILE", key)
	tc, err := LoadTLS()
	if err != nil || !tc.Enabled() {
		t.Fatalf("err=%v enabled=%v", err, tc.Enabled())
	}
}
