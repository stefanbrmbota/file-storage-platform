package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/password"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

var resetNow = time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)

// fakeResetTokens je memorijska implementacija PasswordResetRepository.
type fakeResetTokens struct {
	byUser map[string]model.PasswordResetToken

	getErr, saveErr, consumeErr, restoreErr error
	saves, restores, deletes                int
}

func newFakeResetTokens() *fakeResetTokens {
	return &fakeResetTokens{byUser: map[string]model.PasswordResetToken{}}
}

func (f *fakeResetTokens) Save(_ context.Context, t model.PasswordResetToken) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saves++
	f.byUser[t.UserID] = t
	return nil
}

func (f *fakeResetTokens) Get(_ context.Context, userID string) (model.PasswordResetToken, error) {
	if f.getErr != nil {
		return model.PasswordResetToken{}, f.getErr
	}
	t, ok := f.byUser[userID]
	if !ok {
		return model.PasswordResetToken{}, repository.ErrTokenNotFound
	}
	return t, nil
}

func (f *fakeResetTokens) Consume(_ context.Context, hash string, now time.Time) (model.PasswordResetToken, error) {
	if f.consumeErr != nil {
		return model.PasswordResetToken{}, f.consumeErr
	}
	for id, t := range f.byUser {
		if t.TokenHash == hash && t.ExpiresAt.After(now) {
			delete(f.byUser, id)
			return t, nil
		}
	}
	return model.PasswordResetToken{}, repository.ErrTokenNotFound
}

func (f *fakeResetTokens) Restore(_ context.Context, t model.PasswordResetToken) error {
	if f.restoreErr != nil {
		return f.restoreErr
	}
	f.restores++
	if _, exists := f.byUser[t.UserID]; !exists {
		f.byUser[t.UserID] = t
	}
	return nil
}

func (f *fakeResetTokens) Delete(_ context.Context, userID string) error {
	f.deletes++
	delete(f.byUser, userID)
	return nil
}

// fakeResetUsers je memorijski PasswordResetUsers.
type fakeResetUsers struct {
	users map[string]model.User // po emailu

	findEmailErr, findIDErr, updateErr error
	findEmails                         int
	updates                            int
	gotID, gotOld, gotNew              string
	gotAt                              time.Time
}

func (f *fakeResetUsers) FindByEmail(_ context.Context, email string) (model.User, error) {
	f.findEmails++
	if f.findEmailErr != nil {
		return model.User{}, f.findEmailErr
	}
	u, ok := f.users[email]
	if !ok {
		return model.User{}, repository.ErrNotFound
	}
	return u, nil
}

func (f *fakeResetUsers) FindByID(_ context.Context, id string) (model.User, error) {
	if f.findIDErr != nil {
		return model.User{}, f.findIDErr
	}
	for _, u := range f.users {
		if u.ID == id {
			return u, nil
		}
	}
	return model.User{}, repository.ErrNotFound
}

func (f *fakeResetUsers) UpdatePassword(_ context.Context, id, oldHash, newHash string, at time.Time) error {
	f.updates++
	f.gotID, f.gotOld, f.gotNew, f.gotAt = id, oldHash, newHash, at
	return f.updateErr
}

type fakeOTPDeleter struct {
	calls int
	id    string
	err   error
}

func (f *fakeOTPDeleter) Delete(_ context.Context, id string) error {
	f.calls++
	f.id = id
	return f.err
}

type sentMail struct{ to, subject, body string }

type fakeResetMailer struct {
	sent []sentMail
	err  error
}

func (f *fakeResetMailer) Send(_ context.Context, to, subject, body string) error {
	f.sent = append(f.sent, sentMail{to, subject, body})
	return f.err
}

type resetEnv struct {
	svc    *PasswordResetService
	users  *fakeResetUsers
	tokens *fakeResetTokens
	otps   *fakeOTPDeleter
	mailer *fakeResetMailer
	user   model.User
	clock  *time.Time
}

