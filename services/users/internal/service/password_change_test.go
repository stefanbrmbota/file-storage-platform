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
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/token"
)

const (
	oldPassword = "Stara-lozinka-1!"
	newPassword = "Nova-lozinka-2@"
)

var pwNow = time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)

type fakePasswordStore struct {
	user      model.User
	findErr   error
	updateErr error

	finds   int
	updates int
	gotID   string
	gotOld  string
	gotNew  string
	gotAt   time.Time
}

func (f *fakePasswordStore) FindByID(_ context.Context, _ string) (model.User, error) {
	f.finds++
	return f.user, f.findErr
}

func (f *fakePasswordStore) UpdatePassword(_ context.Context, id, oldHash, newHash string, at time.Time) error {
	f.updates++
	f.gotID, f.gotOld, f.gotNew, f.gotAt = id, oldHash, newHash, at
	return f.updateErr
}

type pwEnv struct {
	svc     *PasswordService
	store   *fakePasswordStore
	revoked *fakeRevoked
}

// newPwEnv pravi servis za korisnika u1 cija je lozinka oldPassword, poslednji
// put promenjena pre changedAgo; najmanja starost je minAge.
func newPwEnv(t *testing.T, changedAgo, minAge time.Duration) pwEnv {
	t.Helper()
	hash, err := password.Hash(oldPassword, testParams)
	if err != nil {
		t.Fatal(err)
	}
	store := &fakePasswordStore{user: model.User{
		ID: "u1", Email: "ana@example.com", PasswordHash: hash, Active: true, Role: model.RoleUser,
		PasswordChangedAt: pwNow.Add(-changedAgo),
	}}
	rev := &fakeRevoked{}
	parser := fakeParser{claims: token.Claims{UserID: "u1", ID: "jti-1", ExpiresAt: pwNow.Add(10 * time.Minute)}}
	svc, err := NewPasswordService(store, parser, rev, testParams, minAge)
	if err != nil {
		t.Fatal(err)
	}
	svc.now = func() time.Time { return pwNow }
	return pwEnv{svc: svc, store: store, revoked: rev}
}

func validChange() ChangePasswordInput {
	return ChangePasswordInput{
		UserID: "u1", Token: "jwt", CurrentPassword: oldPassword, NewPassword: newPassword, NewConfirm: newPassword,
	}
}

func TestChangePassword_Success(t *testing.T) {
	e := newPwEnv(t, 48*time.Hour, 24*time.Hour)
	oldHash := e.store.user.PasswordHash

	if err := e.svc.Change(context.Background(), validChange()); err != nil {
		t.Fatal(err)
	}
	if e.store.updates != 1 || e.store.gotID != "u1" {
		t.Fatalf("updates = %d, id = %q", e.store.updates, e.store.gotID)
	}
	if e.store.gotOld != oldHash {
		t.Error("compare-and-set mora koristiti stari heš")
	}
	if ok, err := password.Verify(newPassword, e.store.gotNew); err != nil || !ok {
		t.Errorf("novi heš ne odgovara novoj lozinci: ok=%v err=%v", ok, err)
	}
	if e.store.gotNew == oldHash || strings.Contains(e.store.gotNew, newPassword) {
		t.Error("novi heš je neispravan")
	}
	if !e.store.gotAt.Equal(pwNow) {
		t.Errorf("passwordChangedAt = %v, ocekivano %v", e.store.gotAt, pwNow)
	}
	if e.revoked.calls != 1 || e.revoked.jti != "jti-1" {
		t.Errorf("token nije opozvan: calls=%d jti=%q", e.revoked.calls, e.revoked.jti)
	}
}

func TestChangePassword_InvalidToken(t *testing.T) {
	cases := map[string]fakeParser{
		"neispravan token": {err: token.ErrInvalidToken},
		"tudj korisnik":    {claims: token.Claims{UserID: "u2", ID: "j", ExpiresAt: pwNow.Add(time.Minute)}},
		"bez jti":          {claims: token.Claims{UserID: "u1", ExpiresAt: pwNow.Add(time.Minute)}},
	}
	for name, parser := range cases {
		e := newPwEnv(t, 48*time.Hour, 24*time.Hour)
		e.svc.tokens = parser
		if err := e.svc.Change(context.Background(), validChange()); !errors.Is(err, ErrInvalidAuthToken) {
			t.Errorf("%s: err = %v, ocekivano ErrInvalidAuthToken", name, err)
		}
		if e.store.finds != 0 || e.store.updates != 0 || e.revoked.calls != 0 {
			t.Errorf("%s: nista ne sme biti procitano ni upisano", name)
		}
	}
}

