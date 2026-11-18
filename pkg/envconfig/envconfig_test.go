package envconfig

import (
	"strings"
	"testing"
	"time"
)

func TestRequiredMissing(t *testing.T) {
	for _, value := range []string{"", "   "} {
		t.Setenv("TEST_DB_URI", value)
		env := New()
		if got := env.Required("TEST_DB_URI"); got != "" {
			t.Fatalf("Required = %q, want prazan string", got)
		}
		err := env.Err()
		if err == nil {
			t.Fatalf("vrednost %q: Err = nil, want greska", value)
		}
		if !strings.Contains(err.Error(), "TEST_DB_URI") {
			t.Fatalf("greska %q ne navodi ime promenljive", err)
		}
	}
}

func TestRequiredPresent(t *testing.T) {
	t.Setenv("TEST_DB_URI", "  mongodb://db:27017  ")
	env := New()
	if got := env.Required("TEST_DB_URI"); got != "mongodb://db:27017" {
		t.Fatalf("Required = %q", got)
	}
	if err := env.Err(); err != nil {
		t.Fatalf("Err = %v, want nil", err)
	}
}

func TestAllMissingReportedAtOnce(t *testing.T) {
	t.Setenv("TEST_A", "")
	t.Setenv("TEST_B", "")
	t.Setenv("TEST_C", "nije-broj")
	env := New()
	env.Required("TEST_A")
	env.Required("TEST_B")
	env.Int("TEST_C", 1)
	err := env.Err()
	if err == nil {
		t.Fatal("Err = nil, want greska")
	}
	for _, key := range []string{"TEST_A", "TEST_B", "TEST_C"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("greska %q ne navodi %s", err, key)
		}
	}
}

func TestErrorDoesNotLeakValue(t *testing.T) {
	const secret = "tajna-lozinka-123"
	t.Setenv("TEST_TIMEOUT", secret)
	t.Setenv("TEST_PORT", secret)
	t.Setenv("TEST_FLAG", secret)
	env := New()
	env.Duration("TEST_TIMEOUT", time.Second)
	env.Int("TEST_PORT", 1)
	env.Bool("TEST_FLAG", false)
	err := env.Err()
	if err == nil {
		t.Fatal("Err = nil, want greska")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("greska otkriva vrednost: %q", err)
	}
}

func TestString(t *testing.T) {
	tests := []struct{ name, value, want string }{
		{"podrazumevano", "", "8083"},
		{"samo razmaci", "  ", "8083"},
		{"zadata vrednost", "9000", "9000"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TEST_PORT", tc.value)
			env := New()
			if got := env.String("TEST_PORT", "8083"); got != tc.want {
				t.Fatalf("String = %q, want %q", got, tc.want)
			}
			if err := env.Err(); err != nil {
				t.Fatalf("Err = %v, want nil", err)
			}
		})
	}
}

func TestInt(t *testing.T) {
	tests := []struct {
		name, value string
		want        int
		wantErr     bool
	}{
		{"podrazumevano", "", 5, false},
		{"zadata vrednost", "10", 10, false},
		{"negativna", "-3", -3, false},
		{"nije broj", "abc", 5, true},
		{"decimalni", "1.5", 5, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TEST_N", tc.value)
			env := New()
			if got := env.Int("TEST_N", 5); got != tc.want {
				t.Fatalf("Int = %d, want %d", got, tc.want)
			}
			if err := env.Err(); (err != nil) != tc.wantErr {
				t.Fatalf("Err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestDuration(t *testing.T) {
	tests := []struct {
		name, value string
		want        time.Duration
		wantErr     bool
	}{
		{"podrazumevano", "", 5 * time.Second, false},
		{"zadata vrednost", "24h", 24 * time.Hour, false},
		{"bez jedinice", "10", 5 * time.Second, true},
		{"nula", "0s", 5 * time.Second, true},
		{"negativno", "-1m", 5 * time.Second, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TEST_TIMEOUT", tc.value)
			env := New()
			if got := env.Duration("TEST_TIMEOUT", 5*time.Second); got != tc.want {
				t.Fatalf("Duration = %v, want %v", got, tc.want)
			}
			if err := env.Err(); (err != nil) != tc.wantErr {
				t.Fatalf("Err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestBool(t *testing.T) {
	tests := []struct {
		name, value string
		want        bool
		wantErr     bool
	}{
		{"podrazumevano", "", true, false},
		{"false", "false", false, false},
		{"nula", "0", false, false},
		{"neispravno", "mozda", true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TEST_FLAG", tc.value)
			env := New()
			if got := env.Bool("TEST_FLAG", true); got != tc.want {
				t.Fatalf("Bool = %v, want %v", got, tc.want)
			}
			if err := env.Err(); (err != nil) != tc.wantErr {
				t.Fatalf("Err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestNoErrorsWhenValid(t *testing.T) {
	t.Setenv("TEST_DB_URI", "mongodb://db:27017")
	t.Setenv("TEST_TIMEOUT", "")
	env := New()
	env.Required("TEST_DB_URI")
	env.Duration("TEST_TIMEOUT", time.Second)
	env.String("TEST_PORT_UNSET", "8083")
	if err := env.Err(); err != nil {
		t.Fatalf("Err = %v, want nil", err)
	}
}
