package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/password"
)

// testMaxAge je rok važenja lozinke u testovima prijave (60 dana, kao u specifikaciji).
const testMaxAge = 60 * 24 * time.Hour

var expiryNow = time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)

// expiryEnv pravi okruženje za prijavu sa zamrznutim časovnikom, a lozinka
// korisnika je poslednji put promenjena pre changedAgo.
func expiryEnv(t *testing.T, changedAgo time.Duration) *loginEnv {
	t.Helper()
	env := newLoginEnv(t, true)
	env.svc.now = func() time.Time { return expiryNow }
	setPasswordChanged(env, expiryNow.Add(-changedAgo))
	return env
}

func setPasswordChanged(env *loginEnv, at time.Time) {
	u := env.repo.users["pera@example.com"]
	u.PasswordChangedAt = at
	env.repo.users[u.Email] = u
}

func TestLoginStart_PasswordWithinLimit_SendsOTP(t *testing.T) {
	env := expiryEnv(t, 59*24*time.Hour)
	if err := env.svc.Start(context.Background(), "pera@example.com", "tajna-lozinka-123"); err != nil {
		t.Fatal(err)
	}
	if env.otp.calls != 1 || env.mailer.calls != 1 {
		t.Errorf("OTP i mejl moraju biti poslati: otp=%d mail=%d", env.otp.calls, env.mailer.calls)
	}
}

func TestLoginStart_ExpiredPassword_BlocksBeforeOTP(t *testing.T) {
	env := expiryEnv(t, 61*24*time.Hour)
	err := env.svc.Start(context.Background(), "pera@example.com", "tajna-lozinka-123")
	if !errors.Is(err, ErrPasswordExpired) {
		t.Fatalf("err = %v, ocekivano ErrPasswordExpired", err)
	}
	if env.otp.calls != 0 || env.mailer.calls != 0 {
		t.Errorf("OTP ne sme biti generisan ni poslat: otp=%d mail=%d", env.otp.calls, env.mailer.calls)
	}
}

func TestLoginStart_ExpiryBoundary(t *testing.T) {
	// Tacno 60 dana: lozinka jos vazi.
	env := expiryEnv(t, testMaxAge)
	if err := env.svc.Start(context.Background(), "pera@example.com", "tajna-lozinka-123"); err != nil {
		t.Fatalf("tacno na granici lozinka mora da vazi: %v", err)
	}
	// Najmanje vreme preko granice: istekla.
	env = expiryEnv(t, testMaxAge+time.Nanosecond)
	if err := env.svc.Start(context.Background(), "pera@example.com", "tajna-lozinka-123"); !errors.Is(err, ErrPasswordExpired) {
		t.Fatalf("preko granice: err = %v, ocekivano ErrPasswordExpired", err)
	}
}

func TestLoginStart_ExpiredButWrongPassword_DoesNotRevealExpiry(t *testing.T) {
	env := expiryEnv(t, 90*24*time.Hour)
	err := env.svc.Start(context.Background(), "pera@example.com", "pogresna")
	if !errors.Is(err, ErrInvalidCredentials) || errors.Is(err, ErrPasswordExpired) {
		t.Fatalf("err = %v, ocekivano ErrInvalidCredentials (stanje naloga se ne otkriva)", err)
	}
}

func TestLoginStart_ExpiredButUnknownOrInactive_DoesNotRevealExpiry(t *testing.T) {
	env := expiryEnv(t, 90*24*time.Hour)
	if err := env.svc.Start(context.Background(), "nema@example.com", "tajna-lozinka-123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("nepostojeci nalog: err = %v", err)
	}

	env = expiryEnv(t, 90*24*time.Hour)
	u := env.repo.users["pera@example.com"]
	u.Active = false
	env.repo.users[u.Email] = u
	if err := env.svc.Start(context.Background(), "pera@example.com", "tajna-lozinka-123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("neaktivan nalog: err = %v", err)
	}
}

func TestLoginStart_PasswordChangedAfterExpiry_LoginWorksAgain(t *testing.T) {
	env := expiryEnv(t, 90*24*time.Hour)
	if err := env.svc.Start(context.Background(), "pera@example.com", "tajna-lozinka-123"); !errors.Is(err, ErrPasswordExpired) {
		t.Fatalf("pre promene: err = %v", err)
	}
	// Reset lozinke (US-15) postavlja passwordChangedAt na "sada".
	setPasswordChanged(env, expiryNow)
	if err := env.svc.Start(context.Background(), "pera@example.com", "tajna-lozinka-123"); err != nil {
		t.Fatalf("posle promene lozinke prijava mora raditi: %v", err)
	}
}

