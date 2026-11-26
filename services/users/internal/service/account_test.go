package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

type fakeAccountRepo struct {
	user  model.User
	err   error
	calls int
	id    string
}

func (f *fakeAccountRepo) FindByID(_ context.Context, id string) (model.User, error) {
	f.calls++
	f.id = id
	return f.user, f.err
}

// passwordChanged je trenutak poslednje promene lozinke u testovima naloga.
var passwordChanged = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

func activeUser() model.User {
	return model.User{
		ID: "u1", FirstName: "Ana", LastName: "Anić", Email: "ana@example.com",
		PasswordHash: "tajni-hes", Role: model.RoleUser, Active: true,
		QuotaBytes: 1000, UsedBytes: 250,
		PasswordChangedAt: passwordChanged, CreatedAt: passwordChanged.Add(-24 * time.Hour),
	}
}

// accountSvc pravi servis sa rokom važenja lozinke od 60 dana (testMaxAge).
func accountSvc(users AccountFinder) *AccountService {
	svc, err := NewAccountService(users, testMaxAge)
	if err != nil {
		panic(err)
	}
	return svc
}

func TestAccountGet_OK(t *testing.T) {
	repo := &fakeAccountRepo{user: activeUser()}
	got, err := accountSvc(repo).Get(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	want := Account{
		ID: "u1", FirstName: "Ana", LastName: "Anić", Email: "ana@example.com",
		Role: model.RoleUser, QuotaBytes: 1000, UsedBytes: 250, RemainingBytes: 750,
		PasswordExpiresAt: passwordChanged.Add(testMaxAge),
	}
	if got != want {
		t.Errorf("got = %+v, ocekivano %+v", got, want)
	}
	if repo.id != "u1" {
		t.Errorf("trazen id = %q", repo.id)
	}
}

func TestAccountGet_RemainingNeverNegative(t *testing.T) {
	u := activeUser()
	u.UsedBytes = 1500 // zauzece vece od kvote (npr. kvota naknadno smanjena)
	got, err := accountSvc(&fakeAccountRepo{user: u}).Get(context.Background(), "u1")
	if err != nil || got.RemainingBytes != 0 {
		t.Fatalf("remaining = %d, err = %v", got.RemainingBytes, err)
	}
}

func TestAccountGet_FullQuota(t *testing.T) {
	u := activeUser()
	u.UsedBytes = u.QuotaBytes
	got, _ := accountSvc(&fakeAccountRepo{user: u}).Get(context.Background(), "u1")
	if got.RemainingBytes != 0 {
		t.Errorf("remaining = %d", got.RemainingBytes)
	}
}

func TestAccountGet_NotFound(t *testing.T) {
	repo := &fakeAccountRepo{err: repository.ErrNotFound}
	if _, err := accountSvc(repo).Get(context.Background(), "nema"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("err = %v, ocekivano ErrUserNotFound", err)
	}
}

func TestAccountGet_InactiveAccountIsNotFound(t *testing.T) {
	u := activeUser()
	u.Active = false
	if _, err := accountSvc(&fakeAccountRepo{user: u}).Get(context.Background(), "u1"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("err = %v, ocekivano ErrUserNotFound", err)
	}
}

func TestAccountGet_BlankIDDoesNotQueryRepo(t *testing.T) {
	repo := &fakeAccountRepo{user: activeUser()}
	for _, id := range []string{"", "   "} {
		if _, err := accountSvc(repo).Get(context.Background(), id); !errors.Is(err, ErrUserNotFound) {
			t.Errorf("id %q: err = %v", id, err)
		}
	}
	if repo.calls != 0 {
		t.Errorf("repo pozvan %d puta", repo.calls)
	}
}

func TestAccountGet_RepositoryError(t *testing.T) {
	boom := errors.New("baza pala")
	_, err := accountSvc(&fakeAccountRepo{err: boom}).Get(context.Background(), "u1")
	if !errors.Is(err, boom) || errors.Is(err, ErrUserNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestAccount_DoesNotExposeSecrets(t *testing.T) {
	got, _ := accountSvc(&fakeAccountRepo{user: activeUser()}).Get(context.Background(), "u1")
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, forbidden := range []string{"tajni-hes", "passwordHash", "passwordChangedAt", "active"} {
		if strings.Contains(s, forbidden) {
			t.Errorf("odgovor sadrzi %q: %s", forbidden, s)
		}
	}
}

func TestAccountGet_PasswordExpiresAt(t *testing.T) {
	got, err := accountSvc(&fakeAccountRepo{user: activeUser()}).Get(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if want := passwordChanged.Add(testMaxAge); !got.PasswordExpiresAt.Equal(want) {
		t.Errorf("PasswordExpiresAt = %v, ocekivano %v", got.PasswordExpiresAt, want)
	}
	if got.PasswordExpiresAt.Location() != time.UTC {
		t.Errorf("rok mora biti u UTC: %v", got.PasswordExpiresAt.Location())
	}
}

func TestAccountGet_PasswordExpiresAt_MatchesLoginRule(t *testing.T) {
	// Prikazani rok mora da se poklapa sa pravilom koje prijava primenjuje:
	// do tog trenutka lozinka vazi, odmah posle njega je istekla.
	u := activeUser()
	got, _ := accountSvc(&fakeAccountRepo{user: u}).Get(context.Background(), "u1")
	if u.PasswordExpired(got.PasswordExpiresAt, testMaxAge) {
		t.Error("na prikazanom roku lozinka jos mora da vazi")
	}
	if !u.PasswordExpired(got.PasswordExpiresAt.Add(time.Nanosecond), testMaxAge) {
		t.Error("odmah posle prikazanog roka lozinka mora biti istekla")
	}
}

func TestAccountGet_PasswordExpiresAt_FallsBackToCreatedAt(t *testing.T) {
	u := activeUser()
	u.PasswordChangedAt = time.Time{} // stari dokument bez polja
	got, _ := accountSvc(&fakeAccountRepo{user: u}).Get(context.Background(), "u1")
	if want := u.CreatedAt.Add(testMaxAge); !got.PasswordExpiresAt.Equal(want) {
		t.Errorf("PasswordExpiresAt = %v, ocekivano %v (od createdAt)", got.PasswordExpiresAt, want)
	}
}

func TestAccountGet_ShortDemoPeriod(t *testing.T) {
	svc, err := NewAccountService(&fakeAccountRepo{user: activeUser()}, 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Get(context.Background(), "u1")
	if want := passwordChanged.Add(2 * time.Minute); !got.PasswordExpiresAt.Equal(want) {
		t.Errorf("PasswordExpiresAt = %v, ocekivano %v", got.PasswordExpiresAt, want)
	}
}

func TestNewAccountService_RequiresPositiveMaxAge(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Hour} {
		if _, err := NewAccountService(&fakeAccountRepo{}, d); err == nil {
			t.Errorf("rok %v: ocekivana greska", d)
		}
	}
}
