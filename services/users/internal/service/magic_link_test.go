package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

var magicNow = time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)

// fakeMagicTokens je memorijska implementacija MagicLinkRepository.
type fakeMagicTokens struct {
	byUser map[string]model.MagicLinkToken

	getErr, saveErr, consumeErr, restoreErr error
	saves, restores, deletes                int
}

func newFakeMagicTokens() *fakeMagicTokens {
	return &fakeMagicTokens{byUser: map[string]model.MagicLinkToken{}}
}

func (f *fakeMagicTokens) Save(_ context.Context, t model.MagicLinkToken) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saves++
	f.byUser[t.UserID] = t
	return nil
}

func (f *fakeMagicTokens) Get(_ context.Context, userID string) (model.MagicLinkToken, error) {
	if f.getErr != nil {
		return model.MagicLinkToken{}, f.getErr
	}
	t, ok := f.byUser[userID]
	if !ok {
		return model.MagicLinkToken{}, repository.ErrTokenNotFound
	}
	return t, nil
}

func (f *fakeMagicTokens) Consume(_ context.Context, hash string, now time.Time) (model.MagicLinkToken, error) {
	if f.consumeErr != nil {
		return model.MagicLinkToken{}, f.consumeErr
	}
	for id, t := range f.byUser {
		if t.TokenHash == hash && t.ExpiresAt.After(now) {
			delete(f.byUser, id)
			return t, nil
		}
	}
	return model.MagicLinkToken{}, repository.ErrTokenNotFound
}

func (f *fakeMagicTokens) Restore(_ context.Context, t model.MagicLinkToken) error {
	if f.restoreErr != nil {
		return f.restoreErr
	}
	f.restores++
	if _, exists := f.byUser[t.UserID]; !exists {
		f.byUser[t.UserID] = t
	}
	return nil
}

func (f *fakeMagicTokens) Delete(_ context.Context, userID string) error {
	f.deletes++
	delete(f.byUser, userID)
	return nil
}

// fakeMagicUsers je memorijski MagicLinkUsers.
type fakeMagicUsers struct {
	users map[string]model.User // po emailu

	findEmailErr, findIDErr error
}

func (f *fakeMagicUsers) FindByEmail(_ context.Context, email string) (model.User, error) {
	if f.findEmailErr != nil {
		return model.User{}, f.findEmailErr
	}
	u, ok := f.users[email]
	if !ok {
		return model.User{}, repository.ErrNotFound
	}
	return u, nil
}