func TestLoginStart_MissingPasswordChangedAt_FallsBackToCreatedAt(t *testing.T) {
	env := expiryEnv(t, 0)
	u := env.repo.users["pera@example.com"]
	u.PasswordChangedAt = time.Time{} // stari dokument bez polja
	u.CreatedAt = expiryNow.Add(-10 * 24 * time.Hour)
	env.repo.users[u.Email] = u
	if err := env.svc.Start(context.Background(), "pera@example.com", "tajna-lozinka-123"); err != nil {
		t.Fatalf("nalog star 10 dana mora moci da se prijavi: %v", err)
	}

	u.CreatedAt = expiryNow.Add(-61 * 24 * time.Hour)
	env.repo.users[u.Email] = u
	if err := env.svc.Start(context.Background(), "pera@example.com", "tajna-lozinka-123"); !errors.Is(err, ErrPasswordExpired) {
		t.Fatalf("nalog star 61 dan: err = %v, ocekivano ErrPasswordExpired", err)
	}
}

func TestLoginFinish_PasswordExpiredBetweenSteps_NoToken(t *testing.T) {
	env := expiryEnv(t, 61*24*time.Hour)
	tok, err := env.svc.Finish(context.Background(), "pera@example.com", "042517")
	if !errors.Is(err, ErrPasswordExpired) {
		t.Fatalf("err = %v, ocekivano ErrPasswordExpired", err)
	}
	if tok != "" || env.tokens.userID != "" {
		t.Error("token ne sme biti izdat za isteklu lozinku")
	}
}

func TestLoginFinish_ExpiredButWrongOTP_DoesNotRevealExpiry(t *testing.T) {
	env := expiryEnv(t, 90*24*time.Hour)
	env.otp.verifyErr = ErrInvalidOTP
	_, err := env.svc.Finish(context.Background(), "pera@example.com", "000000")
	if !errors.Is(err, ErrInvalidOTP) || errors.Is(err, ErrPasswordExpired) {
		t.Fatalf("err = %v, ocekivano ErrInvalidOTP (ko nema OTP ne saznaje stanje naloga)", err)
	}
}

func TestLoginFinish_PasswordWithinLimit_IssuesToken(t *testing.T) {
	env := expiryEnv(t, 59*24*time.Hour)
	tok, err := env.svc.Finish(context.Background(), "pera@example.com", "042517")
	if err != nil || tok != "jwt-token" {
		t.Fatalf("tok=%q err=%v", tok, err)
	}
}

func TestLoginExpiry_ShortDemoPeriod(t *testing.T) {
	// Na odbrani se rok skracuje (npr. 2 min); pravilo je isto.
	env := expiryEnv(t, 3*time.Minute)
	svc, err := NewLoginService(env.repo, env.otp, env.tokens, env.mailer, 5*time.Minute, testParams, 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	svc.now = func() time.Time { return expiryNow }
	if err := svc.Start(context.Background(), "pera@example.com", "tajna-lozinka-123"); !errors.Is(err, ErrPasswordExpired) {
		t.Fatalf("lozinka stara 3 min pri roku od 2 min: err = %v", err)
	}
	setPasswordChanged(env, expiryNow.Add(-time.Minute))
	if err := svc.Start(context.Background(), "pera@example.com", "tajna-lozinka-123"); err != nil {
		t.Fatalf("lozinka stara 1 min pri roku od 2 min: %v", err)
	}
}

func TestNewLoginService_RequiresPositiveMaxAge(t *testing.T) {
	env := newLoginEnv(t, true)
	for _, d := range []time.Duration{0, -time.Hour} {
		if _, err := NewLoginService(env.repo, env.otp, env.tokens, env.mailer, time.Minute, testParams, d); err == nil {
			t.Errorf("rok %v: ocekivana greska (istek se ne sme slucajno iskljuciti)", d)
		}
	}
	if _, err := NewLoginService(env.repo, env.otp, env.tokens, env.mailer, time.Minute, password.Params{}, testMaxAge); err == nil {
		t.Error("neispravni parametri hesiranja moraju biti odbijeni")
	}
}
