package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewUserAlwaysHasUserRole(t *testing.T) {
	u := NewUser("1", "Pera", "Peric", "pera@example.com", "hash", time.Now())
	if u.Role != RoleUser {
		t.Fatalf("očekivana uloga %q, dobijena %q", RoleUser, u.Role)
	}
}

// Zahtev za registraciju ne sme moci da zada ulogu: DTO nema polje role,
// pa se "role":"ADMIN" iz JSON-a ignorise, a NewUser uvek daje USER.
func TestRegistrationRequestCannotSetRole(t *testing.T) {
	var req struct {
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
		Email     string `json:"email"`
		Password  string `json:"password"`
	}
	body := `{"firstName":"Pera","lastName":"Peric","email":"pera@example.com","password":"x","role":"ADMIN"}`
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatal(err)
	}
	u := NewUser("1", req.FirstName, req.LastName, req.Email, "hash", time.Now())
	if u.Role != RoleUser {
		t.Fatalf("uloga se ne sme zadati zahtevom, dobijena %q", u.Role)
	}
}

func TestRoleValid(t *testing.T) {
	cases := map[Role]bool{RoleUser: true, RoleAdmin: true, "": false, "user": false, "ROOT": false}
	for r, want := range cases {
		if got := r.Valid(); got != want {
			t.Errorf("Role(%q).Valid() = %v, očekivano %v", r, got, want)
		}
	}
}

func TestPasswordHashNotSerialized(t *testing.T) {
	u := NewUser("1", "Pera", "Peric", "pera@example.com", "tajna-hes", time.Now())
	b, _ := json.Marshal(u)
	if string(b) == "" || contains(string(b), "tajna-hes") {
		t.Fatalf("hes lozinke ne sme u JSON: %s", b)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestNewUserDefaults(t *testing.T) {
	now := time.Now()
	u := NewUser("1", "Pera", "Peric", "pera@example.com", "hash", now)
	if !u.Active {
		t.Error("novi korisnik treba da bude aktivan")
	}
	if u.QuotaBytes != DefaultQuotaBytes || DefaultQuotaBytes != 1<<30 {
		t.Errorf("kvota = %d, očekivano 1 GiB", u.QuotaBytes)
	}
	if u.UsedBytes != 0 {
		t.Errorf("usedBytes = %d, očekivano 0", u.UsedBytes)
	}
	if !u.PasswordChangedAt.Equal(now) {
		t.Error("passwordChangedAt treba da bude trenutak kreiranja")
	}
}
