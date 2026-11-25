package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

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

func activeUser() model.User {
	return model.User{
		ID: "u1", FirstName: "Ana", LastName: "Anić", Email: "ana@example.com",
		PasswordHash: "tajni-hes", Role: model.RoleUser, Active: true,
		QuotaBytes: 1000, UsedBytes: 250,
	}
}

func TestAccountGet_OK(t *testing.T) {
	repo := &fakeAccountRepo{user: activeUser()}
	got, err := NewAccountService(repo).Get(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	want := Account{
		ID: "u1", FirstName: "Ana", LastName: "Anić", Email: "ana@example.com",
		Role: model.RoleUser, QuotaBytes: 1000, UsedBytes: 250, RemainingBytes: 750,
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
	got, err := NewAccountService(&fakeAccountRepo{user: u}).Get(context.Background(), "u1")
	if err != nil || got.RemainingBytes != 0 {
		t.Fatalf("remaining = %d, err = %v", got.RemainingBytes, err)
	}
}

func TestAccountGet_FullQuota(t *testing.T) {
	u := activeUser()
	u.UsedBytes = u.QuotaBytes
	got, _ := NewAccountService(&fakeAccountRepo{user: u}).Get(context.Background(), "u1")
	if got.RemainingBytes != 0 {
		t.Errorf("remaining = %d", got.RemainingBytes)
	}
}

func TestAccountGet_NotFound(t *testing.T) {
	repo := &fakeAccountRepo{err: repository.ErrNotFound}
	if _, err := NewAccountService(repo).Get(context.Background(), "nema"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("err = %v, ocekivano ErrUserNotFound", err)
	}
}

func TestAccountGet_InactiveAccountIsNotFound(t *testing.T) {
	u := activeUser()
	u.Active = false
	if _, err := NewAccountService(&fakeAccountRepo{user: u}).Get(context.Background(), "u1"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("err = %v, ocekivano ErrUserNotFound", err)
	}
}

func TestAccountGet_BlankIDDoesNotQueryRepo(t *testing.T) {
	repo := &fakeAccountRepo{user: activeUser()}
	for _, id := range []string{"", "   "} {
		if _, err := NewAccountService(repo).Get(context.Background(), id); !errors.Is(err, ErrUserNotFound) {
			t.Errorf("id %q: err = %v", id, err)
		}
	}
	if repo.calls != 0 {
		t.Errorf("repo pozvan %d puta", repo.calls)
	}
}

func TestAccountGet_RepositoryError(t *testing.T) {
	boom := errors.New("baza pala")
	_, err := NewAccountService(&fakeAccountRepo{err: boom}).Get(context.Background(), "u1")
	if !errors.Is(err, boom) || errors.Is(err, ErrUserNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestAccount_DoesNotExposeSecrets(t *testing.T) {
	got, _ := NewAccountService(&fakeAccountRepo{user: activeUser()}).Get(context.Background(), "u1")
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, forbidden := range []string{"tajni-hes", "passwordHash", "password", "active"} {
		if strings.Contains(s, forbidden) {
			t.Errorf("odgovor sadrzi %q: %s", forbidden, s)
		}
	}
}
