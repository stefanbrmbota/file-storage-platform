package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/password"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

// requestToken trazi reset i vraca token iz mejla; mejlovi se posle brisu da
// testovi mogu da broje samo obavestenje o promeni.
func (e *resetEnv) requestToken(t *testing.T) string {
	t.Helper()
	if err := e.svc.Request(context.Background(), resetEmail); err != nil {
		t.Fatal(err)
	}
	raw := e.tokenFromMail(t)
	e.mailer.sent = nil
	return raw
}

func (e *resetEnv) resetWith(token, pw, confirm string) error {
	return e.svc.Reset(context.Background(), ResetPasswordInput{Token: token, NewPassword: pw, NewConfirm: confirm})
}

func TestReset_Success(t *testing.T) {
	e := newResetEnv(t)
	raw := e.requestToken(t)
	oldHash := e.user.PasswordHash
	e.advance(5 * time.Minute)

	if err := e.resetWith(raw, resetNewPass, resetNewPass); err != nil {
		t.Fatal(err)
	}
	if e.users.updates != 1 || e.users.gotID != "u1" {
		t.Fatalf("updates = %d, id = %q", e.users.updates, e.users.gotID)
	}
	if e.users.gotOld != oldHash {
		t.Error("compare-and-set mora koristiti trenutni heš")
	}
	if ok, err := password.Verify(resetNewPass, e.users.gotNew); err != nil || !ok {
		t.Errorf("novi heš ne odgovara novoj lozinci: ok=%v err=%v", ok, err)
	}
	if !e.users.gotAt.Equal(resetNow.Add(5 * time.Minute)) {
		t.Errorf("passwordChangedAt = %v, ocekivano trenutak reseta", e.users.gotAt)
	}
	if e.otps.calls != 1 || e.otps.id != "u1" {
		t.Errorf("OTP korisnika mora biti obrisan: calls=%d id=%q", e.otps.calls, e.otps.id)
	}
	if len(e.mailer.sent) != 1 || e.mailer.sent[0].to != resetEmail || e.mailer.sent[0].subject != "Lozinka je promenjena" {
		t.Errorf("vlasnik mora dobiti obavestenje: %+v", e.mailer.sent)
	}
}

func TestReset_TokenIsSingleUse(t *testing.T) {
	e := newResetEnv(t)
	raw := e.requestToken(t)

	if err := e.resetWith(raw, resetNewPass, resetNewPass); err != nil {
		t.Fatal(err)
	}
	if err := e.resetWith(raw, "Druga-lozinka-3#", "Druga-lozinka-3#"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("drugo koriscenje: err = %v, ocekivano ErrInvalidToken", err)
	}
	if e.users.updates != 1 {
		t.Errorf("lozinka je promenjena %d puta, ocekivano 1", e.users.updates)
	}
}

func TestReset_IgnoresMinPasswordAge(t *testing.T) {
	// Korisnik je lozinku zaboravio: reset radi i kad je lozinka tek promenjena.
	e := newResetEnv(t)
	u := e.users.users[resetEmail]
	u.PasswordChangedAt = resetNow.Add(-time.Minute)
	e.users.users[resetEmail] = u

	raw := e.requestToken(t)
	if err := e.resetWith(raw, resetNewPass, resetNewPass); err != nil {
		t.Fatalf("reset ne sme zavisiti od starosti lozinke: %v", err)
	}
}

func TestReset_InvalidTokens(t *testing.T) {
	cases := map[string]string{
		"prazan":     "",
		"razmaci":    "   ",
		"nepoznat":   "nepostojeci-token",
		"predugacak": strings.Repeat("a", maxTokenLen+1),
	}
	for name, tok := range cases {
		e := newResetEnv(t)
		_ = e.requestToken(t)
		if err := e.resetWith(tok, resetNewPass, resetNewPass); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("%s: err = %v, ocekivano ErrInvalidToken", name, err)
		}
		if e.users.updates != 0 {
			t.Errorf("%s: lozinka ne sme biti izmenjena", name)
		}
	}
}

func TestReset_ExpiredToken(t *testing.T) {
	e := newResetEnv(t)
	raw := e.requestToken(t)
	e.advance(15*time.Minute + time.Second) // rok je 15 min

	if err := e.resetWith(raw, resetNewPass, resetNewPass); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, ocekivano ErrInvalidToken", err)
	}
	if e.users.updates != 0 {
		t.Error("lozinka ne sme biti izmenjena")
	}
}

func TestReset_OlderLinkInvalidAfterNewRequest(t *testing.T) {
	e := newResetEnv(t)
	first := e.requestToken(t)
	e.advance(2 * time.Minute)
	second := e.requestToken(t)

	if err := e.resetWith(first, resetNewPass, resetNewPass); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("stari link: err = %v, ocekivano ErrInvalidToken", err)
	}
	if err := e.resetWith(second, resetNewPass, resetNewPass); err != nil {
		t.Fatalf("novi link mora raditi: %v", err)
	}
}

