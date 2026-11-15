package config

import "testing"

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
