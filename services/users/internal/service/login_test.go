package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/password"
)

type fakeOTPIssuer struct {
	code   string
	err    error
	userID string
	calls  int
}

func (f *fakeOTPIssuer) Generate(_ context.Context, userID string) (string, error) {
	f.calls++
	f.userID = userID
	return f.code, f.err
}

type loginEnv struct {
	svc    *LoginService
	repo   *fakeRepo
	otp    *fakeOTPIssuer
	mailer *fakeMailer
}

func newLoginEnv(t *testing.T, active bool) *loginEnv {
	t.Helper()
	repo := newFakeRepo()
	hash, err := password.Hash("tajna-lozinka-123", testParams)
	if err != nil {
		t.Fatal(err)
	}
	u := model.NewUser("u1", "Pera", "Peric", "pera@example.com", hash, 1<<30, time.Now())
	u.Active = active
	repo.users[u.Email] = u

	otp := &fakeOTPIssuer{code: "042517"}
	mailer := &fakeMailer{}
	svc, err := NewLoginService(repo, otp, mailer, 5*time.Minute, testParams)
	if err != nil {
		t.Fatal(err)
	}
	return &loginEnv{svc: svc, repo: repo, otp: otp, mailer: mailer}
}

func TestLoginStart_SendsOTP(t *testing.T) {
	env := newLoginEnv(t, true)
	// Email se normalizuje (razmaci i velika slova).
	if err := env.svc.Start(context.Background(), "  Pera@Example.com ", "tajna-lozinka-123"); err != nil {
		t.Fatal(err)
	}
	if env.otp.userID != "u1" {
		t.Errorf("OTP za korisnika %q", env.otp.userID)
	}
	if env.mailer.calls != 1 || env.mailer.to != "pera@example.com" {
		t.Fatalf("mejl: calls=%d to=%q", env.mailer.calls, env.mailer.to)
	}
	if !strings.Contains(env.mailer.msg, "042517") {
		t.Error("mejl ne sadrzi kod")
	}
	if !regexp.MustCompile(`važi 5 min`).MatchString(env.mailer.msg) {
		t.Error("mejl ne sadrzi rok vazenja")
	}
}

func TestLoginStart_InvalidCredentials(t *testing.T) {
	tests := []struct {
		name     string
		active   bool
		email    string
		password string
	}{
		{"pogresna lozinka", true, "pera@example.com", "pogresna"},
		{"nepostojeci email", true, "nema@example.com", "tajna-lozinka-123"},
		{"neaktivan nalog", false, "pera@example.com", "tajna-lozinka-123"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newLoginEnv(t, tc.active)
			err := env.svc.Start(context.Background(), tc.email, tc.password)
			if !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("err = %v, ocekivano ErrInvalidCredentials", err)
			}
			if env.otp.calls != 0 || env.mailer.calls != 0 {
				t.Errorf("OTP/mejl ne smeju biti poslati: otp=%d mail=%d", env.otp.calls, env.mailer.calls)
			}
		})
	}
}

func TestLoginStart_Failures(t *testing.T) {
	t.Run("OTP greska", func(t *testing.T) {
		env := newLoginEnv(t, true)
		env.otp.err = errors.New("baza pala")
		err := env.svc.Start(context.Background(), "pera@example.com", "tajna-lozinka-123")
		if err == nil || errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("err = %v, ocekivana interna greska", err)
		}
		if env.mailer.calls != 0 {
			t.Error("mejl ne sme biti poslat bez OTP-a")
		}
	})
	t.Run("mejl greska", func(t *testing.T) {
		env := newLoginEnv(t, true)
		env.mailer.err = errors.New("smtp pao")
		err := env.svc.Start(context.Background(), "pera@example.com", "tajna-lozinka-123")
		if err == nil || errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("err = %v, ocekivana interna greska", err)
		}
	})
	t.Run("pokvaren heš", func(t *testing.T) {
		env := newLoginEnv(t, true)
		u := env.repo.users["pera@example.com"]
		u.PasswordHash = "nije-phc"
		env.repo.users[u.Email] = u
		err := env.svc.Start(context.Background(), "pera@example.com", "x")
		if err == nil || errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("err = %v, ocekivana interna greska", err)
		}
	})
}