func TestChangePassword_NewPasswordRules(t *testing.T) {
	cases := map[string]struct {
		pw, confirm string
		want        string
	}{
		"prekratka":         {"Ab1!", "Ab1!", "najmanje 8 karaktera"},
		"bez specijalnog":   {"Nova-lozinka", "Nova-lozinka", "broj"},
		"bez velikog slova": {"nova-lozinka-2@", "nova-lozinka-2@", "veliko slovo"},
		"ne poklapa se":     {newPassword, "Druga-lozinka-3#", "ne poklapaju"},
		"cesta lozinka":     {"Password1!", "Password1!", "previše česta"},
		"prazna":            {"", "", "obavezna"},
	}
	for name, c := range cases {
		e := newPwEnv(t, 48*time.Hour, 24*time.Hour)
		in := validChange()
		in.NewPassword, in.NewConfirm = c.pw, c.confirm
		err := e.svc.Change(context.Background(), in)
		var rules *PasswordRulesError
		if !errors.As(err, &rules) {
			t.Errorf("%s: err = %v, ocekivano *PasswordRulesError", name, err)
			continue
		}
		if !strings.Contains(strings.Join(rules.Problems, "|"), c.want) {
			t.Errorf("%s: problemi = %v, ocekivano da sadrze %q", name, rules.Problems, c.want)
		}
		if e.store.updates != 0 || e.revoked.calls != 0 {
			t.Errorf("%s: lozinka ne sme biti izmenjena", name)
		}
	}
}

func TestChangePassword_WrongCurrentPassword(t *testing.T) {
	for name, current := range map[string]string{
		"pogresna":   "Pogresna-lozinka-9!",
		"prazna":     "",
		"predugacka": strings.Repeat("a", maxPasswordLen+1),
	} {
		e := newPwEnv(t, 48*time.Hour, 24*time.Hour)
		in := validChange()
		in.CurrentPassword = current
		if err := e.svc.Change(context.Background(), in); !errors.Is(err, ErrWrongPassword) {
			t.Errorf("%s: err = %v, ocekivano ErrWrongPassword", name, err)
		}
		if e.store.updates != 0 || e.revoked.calls != 0 {
			t.Errorf("%s: lozinka ne sme biti izmenjena", name)
		}
	}
}

func TestChangePassword_TooNew(t *testing.T) {
	e := newPwEnv(t, time.Hour, 24*time.Hour) // promenjena pre 1h, a treba 24h
	err := e.svc.Change(context.Background(), validChange())
	var tooNew *PasswordTooNewError
	if !errors.As(err, &tooNew) {
		t.Fatalf("err = %v, ocekivano *PasswordTooNewError", err)
	}
	want := e.store.user.PasswordChangedAt.Add(24 * time.Hour)
	if !tooNew.AllowedAt.Equal(want) {
		t.Errorf("AllowedAt = %v, ocekivano %v", tooNew.AllowedAt, want)
	}
	if e.store.updates != 0 || e.revoked.calls != 0 {
		t.Error("lozinka ne sme biti izmenjena")
	}
}

func TestChangePassword_ExactlyMinAgeIsAllowed(t *testing.T) {
	e := newPwEnv(t, 24*time.Hour, 24*time.Hour)
	if err := e.svc.Change(context.Background(), validChange()); err != nil {
		t.Fatalf("tacno posle 24h mora biti dozvoljeno: %v", err)
	}
}

func TestChangePassword_WrongCurrentDoesNotRevealAge(t *testing.T) {
	// Ko ne zna trenutnu lozinku ne sazna ni kada je poslednji put menjana.
	e := newPwEnv(t, time.Hour, 24*time.Hour)
	in := validChange()
	in.CurrentPassword = "Pogresna-lozinka-9!"
	err := e.svc.Change(context.Background(), in)
	var tooNew *PasswordTooNewError
	if !errors.Is(err, ErrWrongPassword) || errors.As(err, &tooNew) {
		t.Fatalf("err = %v, ocekivano ErrWrongPassword", err)
	}
}

