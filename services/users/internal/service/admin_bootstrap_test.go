package service

import (
	"context"
	"testing"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/password"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

type fakeRepo struct {
	users     map[string]model.User
	raceOnNew bool // simulira da je druga instanca upisala nalog izmedju Find i Create
}

func newFakeRepo() *fakeRepo { return &fakeRepo{users: map[string]model.User{}} }

func (f *fakeRepo) FindByEmail(_ context.Context, email string) (model.User, error) {
	if u, ok := f.users[email]; ok {
		return u, nil
	}
	return model.User{}, repository.ErrNotFound
}

func (f *fakeRepo) FindByID(_ context.Context, id string) (model.User, error) {
	for _, u := range f.users {
		if u.ID == id {
			return u, nil
		}
	}
	return model.User{}, repository.ErrNotFound
}

func (f *fakeRepo) Create(_ context.Context, u model.User) error {
	if _, ok := f.users[u.Email]; ok || f.raceOnNew {
		return repository.ErrEmailTaken
	}
	f.users[u.Email] = u
	return nil
}

// Slabi parametri da testovi budu brzi.
var testParams = password.Params{Memory: 64, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32}

func TestEnsureAdmin_CreatesAdmin(t *testing.T) {
	repo := newFakeRepo()
	created, err := EnsureAdmin(context.Background(), repo, "admin@example.com", "tajna-lozinka", testParams)
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	u := repo.users["admin@example.com"]
	if u.Role != model.RoleAdmin {
		t.Errorf("uloga = %q, want ADMIN", u.Role)
	}
	if u.PasswordHash == "tajna-lozinka" {
		t.Fatal("lozinka nije hesirana")
	}
	if ok, err := password.Verify("tajna-lozinka", u.PasswordHash); err != nil || !ok {
		t.Errorf("Verify = %v, %v", ok, err)
	}
}

func TestEnsureAdmin_Idempotent(t *testing.T) {
	repo := newFakeRepo()
	ctx := context.Background()
	if _, err := EnsureAdmin(ctx, repo, "admin@example.com", "tajna", testParams); err != nil {
		t.Fatal(err)
	}
	first := repo.users["admin@example.com"]

	created, err := EnsureAdmin(ctx, repo, "admin@example.com", "druga-lozinka", testParams)
	if err != nil || created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if len(repo.users) != 1 || repo.users["admin@example.com"].ID != first.ID {
		t.Fatal("ponovni poziv je izmenio ili dupliro nalog")
	}
}

func TestEnsureAdmin_RaceIsNotAnError(t *testing.T) {
	repo := newFakeRepo()
	repo.raceOnNew = true
	created, err := EnsureAdmin(context.Background(), repo, "admin@example.com", "tajna", testParams)
	if err != nil || created {
		t.Fatalf("created=%v err=%v", created, err)
	}
}

func TestEnsureAdmin_RequiresCredentials(t *testing.T) {
	if _, err := EnsureAdmin(context.Background(), newFakeRepo(), "", "x", testParams); err == nil {
		t.Error("prazan email treba da bude greska")
	}
	if _, err := EnsureAdmin(context.Background(), newFakeRepo(), "a@b.c", "", testParams); err == nil {
		t.Error("prazna lozinka treba da bude greska")
	}
}