func (f *fakeMagicUsers) FindByID(_ context.Context, id string) (model.User, error) {
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

type magicEnv struct {
	svc    *MagicLinkService
	users  *fakeMagicUsers
	tokens *fakeMagicTokens
	mailer *fakeResetMailer
	user   model.User
	clock  *time.Time
}

const (
	magicEmail   = "ana@example.com"
	magicBaseURL = "https://app.test/magicni-link"
)

func newMagicEnv(t *testing.T) *magicEnv {
	t.Helper()
	user := model.User{
		ID: "u1", FirstName: "Ana", Email: magicEmail, Active: true, Role: model.RoleUser,
		PasswordChangedAt: magicNow.Add(-48 * time.Hour),
	}
	users := &fakeMagicUsers{users: map[string]model.User{magicEmail: user}}
	tokens := newFakeMagicTokens()
	mailer := &fakeResetMailer{}
	svc, err := NewMagicLinkService(users, tokens, mailer, MagicLinkConfig{
		BaseURL: magicBaseURL, TTL: 10 * time.Minute, Cooldown: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	clock := magicNow
	svc.now = func() time.Time { return clock }
	svc.async = func(f func()) { f() } // deterministicno: mejl se salje odmah
	return &magicEnv{svc: svc, users: users, tokens: tokens, mailer: mailer, user: user, clock: &clock}
}

func (e *magicEnv) advance(d time.Duration) { *e.clock = e.clock.Add(d) }

// tokenFromMail vraca token iz poslednjeg mejla sa linkom (deo posle #token=).
func (e *magicEnv) tokenFromMail(t *testing.T) string {
	t.Helper()
	if len(e.mailer.sent) == 0 {
		t.Fatal("nijedan mejl nije poslat")
	}
	body := e.mailer.sent[len(e.mailer.sent)-1].body
	prefix := magicBaseURL + "#token="
	i := strings.Index(body, prefix)
	if i < 0 {
		t.Fatalf("mejl nema link: %q", body)
	}
	rest := body[i+len(prefix):]
	if end := strings.IndexAny(rest, " \n\r"); end >= 0 {
		rest = rest[:end]
	}
	return rest
}

func TestNewMagicLinkService_Validation(t *testing.T) {
	users := &fakeMagicUsers{}
	tokens := newFakeMagicTokens()
	mailer := &fakeResetMailer{}
	cases := map[string]MagicLinkConfig{
		"prazan URL":         {BaseURL: " ", TTL: time.Minute, Cooldown: time.Minute},
		"TTL nula":           {BaseURL: magicBaseURL, TTL: 0, Cooldown: time.Minute},
		"negativan cooldown": {BaseURL: magicBaseURL, TTL: time.Minute, Cooldown: -time.Second},
	}
	for name, cfg := range cases {
		if _, err := NewMagicLinkService(users, tokens, mailer, cfg); err == nil {
			t.Errorf("%s: ocekivana greska", name)
		}
	}
	if _, err := NewMagicLinkService(users, tokens, mailer, MagicLinkConfig{BaseURL: magicBaseURL, TTL: time.Minute, Cooldown: 0}); err != nil {
		t.Errorf("cooldown 0 mora biti dozvoljen: %v", err)
	}
}

func TestMagicRequest_SendsLinkAndStoresHashedToken(t *testing.T) {
	e := newMagicEnv(t)

	if err := e.svc.Request(context.Background(), "  ANA@Example.com "); err != nil {
		t.Fatal(err)
	}
	if len(e.mailer.sent) != 1 || e.mailer.sent[0].to != magicEmail {
		t.Fatalf("poslato: %+v", e.mailer.sent)
	}
	raw := e.tokenFromMail(t)
	if raw == "" {
		t.Fatal("token u linku je prazan")
	}
	stored, ok := e.tokens.byUser["u1"]
	if !ok {
		t.Fatal("token nije sacuvan")
	}
	if stored.TokenHash == raw || stored.TokenHash != hashToken(raw) {
		t.Errorf("u bazi mora biti heš tokena, ne sam token: %q", stored.TokenHash)
	}
	if !stored.ExpiresAt.Equal(magicNow.Add(10 * time.Minute)) {
		t.Errorf("rok vazenja = %v", stored.ExpiresAt)
	}
}

func TestMagicRequest_UnknownAccount_NoMailNoError(t *testing.T) {
	e := newMagicEnv(t)

	if err := e.svc.Request(context.Background(), "nepoznat@example.com"); err != nil {
		t.Fatalf("nepoznat nalog mora dati isti ishod kao poznat: %v", err)
	}
	if len(e.mailer.sent) != 0 || e.tokens.saves != 0 {
		t.Errorf("nema mejla ni tokena: mejlova=%d, upisa=%d", len(e.mailer.sent), e.tokens.saves)
	}
}

func TestMagicRequest_InactiveAccount_NoMailNoError(t *testing.T) {
	e := newMagicEnv(t)
	e.user.Active = false
	e.users.users[magicEmail] = e.user

	if err := e.svc.Request(context.Background(), magicEmail); err != nil {
		t.Fatal(err)
	}
	if len(e.mailer.sent) != 0 || e.tokens.saves != 0 {
		t.Errorf("neaktivan nalog ne dobija link: mejlova=%d, upisa=%d", len(e.mailer.sent), e.tokens.saves)
	}
}

func TestMagicRequest_InvalidEmail(t *testing.T) {
	e := newMagicEnv(t)
	for _, bad := range []string{"", "   ", "nije-email", "a@b", "a b@example.com"} {
		if err := e.svc.Request(context.Background(), bad); !errors.Is(err, ErrInvalidEmail) {
			t.Errorf("%q: err = %v, ocekivano ErrInvalidEmail", bad, err)
		}
	}
	if len(e.mailer.sent) != 0 {
		t.Error("za neispravan email se ne salje mejl")
	}
}

func TestMagicRequest_CooldownBlocksSecondMail(t *testing.T) {
	e := newMagicEnv(t)
	ctx := context.Background()

	_ = e.svc.Request(ctx, magicEmail)
	e.advance(30 * time.Second)
	if err := e.svc.Request(ctx, magicEmail); err != nil {
		t.Fatalf("cooldown se tiho preskace, bez greske: %v", err)
	}
	if len(e.mailer.sent) != 1 || e.tokens.saves != 1 {
		t.Fatalf("u cooldown-u nema novog mejla: mejlova=%d, upisa=%d", len(e.mailer.sent), e.tokens.saves)
	}

	e.advance(31 * time.Second) // ukupno 61 s > cooldown
	_ = e.svc.Request(ctx, magicEmail)
	if len(e.mailer.sent) != 2 {
		t.Fatalf("posle cooldown-a mejl se salje: mejlova=%d", len(e.mailer.sent))
	}
}

func TestMagicRequest_NewTokenReplacesOld(t *testing.T) {
	e := newMagicEnv(t)
	ctx := context.Background()

	_ = e.svc.Request(ctx, magicEmail)
	first := e.tokenFromMail(t)
	e.advance(2 * time.Minute)
	_ = e.svc.Request(ctx, magicEmail)
	second := e.tokenFromMail(t)

	if first == second {
		t.Fatal("svaki zahtev pravi novi token")
	}
	if len(e.tokens.byUser) != 1 || e.tokens.byUser["u1"].TokenHash != hashToken(second) {
		t.Errorf("mora ostati samo noviji token: %+v", e.tokens.byUser)
	}
}

func TestMagicRequest_MailFailureDeletesToken(t *testing.T) {
	e := newMagicEnv(t)
	e.mailer.err = errors.New("smtp pao")

	if err := e.svc.Request(context.Background(), magicEmail); err != nil {
		t.Fatalf("pad slanja ne sme da otkrije nalog: %v", err)
	}
	if _, ok := e.tokens.byUser["u1"]; ok {
		t.Error("token bez poslatog mejla mora biti obrisan")
	}
}

func TestMagicRequest_RepositoryErrors(t *testing.T) {
	boom := errors.New("baza pala")
	t.Run("trazenje naloga", func(t *testing.T) {
		e := newMagicEnv(t)
		e.users.findEmailErr = boom
		if err := e.svc.Request(context.Background(), magicEmail); !errors.Is(err, boom) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("provera prethodnog tokena", func(t *testing.T) {
		e := newMagicEnv(t)
		e.tokens.getErr = boom
		if err := e.svc.Request(context.Background(), magicEmail); !errors.Is(err, boom) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("upis tokena", func(t *testing.T) {
		e := newMagicEnv(t)
		e.tokens.saveErr = boom
		if err := e.svc.Request(context.Background(), magicEmail); !errors.Is(err, boom) {
			t.Errorf("err = %v", err)
		}
		if len(e.mailer.sent) != 0 {
			t.Error("bez upisanog tokena nema mejla")
		}
	})
}