const (
	resetEmail   = "ana@example.com"
	resetOldPass = "Stara-lozinka-1!"
	resetNewPass = "Nova-lozinka-2@"
	resetBaseURL = "https://app.test/reset-lozinke"
)

func newResetEnv(t *testing.T) *resetEnv {
	t.Helper()
	hash, err := password.Hash(resetOldPass, testParams)
	if err != nil {
		t.Fatal(err)
	}
	user := model.User{
		ID: "u1", FirstName: "Ana", Email: resetEmail, PasswordHash: hash, Active: true, Role: model.RoleUser,
		PasswordChangedAt: resetNow.Add(-48 * time.Hour),
	}
	users := &fakeResetUsers{users: map[string]model.User{resetEmail: user}}
	tokens := newFakeResetTokens()
	otps := &fakeOTPDeleter{}
	mailer := &fakeResetMailer{}
	svc, err := NewPasswordResetService(users, tokens, otps, mailer, PasswordResetConfig{
		BaseURL: resetBaseURL, TTL: 15 * time.Minute, Cooldown: time.Minute, Params: testParams,
	})
	if err != nil {
		t.Fatal(err)
	}
	clock := resetNow
	svc.now = func() time.Time { return clock }
	svc.async = func(f func()) { f() } // deterministicno: mejl se salje odmah
	return &resetEnv{svc: svc, users: users, tokens: tokens, otps: otps, mailer: mailer, user: user, clock: &clock}
}

func (e *resetEnv) advance(d time.Duration) { *e.clock = e.clock.Add(d) }