func TestChangePassword_SameAsCurrent(t *testing.T) {
	e := newPwEnv(t, 48*time.Hour, 24*time.Hour)
	in := validChange()
	in.NewPassword, in.NewConfirm = oldPassword, oldPassword
	err := e.svc.Change(context.Background(), in)
	var rules *PasswordRulesError
	if !errors.As(err, &rules) || !strings.Contains(strings.Join(rules.Problems, "|"), "drugačija") {
		t.Fatalf("err = %v, ocekivano pravilo o razlicitoj lozinci", err)
	}
	if e.store.updates != 0 {
		t.Error("lozinka ne sme biti izmenjena")
	}
}

func TestChangePassword_ConcurrentChange(t *testing.T) {
	e := newPwEnv(t, 48*time.Hour, 24*time.Hour)
	e.store.updateErr = repository.ErrPasswordConflict
	if err := e.svc.Change(context.Background(), validChange()); !errors.Is(err, ErrPasswordChangeConflict) {
		t.Fatalf("err = %v, ocekivano ErrPasswordChangeConflict", err)
	}
	if e.revoked.calls != 0 {
		t.Error("token ne sme biti opozvan kad lozinka nije promenjena")
	}
}

func TestChangePassword_UserNotFoundOrInactive(t *testing.T) {
	e := newPwEnv(t, 48*time.Hour, 24*time.Hour)
	e.store.findErr = repository.ErrNotFound
	if err := e.svc.Change(context.Background(), validChange()); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("nepostojeci: err = %v", err)
	}

	e = newPwEnv(t, 48*time.Hour, 24*time.Hour)
	e.store.user.Active = false
	if err := e.svc.Change(context.Background(), validChange()); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("neaktivan: err = %v", err)
	}
	if e.store.updates != 0 {
		t.Error("neaktivnom nalogu se lozinka ne menja")
	}
}

func TestChangePassword_StoreErrors(t *testing.T) {
	boom := errors.New("baza pala")

	e := newPwEnv(t, 48*time.Hour, 24*time.Hour)
	e.store.findErr = boom
	if err := e.svc.Change(context.Background(), validChange()); !errors.Is(err, boom) || errors.Is(err, ErrUserNotFound) {
		t.Errorf("find: err = %v", err)
	}

	e = newPwEnv(t, 48*time.Hour, 24*time.Hour)
	e.store.updateErr = boom
	if err := e.svc.Change(context.Background(), validChange()); !errors.Is(err, boom) {
		t.Errorf("update: err = %v", err)
	}
	if e.revoked.calls != 0 {
		t.Error("token ne sme biti opozvan kad upis nije uspeo")
	}
}

func TestChangePassword_RevokeFailureDoesNotFailRequest(t *testing.T) {
	e := newPwEnv(t, 48*time.Hour, 24*time.Hour)
	e.revoked.err = errors.New("baza pala")
	if err := e.svc.Change(context.Background(), validChange()); err != nil {
		t.Fatalf("lozinka je promenjena, zahtev ne sme da padne: %v", err)
	}
	if e.store.updates != 1 {
		t.Error("lozinka mora biti upisana")
	}
}

func TestChangePassword_ErrorsNeverContainPasswords(t *testing.T) {
	e := newPwEnv(t, time.Hour, 24*time.Hour)
	err := e.svc.Change(context.Background(), validChange())
	if err == nil {
		t.Fatal("ocekivana greska (prerano menjanje)")
	}
	for _, secret := range []string{oldPassword, newPassword} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("greska sadrzi lozinku: %v", err)
		}
	}
	in := validChange()
	in.NewPassword, in.NewConfirm = "kratka", "kratka"
	err = e.svc.Change(context.Background(), in)
	if err == nil || strings.Contains(err.Error(), "kratka") {
		t.Errorf("greska sadrzi lozinku: %v", err)
	}
}

func TestNewPasswordService_Validation(t *testing.T) {
	store := &fakePasswordStore{}
	if _, err := NewPasswordService(store, fakeParser{}, &fakeRevoked{}, testParams, -time.Second); err == nil {
		t.Error("negativna najmanja starost mora biti odbijena")
	}
	if _, err := NewPasswordService(store, fakeParser{}, &fakeRevoked{}, password.Params{}, time.Hour); err == nil {
		t.Error("neispravni parametri heširanja moraju biti odbijeni")
	}
}
