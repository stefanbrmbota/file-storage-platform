package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/password"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

const (
	adminFirstName = "Admin"
	adminLastName  = "Admin"
)

// EnsureAdmin pravi admin nalog ako ne postoji nalog sa datim emailom (US-24).
// Lozinka se hesira paketom password. Vraca true ako je nalog kreiran, false
// ako je vec postojao. Ponovni poziv ne pravi duplikat.
func EnsureAdmin(ctx context.Context, repo repository.UserRepository, email, plainPassword string, quotaBytes int64, params password.Params) (bool, error) {
	if email == "" || plainPassword == "" {
		return false, errors.New("admin: email i lozinka su obavezni")
	}

	email = NormalizeEmail(email)
	_, err := repo.FindByEmail(ctx, email)
	switch {
	case err == nil:
		return false, nil
	case !errors.Is(err, repository.ErrNotFound):
		return false, err
	}

	hash, err := password.Hash(plainPassword, params)
	if err != nil {
		return false, fmt.Errorf("admin: heširanje lozinke: %w", err)
	}
	id, err := newID()
	if err != nil {
		return false, err
	}

	err = repo.Create(ctx, model.NewAdmin(id, adminFirstName, adminLastName, email, hash, quotaBytes, time.Now().UTC()))
	if errors.Is(err, repository.ErrEmailTaken) {
		// Druga instanca servisa je upravo napravila nalog.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("admin: generisanje id-a: %w", err)
	}
	return hex.EncodeToString(b), nil
}
