package token

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
)

const testSecret = "test-secret-0123456789-0123456789"

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	m, err := NewManager(testSecret, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestNewManager_Validates(t *testing.T) {
	if _, err := NewManager("kratka", time.Minute); err == nil {
		t.Error("ocekivana greska za kratku tajnu")
	}
	if _, err := NewManager(testSecret, 0); err == nil {
		t.Error("ocekivana greska za nepozitivno trajanje")
	}
}

func TestIssueParse_RoundTrip(t *testing.T) {
	m := newTestManager(t)
	for _, role := range []model.Role{model.RoleUser, model.RoleAdmin} {
		s, err := m.Issue("u1", role)
		if err != nil {
			t.Fatal(err)
		}
		got, err := m.Parse(s)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if got.UserID != "u1" || got.Role != role {
			t.Errorf("got %+v", got)
		}
	}
}

func TestIssue_HasUniqueJTIAndParseReturnsIt(t *testing.T) {
	m := newTestManager(t)
	a, _ := m.Issue("u1", model.RoleUser)
	b, _ := m.Issue("u1", model.RoleUser)

	ca, err := m.Parse(a)
	if err != nil {
		t.Fatal(err)
	}
	cb, _ := m.Parse(b)
	if len(ca.ID) != 32 || ca.ID == cb.ID {
		t.Errorf("jti nije jedinstven 128-bitni: %q %q", ca.ID, cb.ID)
	}
	if ca.ExpiresAt.IsZero() || !ca.ExpiresAt.After(time.Now()) {
		t.Errorf("ExpiresAt = %v", ca.ExpiresAt)
	}
}

func TestIssue_RejectsBadInput(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.Issue("", model.RoleUser); err == nil {
		t.Error("prazan userId mora biti odbijen")
	}
	if _, err := m.Issue("u1", model.Role("ROOT")); err == nil {
		t.Error("nepoznata uloga mora biti odbijena")
	}
}

func TestParse_Expired(t *testing.T) {
	m := newTestManager(t)
	s, _ := m.Issue("u1", model.RoleUser)
	m.now = func() time.Time { return time.Now().Add(16 * time.Minute) }
	if _, err := m.Parse(s); err != ErrInvalidToken {
		t.Errorf("err = %v, ocekivano ErrInvalidToken", err)
	}
}

func TestParse_Tampered(t *testing.T) {
	m := newTestManager(t)
	s, _ := m.Issue("u1", model.RoleUser)
	parts := strings.Split(s, ".")

	// Izmenjen potpis.
	sig := []byte(parts[2])
	if sig[0] == 'A' {
		sig[0] = 'B'
	} else {
		sig[0] = 'A'
	}
	if _, err := m.Parse(parts[0] + "." + parts[1] + "." + string(sig)); err != ErrInvalidToken {
		t.Errorf("izmenjen potpis: err = %v", err)
	}

	// Izmenjen payload (uloga USER -> ADMIN) sa starim potpisom.
	admin, _ := m.Issue("u1", model.RoleAdmin)
	ap := strings.Split(admin, ".")
	if _, err := m.Parse(parts[0] + "." + ap[1] + "." + parts[2]); err != ErrInvalidToken {
		t.Errorf("izmenjen payload: err = %v", err)
	}

	if _, err := m.Parse("nije.jwt.token"); err != ErrInvalidToken {
		t.Errorf("smece: err = %v", err)
	}
	if _, err := m.Parse(""); err != ErrInvalidToken {
		t.Errorf("prazan: err = %v", err)
	}
}

func TestParse_WrongSecret(t *testing.T) {
	m := newTestManager(t)
	other, _ := NewManager("druga-tajna-0123456789-0123456789", time.Minute)
	s, _ := other.Issue("u1", model.RoleUser)
	if _, err := m.Parse(s); err != ErrInvalidToken {
		t.Errorf("err = %v, ocekivano ErrInvalidToken", err)
	}
}

func signRaw(t *testing.T, method jwt.SigningMethod, key any, c claims) string {
	t.Helper()
	s, err := jwt.NewWithClaims(method, c).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func validClaims() claims {
	return claims{
		Role: model.RoleUser,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "u1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
}

func TestParse_RejectsOtherAlgorithms(t *testing.T) {
	m := newTestManager(t)

	none := signRaw(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, validClaims())
	if _, err := m.Parse(none); err != ErrInvalidToken {
		t.Errorf("alg=none: err = %v", err)
	}

	hs512 := signRaw(t, jwt.SigningMethodHS512, []byte(testSecret), validClaims())
	if _, err := m.Parse(hs512); err != ErrInvalidToken {
		t.Errorf("HS512: err = %v", err)
	}
}

func TestParse_RejectsBadClaims(t *testing.T) {
	m := newTestManager(t)
	key := []byte(testSecret)

	noExp := validClaims()
	noExp.ExpiresAt = nil
	if _, err := m.Parse(signRaw(t, jwt.SigningMethodHS256, key, noExp)); err != ErrInvalidToken {
		t.Errorf("bez exp: err = %v", err)
	}

	badRole := validClaims()
	badRole.Role = "ROOT"
	if _, err := m.Parse(signRaw(t, jwt.SigningMethodHS256, key, badRole)); err != ErrInvalidToken {
		t.Errorf("nepoznata uloga: err = %v", err)
	}

	noSub := validClaims()
	noSub.Subject = ""
	if _, err := m.Parse(signRaw(t, jwt.SigningMethodHS256, key, noSub)); err != ErrInvalidToken {
		t.Errorf("bez sub: err = %v", err)
	}
}