func TestReset_BadPasswordDoesNotBurnToken(t *testing.T) {
	cases := map[string][2]string{
		"slaba":           {"abc", "abc"},
		"bez specijalnog": {"Nova-lozinka", "Nova-lozinka"},
		"ne poklapa se":   {resetNewPass, "Druga-lozinka-3#"},
		"cesta":           {"Password1!", "Password1!"},
		"prazna":          {"", ""},
	}
	for name, c := range cases {
		e := newResetEnv(t)
		raw := e.requestToken(t)

		err := e.resetWith(raw, c[0], c[1])
		var rules *PasswordRulesError
		if !errors.As(err, &rules) {
			t.Errorf("%s: err = %v, ocekivano *PasswordRulesError", name, err)
			continue
		}
		if e.users.updates != 0 {
			t.Errorf("%s: lozinka ne sme biti izmenjena", name)
		}
		// Link mora ostati upotrebljiv posle odbijene lozinke.
		if err := e.resetWith(raw, resetNewPass, resetNewPass); err != nil {
			t.Errorf("%s: token je izgoreo na losoj lozinci: %v", name, err)
		}
	}
}

func TestReset_AccountDeletedOrInactive(t *testing.T) {
	e := newResetEnv(t)
	raw := e.requestToken(t)
	delete(e.users.users, resetEmail) // nalog obrisan posle slanja linka
	if err := e.resetWith(raw, resetNewPass, resetNewPass); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("obrisan nalog: err = %v", err)
	}
	if e.tokens.restores != 0 {
		t.Error("token obrisanog naloga se ne vraca")
	}

	e = newResetEnv(t)
	raw = e.requestToken(t)
	u := e.users.users[resetEmail]
	u.Active = false
	e.users.users[resetEmail] = u
	if err := e.resetWith(raw, resetNewPass, resetNewPass); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("neaktivan nalog: err = %v", err)
	}
	if e.users.updates != 0 {
		t.Error("neaktivnom nalogu se lozinka ne menja")
	}
}

func TestReset_ConcurrentPasswordChange_RestoresToken(t *testing.T) {
	e := newResetEnv(t)
	raw := e.requestToken(t)
	e.users.updateErr = repository.ErrPasswordConflict

	if err := e.resetWith(raw, resetNewPass, resetNewPass); !errors.Is(err, ErrPasswordChangeConflict) {
		t.Fatalf("err = %v, ocekivano ErrPasswordChangeConflict", err)
	}
	if e.tokens.restores != 1 {
		t.Errorf("token mora biti vracen: restores = %d", e.tokens.restores)
	}
	if len(e.mailer.sent) != 0 || e.otps.calls != 0 {
		t.Error("bez uspeha nema obavestenja ni brisanja OTP-a")
	}

	e.users.updateErr = nil
	if err := e.resetWith(raw, resetNewPass, resetNewPass); err != nil {
		t.Fatalf("ponovni pokusaj istim linkom mora uspeti: %v", err)
	}
}

func TestReset_InfrastructureErrorsRestoreToken(t *testing.T) {
	boom := errors.New("baza pala")

	e := newResetEnv(t)
	raw := e.requestToken(t)
	e.users.findIDErr = boom
	if err := e.resetWith(raw, resetNewPass, resetNewPass); !errors.Is(err, boom) || errors.Is(err, ErrInvalidToken) {
		t.Errorf("trazenje naloga: err = %v", err)
	}
	if e.tokens.restores != 1 {
		t.Errorf("trazenje naloga: restores = %d", e.tokens.restores)
	}

	e = newResetEnv(t)
	raw = e.requestToken(t)
	e.users.updateErr = boom
	if err := e.resetWith(raw, resetNewPass, resetNewPass); !errors.Is(err, boom) {
		t.Errorf("upis: err = %v", err)
	}
	if e.tokens.restores != 1 {
		t.Errorf("upis: restores = %d", e.tokens.restores)
	}

	e = newResetEnv(t)
	raw = e.requestToken(t)
	e.tokens.consumeErr = boom
	if err := e.resetWith(raw, resetNewPass, resetNewPass); !errors.Is(err, boom) {
		t.Errorf("trosenje tokena: err = %v", err)
	}
}

func TestReset_SideEffectFailuresDoNotFailReset(t *testing.T) {
	e := newResetEnv(t)
	raw := e.requestToken(t)
	e.otps.err = errors.New("baza pala")
	e.mailer.err = errors.New("smtp pao")

	if err := e.resetWith(raw, resetNewPass, resetNewPass); err != nil {
		t.Fatalf("lozinka je promenjena, zahtev ne sme da padne: %v", err)
	}
	if e.users.updates != 1 {
		t.Error("lozinka mora biti upisana")
	}

	// Korisnik bez OTP-a je uobicajen slucaj i nije greska.
	e = newResetEnv(t)
	raw = e.requestToken(t)
	e.otps.err = repository.ErrOTPNotFound
	if err := e.resetWith(raw, resetNewPass, resetNewPass); err != nil {
		t.Fatalf("ErrOTPNotFound se ignorise: %v", err)
	}
}

func TestReset_ErrorsNeverContainSecrets(t *testing.T) {
	e := newResetEnv(t)
	raw := e.requestToken(t)

	e.users.updateErr = errors.New("baza pala")
	err := e.resetWith(raw, resetNewPass, resetNewPass)
	if err == nil {
		t.Fatal("ocekivana greska")
	}
	for _, secret := range []string{resetNewPass, raw} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("greska sadrzi tajnu: %v", err)
		}
	}

	err = e.resetWith(raw, "kratka", "kratka")
	if err == nil || strings.Contains(err.Error(), "kratka") {
		t.Errorf("greska sadrzi lozinku: %v", err)
	}
}
