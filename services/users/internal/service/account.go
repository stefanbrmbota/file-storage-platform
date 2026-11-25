package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

// Account su podaci o nalogu za prikaz samom korisniku (US-06): profil, kvota,
// zauzeće i preostali prostor. Namerno nema heš lozinke ni interna polja.
type Account struct {
	ID             string     `json:"id"`
	FirstName      string     `json:"firstName"`
	LastName       string     `json:"lastName"`
	Email          string     `json:"email"`
	Role           model.Role `json:"role"`
	QuotaBytes     int64      `json:"quotaBytes"`
	UsedBytes      int64      `json:"usedBytes"`
	RemainingBytes int64      `json:"remainingBytes"`
}

// AccountFinder je pristup nalogu po ID-ju.
type AccountFinder interface {
	FindByID(ctx context.Context, id string) (model.User, error)
}

// AccountService vraća podatke o nalogu prijavljenog korisnika (US-06).
type AccountService struct {
	users AccountFinder
}

// NewAccountService vraća servis za podatke o nalogu.
func NewAccountService(users AccountFinder) *AccountService {
	return &AccountService{users: users}
}

// Get vraća nalog sa kvotom i preostalim prostorom (kvota - zauzeće, najmanje 0).
// Prazan ID, nepostojeći i neaktivan nalog daju ErrUserNotFound: token neaktivnog
// naloga ne otkriva podatke.
func (s *AccountService) Get(ctx context.Context, userID string) (Account, error) {
	if strings.TrimSpace(userID) == "" {
		return Account{}, ErrUserNotFound
	}
	u, err := s.users.FindByID(ctx, userID)
	switch {
	case err == nil:
	case errors.Is(err, repository.ErrNotFound):
		return Account{}, ErrUserNotFound
	default:
		return Account{}, fmt.Errorf("podaci o nalogu: %w", err)
	}
	if !u.Active {
		return Account{}, ErrUserNotFound
	}
	return Account{
		ID:             u.ID,
		FirstName:      u.FirstName,
		LastName:       u.LastName,
		Email:          u.Email,
		Role:           u.Role,
		QuotaBytes:     u.QuotaBytes,
		UsedBytes:      u.UsedBytes,
		RemainingBytes: max(0, u.QuotaBytes-u.UsedBytes),
	}, nil
}
