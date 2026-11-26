package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
)

type fakeMagicIssuer struct {
	err    error
	calls  int
	id     string
	role   model.Role
	result string
}

func (f *fakeMagicIssuer) Issue(userID string, role model.Role) (string, error) {
	f.calls++
	f.id, f.role = userID, role
	if f.err != nil {
		return "", f.err
	}
	return f.result, nil
}

type magicLoginEnv struct {
	svc    *MagicLoginService
	users  *fakeMagicUsers
	tokens *fakeMagicTokens
	issuer *fakeMagicIssuer
	user   model.User
	raw    string // token iz "poslatog" linka
}

func newMagicLoginEnv(t *testing.T) *magicLoginEnv {
	t.Helper()
	user := model.User{
		ID: "u1", FirstName: "Ana", Email: magicEmail, Active: true, Role: model.RoleAdmin,
		PasswordChangedAt: magicNow.Add(-48 * time.Hour),
	}
	users := &fakeMagicUsers{users: map[string]model.User{magicEmail: user}}
	tokens := newFakeMagicTokens()
	issuer := &fakeMagicIssuer{result: "jwt-za-ana"}
	svc := NewMagicLoginService(users, tokens, issuer)
	svc.now = func() time.Time { return magicNow }

	raw, err := newToken()
	if err != nil {
		t.Fatal(err)
	}
	_ = tokens.Save(context.Background(), model.MagicLinkToken{
		UserID: "u1", TokenHash: hashToken(raw), CreatedAt: magicNow, ExpiresAt: magicNow.Add(10 * time.Minute),
	})
	tokens.saves = 0
	return &magicLoginEnv{svc: svc, users: users, tokens: tokens, issuer: issuer, user: user, raw: raw}
}

func TestMagicLogin_Success(t *testing.T) {
	e := newMagicLoginEnv(t)

	tok, err := e.svc.Login(context.Background(), e.raw)
	if err != nil {
		t.Fatal(err)
	}
	if tok != "jwt-za-ana" {
		t.Errorf("token = %q", tok)
	}
	if e.issuer.id != "u1" || e.issuer.role != model.RoleAdmin {
		t.Errorf("JWT mora nositi ID i ulogu korisnika: %q %v", e.issuer.id, e.issuer.role)
	}
	if len(e.tokens.byUser) != 0 {
		t.Error("iskoriscen token mora biti obrisan")
	}
}

func TestMagicLogin_TokenIsSingleUse(t *testing.T) {
	e := newMagicLoginEnv(t)
	ctx := context.Background()

	if _, err := e.svc.Login(ctx, e.raw); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Login(ctx, e.raw); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("drugi klik: err = %v, ocekivano ErrInvalidToken", err)
	}
	if e.issuer.calls != 1 {
		t.Errorf("JWT izdat %d puta, ocekivano 1", e.issuer.calls)
	}
}

func TestMagicLogin_ExpiredToken(t *testing.T) {
	e := newMagicLoginEnv(t)
	e.svc.now = func() time.Time { return magicNow.Add(10*time.Minute + time.Second) }

	if _, err := e.svc.Login(context.Background(), e.raw); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, ocekivano ErrInvalidToken", err)
	}
	if e.issuer.calls != 0 {
		t.Error("istekao link ne sme da izda JWT")
	}
}

func TestMagicLogin_ValidJustBeforeExpiry(t *testing.T) {
	e := newMagicLoginEnv(t)
	e.svc.now = func() time.Time { return magicNow.Add(10*time.Minute - time.Second) }

	if _, err := e.svc.Login(context.Background(), e.raw); err != nil {
		t.Fatalf("link je jos vazeci: %v", err)
	}
}

func TestMagicLogin_UnknownAndMalformedToken(t *testing.T) {
	e := newMagicLoginEnv(t)
	for name, raw := range map[string]string{
		"nepoznat": "nepostojeci-token",
		"prazan":   "",
		"razmaci":  "   ",
		"predug":   strings.Repeat("a", maxTokenLen+1),
	} {
		if _, err := e.svc.Login(context.Background(), raw); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("%s: err = %v, ocekivano ErrInvalidToken", name, err)
		}
	}
	if e.issuer.calls != 0 {
		t.Error("nevazeci token ne sme da izda JWT")
	}
	if len(e.tokens.byUser) != 1 {
		t.Error("pogresan token ne sme da potrosi tudj token")
	}
}

