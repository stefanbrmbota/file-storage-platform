package config

import (
	"testing"
	"time"
)

func TestLoadDefaultQuota(t *testing.T) {
	tests := []struct {
		name, value string
		want        int64
		wantErr     bool
	}{
		{"podrazumevano 1 GiB", "", 1 << 30, false},
		{"zadata vrednost", "5368709120", 5 << 30, false},
		{"nula", "0", 0, true},
		{"negativna", "-1", 0, true},
		{"nije broj", "abc", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ADMIN_EMAIL", "")
			t.Setenv("ADMIN_PASSWORD", "")
			t.Setenv("USERS_DEFAULT_QUOTA_BYTES", tc.value)
			cfg, err := Load()
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && cfg.DefaultQuotaBytes != tc.want {
				t.Fatalf("DefaultQuotaBytes = %d, want %d", cfg.DefaultQuotaBytes, tc.want)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name, email, pass string
		wantErr, wantAdm  bool
	}{
		{"bez admina", "", "", false, false},
		{"oba postavljena", "admin@example.com", "tajna", false, true},
		{"samo email", "admin@example.com", "", true, false},
		{"samo lozinka", "", "tajna", true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ADMIN_EMAIL", tc.email)
			t.Setenv("ADMIN_PASSWORD", tc.pass)
			cfg, err := Load()
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && cfg.AdminConfigured() != tc.wantAdm {
				t.Fatalf("AdminConfigured = %v, want %v", cfg.AdminConfigured(), tc.wantAdm)
			}
		})
	}
}

func TestLoadConfirmationSettings(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "")
	t.Setenv("ADMIN_PASSWORD", "")

	t.Run("podrazumevane vrednosti", func(t *testing.T) {
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.ConfirmationTTL != 24*time.Hour || cfg.SMTPPort != "1025" || cfg.FilesServiceURL != "http://localhost:8082" {
			t.Errorf("neočekivane podrazumevane vrednosti: %+v", cfg)
		}
	})
	t.Run("zadat rok", func(t *testing.T) {
		t.Setenv("CONFIRMATION_TTL", "30m")
		cfg, err := Load()
		if err != nil || cfg.ConfirmationTTL != 30*time.Minute {
			t.Fatalf("ttl = %v, err = %v", cfg.ConfirmationTTL, err)
		}
	})
	for _, bad := range []string{"abc", "0s", "-1h"} {
		t.Run("neispravan rok "+bad, func(t *testing.T) {
			t.Setenv("CONFIRMATION_TTL", bad)
			if _, err := Load(); err == nil {
				t.Fatal("očekivana greška")
			}
		})
	}
	t.Run("SMTP_USER bez lozinke", func(t *testing.T) {
		t.Setenv("SMTP_USER", "u")
		t.Setenv("SMTP_PASSWORD", "")
		if _, err := Load(); err == nil {
			t.Fatal("očekivana greška")
		}
	})
}

func TestLoadPasswordMinAge(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "")
	t.Setenv("ADMIN_PASSWORD", "")

	t.Run("podrazumevano 24h", func(t *testing.T) {
		t.Setenv("PASSWORD_MIN_AGE", "")
		cfg, err := Load()
		if err != nil || cfg.PasswordMinAge != 24*time.Hour {
			t.Fatalf("PasswordMinAge = %v, err = %v", cfg.PasswordMinAge, err)
		}
	})
	t.Run("iz env (skracen period za demonstraciju)", func(t *testing.T) {
		t.Setenv("PASSWORD_MIN_AGE", "1m")
		cfg, err := Load()
		if err != nil || cfg.PasswordMinAge != time.Minute {
			t.Fatalf("PasswordMinAge = %v, err = %v", cfg.PasswordMinAge, err)
		}
	})
	t.Run("neispravno", func(t *testing.T) {
		for _, bad := range []string{"abc", "0s", "-1h"} {
			t.Setenv("PASSWORD_MIN_AGE", bad)
			if _, err := Load(); err == nil {
				t.Errorf("%q: ocekivana greska", bad)
			}
		}
	})
}

func TestLoadJWTSettings(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "")
	t.Setenv("ADMIN_PASSWORD", "")

	t.Run("podrazumevano", func(t *testing.T) {
		t.Setenv("JWT_SECRET", "")
		t.Setenv("JWT_TTL", "")
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.JWTTTL != 15*time.Minute {
			t.Errorf("JWTTTL = %v, ocekivano 15m", cfg.JWTTTL)
		}
		if cfg.JWTSecret != "" {
			t.Error("JWTSecret ne sme imati podrazumevanu vrednost")
		}
	})
	t.Run("iz env", func(t *testing.T) {
		t.Setenv("JWT_SECRET", "tajna")
		t.Setenv("JWT_TTL", "1h")
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.JWTSecret != "tajna" || cfg.JWTTTL != time.Hour {
			t.Errorf("neocekivano: %v %v", cfg.JWTSecret, cfg.JWTTTL)
		}
	})
	t.Run("los TTL", func(t *testing.T) {
		t.Setenv("JWT_TTL", "abc")
		if _, err := Load(); err == nil {
			t.Error("ocekivana greska")
		}
	})
}

func TestLoadInternalToken(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "")
	t.Setenv("ADMIN_PASSWORD", "")

	t.Setenv("INTERNAL_TOKEN", "")
	cfg, err := Load()
	if err != nil || cfg.InternalToken != "" {
		t.Fatalf("bez env: %v, %q (nema podrazumevane vrednosti)", err, cfg.InternalToken)
	}
	t.Setenv("INTERNAL_TOKEN", "neka-tajna")
	cfg, err = Load()
	if err != nil || cfg.InternalToken != "neka-tajna" {
		t.Fatalf("iz env: %v, %q", err, cfg.InternalToken)
	}
}

func TestLoadOTPMaxAttempts(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "")
	t.Setenv("ADMIN_PASSWORD", "")

	t.Setenv("OTP_MAX_ATTEMPTS", "")
	cfg, err := Load()
	if err != nil || cfg.OTPMaxAttempts != 5 {
		t.Fatalf("podrazumevano: %v, %d", err, cfg.OTPMaxAttempts)
	}
	t.Setenv("OTP_MAX_ATTEMPTS", "3")
	cfg, err = Load()
	if err != nil || cfg.OTPMaxAttempts != 3 {
		t.Fatalf("iz env: %v, %d", err, cfg.OTPMaxAttempts)
	}
	for _, bad := range []string{"0", "-1", "abc"} {
		t.Setenv("OTP_MAX_ATTEMPTS", bad)
		if _, err := Load(); err == nil {
			t.Errorf("%q: ocekivana greska", bad)
		}
	}
}
