package config

import "testing"

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