func TestMagicLogin_TrimsToken(t *testing.T) {
	e := newMagicLoginEnv(t)
	if _, err := e.svc.Login(context.Background(), "  "+e.raw+"\n"); err != nil {
		t.Fatalf("razmaci oko tokena se uklanjaju: %v", err)
	}
}

func TestMagicLogin_InactiveOrDeletedAccount(t *testing.T) {
	t.Run("neaktivan", func(t *testing.T) {
		e := newMagicLoginEnv(t)
		e.user.Active = false
		e.users.users[magicEmail] = e.user
		if _, err := e.svc.Login(context.Background(), e.raw); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("err = %v, ocekivano ErrInvalidToken", err)
		}
		if e.issuer.calls != 0 {
			t.Error("neaktivan nalog ne dobija JWT")
		}
	})
	t.Run("obrisan", func(t *testing.T) {
		e := newMagicLoginEnv(t)
		delete(e.users.users, magicEmail)
		if _, err := e.svc.Login(context.Background(), e.raw); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("err = %v, ocekivano ErrInvalidToken", err)
		}
	})
}

func TestMagicLogin_ExpiredPasswordDoesNotBlock(t *testing.T) {
	e := newMagicLoginEnv(t)
	e.user.PasswordChangedAt = magicNow.Add(-365 * 24 * time.Hour) // lozinka odavno istekla
	e.users.users[magicEmail] = e.user

	if _, err := e.svc.Login(context.Background(), e.raw); err != nil {
		t.Fatalf("povracaj naloga mora raditi i sa isteklom lozinkom: %v", err)
	}
}

func TestMagicLogin_RestoresTokenOnInfrastructureFailure(t *testing.T) {
	boom := errors.New("kvar")
	t.Run("izdavanje JWT-a", func(t *testing.T) {
		e := newMagicLoginEnv(t)
		e.issuer.err = boom
		if _, err := e.svc.Login(context.Background(), e.raw); !errors.Is(err, boom) {
			t.Fatalf("err = %v", err)
		}
		if e.tokens.restores != 1 || len(e.tokens.byUser) != 1 {
			t.Errorf("token mora biti vracen: restores=%d, tokena=%d", e.tokens.restores, len(e.tokens.byUser))
		}
		// Posle kvara isti link opet radi.
		e.issuer.err = nil
		if _, err := e.svc.Login(context.Background(), e.raw); err != nil {
			t.Errorf("ponovni pokusaj: %v", err)
		}
	})
	t.Run("citanje naloga", func(t *testing.T) {
		e := newMagicLoginEnv(t)
		e.users.findIDErr = boom
		if _, err := e.svc.Login(context.Background(), e.raw); !errors.Is(err, boom) {
			t.Fatalf("err = %v", err)
		}
		if e.tokens.restores != 1 {
			t.Errorf("token mora biti vracen: restores=%d", e.tokens.restores)
		}
	})
}

func TestMagicLogin_ConsumeError(t *testing.T) {
	e := newMagicLoginEnv(t)
	boom := errors.New("baza pala")
	e.tokens.consumeErr = boom

	if _, err := e.svc.Login(context.Background(), e.raw); !errors.Is(err, boom) || errors.Is(err, ErrInvalidToken) {
		t.Fatalf("kvar baze nije isto sto i nevazeci link: err = %v", err)
	}
}

func TestMagicRequestThenLogin_EndToEnd(t *testing.T) {
	e := newMagicEnv(t)
	issuer := &fakeMagicIssuer{result: "jwt"}
	login := NewMagicLoginService(e.users, e.tokens, issuer)
	login.now = func() time.Time { return *e.clock }

	if err := e.svc.Request(context.Background(), magicEmail); err != nil {
		t.Fatal(err)
	}
	raw := e.tokenFromMail(t)
	e.advance(3 * time.Minute)

	if tok, err := login.Login(context.Background(), raw); err != nil || tok != "jwt" {
		t.Fatalf("tok=%q err=%v", tok, err)
	}
	if _, err := login.Login(context.Background(), raw); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("link se ne moze iskoristiti dvaput: %v", err)
	}
}