// tokenFromMail vraca token iz poslednjeg mejla sa linkom (deo posle #token=).
func (e *resetEnv) tokenFromMail(t *testing.T) string {
	t.Helper()
	if len(e.mailer.sent) == 0 {
		t.Fatal("nijedan mejl nije poslat")
	}
	body := e.mailer.sent[len(e.mailer.sent)-1].body
	i := strings.Index(body, resetBaseURL+"#token=")
	if i < 0 {
		t.Fatalf("mejl nema link: %q", body)
	}
	rest := body[i+len(resetBaseURL+"#token="):]
	if j := strings.IndexAny(rest, " \r\n"); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

// ---- Request (T-15.2) ----

func TestResetRequest_ActiveAccount_SavesTokenAndSendsMail(t *testing.T) {
	e := newResetEnv(t)
	if err := e.svc.Request(context.Background(), resetEmail); err != nil {
		t.Fatal(err)
	}

	tok, err := e.tokens.Get(context.Background(), "u1")
	if err != nil {
		t.Fatalf("token nije sacuvan: %v", err)
	}
	if !tok.CreatedAt.Equal(resetNow) || !tok.ExpiresAt.Equal(resetNow.Add(15*time.Minute)) {
		t.Errorf("vreme tokena: kreiran %v, istice %v", tok.CreatedAt, tok.ExpiresAt)
	}

	if len(e.mailer.sent) != 1 {
		t.Fatalf("poslato mejlova: %d", len(e.mailer.sent))
	}
	m := e.mailer.sent[0]
	if m.to != resetEmail || m.subject != "Reset lozinke" {
		t.Errorf("mejl: %+v", m)
	}
	raw := e.tokenFromMail(t)
	if tok.TokenHash != hashToken(raw) {
		t.Error("sacuvan je heš tokena iz mejla")
	}
	if tok.TokenHash == raw || strings.Contains(tok.TokenHash, raw) {
		t.Error("u bazi sme biti samo heš, ne i sam token")
	}
	if !strings.Contains(m.body, "Ana") || !strings.Contains(m.body, "26.11.2026. 12:15 UTC") {
		t.Errorf("mejl mora imati ime i rok vazenja linka: %q", m.body)
	}
}

func TestResetRequest_UnknownAccount_DoesNothingAndReturnsNil(t *testing.T) {
	e := newResetEnv(t)
	if err := e.svc.Request(context.Background(), "niko@example.com"); err != nil {
		t.Fatalf("nepoznat nalog ne sme dati gresku: %v", err)
	}
	if e.tokens.saves != 0 || len(e.mailer.sent) != 0 {
		t.Errorf("nista ne sme biti sacuvano ni poslato: saves=%d mejlova=%d", e.tokens.saves, len(e.mailer.sent))
	}
}

func TestResetRequest_InactiveAccount_DoesNothing(t *testing.T) {
	e := newResetEnv(t)
	u := e.users.users[resetEmail]
	u.Active = false
	e.users.users[resetEmail] = u

	if err := e.svc.Request(context.Background(), resetEmail); err != nil {
		t.Fatal(err)
	}
	if e.tokens.saves != 0 || len(e.mailer.sent) != 0 {
		t.Error("neaktivnom nalogu se ne salje link")
	}
}

func TestResetRequest_InvalidEmail(t *testing.T) {
	for _, in := range []string{"", "   ", "nije-email", "a@b", "ime <a@b.c>", strings.Repeat("a", 260) + "@example.com"} {
		e := newResetEnv(t)
		if err := e.svc.Request(context.Background(), in); !errors.Is(err, ErrInvalidEmail) {
			t.Errorf("%q: err = %v, ocekivano ErrInvalidEmail", in, err)
		}
		if e.users.findEmails != 0 {
			t.Errorf("%q: baza ne sme biti pitana", in)
		}
	}
}

func TestResetRequest_EmailIsNormalized(t *testing.T) {
	e := newResetEnv(t)
	if err := e.svc.Request(context.Background(), "  ANA@Example.COM "); err != nil {
		t.Fatal(err)
	}
	if len(e.mailer.sent) != 1 {
		t.Fatal("nalog mora biti pronadjen i pored velikih slova i razmaka")
	}
}

func TestResetRequest_CooldownBlocksSecondMailThenAllowsAfter(t *testing.T) {
	e := newResetEnv(t)
	ctx := context.Background()
	if err := e.svc.Request(ctx, resetEmail); err != nil {
		t.Fatal(err)
	}
	first, _ := e.tokens.Get(ctx, "u1")

	e.advance(30 * time.Second) // unutar 1 min
	if err := e.svc.Request(ctx, resetEmail); err != nil {
		t.Fatalf("ogranicenje ne sme biti vidljivo pozivaocu: %v", err)
	}
	if len(e.mailer.sent) != 1 {
		t.Fatalf("drugi mejl unutar ogranicenja: poslato %d", len(e.mailer.sent))
	}
	if cur, _ := e.tokens.Get(ctx, "u1"); cur.TokenHash != first.TokenHash {
		t.Error("token se ne sme menjati unutar ogranicenja")
	}

	e.advance(31 * time.Second) // ukupno 61 s
	if err := e.svc.Request(ctx, resetEmail); err != nil {
		t.Fatal(err)
	}
	if len(e.mailer.sent) != 2 {
		t.Fatalf("posle ogranicenja mejl mora biti poslat: %d", len(e.mailer.sent))
	}
	if cur, _ := e.tokens.Get(ctx, "u1"); cur.TokenHash == first.TokenHash {
		t.Error("novi zahtev mora zameniti stari token")
	}
}

func TestResetRequest_MailFailure_DeletesTokenAndDoesNotBlockRetry(t *testing.T) {
	e := newResetEnv(t)
	e.mailer.err = errors.New("smtp pao")
	ctx := context.Background()

	if err := e.svc.Request(ctx, resetEmail); err != nil {
		t.Fatalf("neuspelo slanje ne sme biti vidljivo pozivaocu: %v", err)
	}
	if _, err := e.tokens.Get(ctx, "u1"); !errors.Is(err, repository.ErrTokenNotFound) {
		t.Fatalf("token bez mejla mora biti obrisan: err = %v", err)
	}

	// Odmah ponovo: ogranicenje ucestalosti ne sme da blokira, jer link nikad nije stigao.
	e.mailer.err = nil
	if err := e.svc.Request(ctx, resetEmail); err != nil {
		t.Fatal(err)
	}
	if len(e.mailer.sent) != 2 {
		t.Errorf("ponovni pokusaj mora poslati mejl: poslato %d", len(e.mailer.sent))
	}
}

func TestResetRequest_MailIsSentInBackground(t *testing.T) {
	e := newResetEnv(t)
	var pending []func()
	e.svc.async = func(f func()) { pending = append(pending, f) } // ne izvrsava odmah

	if err := e.svc.Request(context.Background(), resetEmail); err != nil {
		t.Fatal(err)
	}
	if len(e.mailer.sent) != 0 {
		t.Fatal("Request ne sme da ceka slanje mejla")
	}
	if len(pending) != 1 {
		t.Fatalf("zakazano slanja: %d", len(pending))
	}
	pending[0]()
	if len(e.mailer.sent) != 1 {
		t.Error("posle izvrsavanja zakazanog posla mejl mora biti poslat")
	}

	// Nepoznat nalog ne zakazuje nista, ali pozivaocu izgleda isto.
	pending = nil
	if err := e.svc.Request(context.Background(), "niko@example.com"); err != nil || len(pending) != 0 {
		t.Errorf("nepoznat nalog: err = %v, zakazano %d", err, len(pending))
	}
}

func TestResetRequest_InfrastructureErrors(t *testing.T) {
	boom := errors.New("baza pala")

	e := newResetEnv(t)
	e.users.findEmailErr = boom
	if err := e.svc.Request(context.Background(), resetEmail); !errors.Is(err, boom) {
		t.Errorf("trazenje naloga: err = %v", err)
	}

	e = newResetEnv(t)
	e.tokens.getErr = boom
	if err := e.svc.Request(context.Background(), resetEmail); !errors.Is(err, boom) {
		t.Errorf("provera tokena: err = %v", err)
	}

	e = newResetEnv(t)
	e.tokens.saveErr = boom
	if err := e.svc.Request(context.Background(), resetEmail); !errors.Is(err, boom) {
		t.Errorf("upis tokena: err = %v", err)
	}
	if len(e.mailer.sent) != 0 {
		t.Error("bez sacuvanog tokena mejl se ne salje")
	}
}

func TestResetRequest_TokensAreUniqueAndLong(t *testing.T) {
	e := newResetEnv(t)
	ctx := context.Background()
	_ = e.svc.Request(ctx, resetEmail)
	a := e.tokenFromMail(t)
	e.advance(2 * time.Minute)
	_ = e.svc.Request(ctx, resetEmail)
	b := e.tokenFromMail(t)
	if a == b || len(a) < 40 || len(b) < 40 {
		t.Errorf("tokeni moraju biti jedinstveni i dovoljno dugi: %q %q", a, b)
	}
}

func TestNewPasswordResetService_Validation(t *testing.T) {
	ok := PasswordResetConfig{BaseURL: resetBaseURL, TTL: time.Minute, Cooldown: time.Second, Params: testParams}
	users, tokens, otps, mailer := &fakeResetUsers{}, newFakeResetTokens(), &fakeOTPDeleter{}, &fakeResetMailer{}

	cases := map[string]PasswordResetConfig{
		"prazan URL":         {BaseURL: " ", TTL: time.Minute, Params: testParams},
		"ttl nula":           {BaseURL: resetBaseURL, TTL: 0, Params: testParams},
		"negativan cooldown": {BaseURL: resetBaseURL, TTL: time.Minute, Cooldown: -time.Second, Params: testParams},
		"losi parametri":     {BaseURL: resetBaseURL, TTL: time.Minute, Params: password.Params{}},
	}
	for name, cfg := range cases {
		if _, err := NewPasswordResetService(users, tokens, otps, mailer, cfg); err == nil {
			t.Errorf("%s: ocekivana greska", name)
		}
	}
	if _, err := NewPasswordResetService(users, tokens, otps, mailer, ok); err != nil {
		t.Errorf("ispravna konfiguracija: %v", err)
	}
}
