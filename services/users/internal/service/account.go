package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

// Account su podaci o nalogu za prikaz samom korisniku (US-06): profil, kvota,
// zauzeće i preostali prostor, te kada lozinka ističe (US-16). Namerno nema heš
// lozinke ni interna polja.
type Account struct {
	ID             string     `json:"id"`
	FirstName      string     `json:"firstName"`
	LastName       string     `json:"lastName"`
	Email          string     `json:"email"`
	Role           model.Role `json:"role"`
	QuotaBytes     int64      `json:"quotaBytes"`
	UsedBytes      int64      `json:"usedBytes"`
	RemainingBytes int64      `json:"remainingBytes"`
	// PasswordExpiresAt je trenutak (UTC) posle kog prijava više nije moguća dok se
	// lozinka ne promeni: poslednja promena + PASSWORD_MAX_AGE.
	PasswordExpiresAt time.Time `json:"passwordExpiresAt"`
}

// AccountFinder je pristup nalogu po ID-ju.
type AccountFinder interface {
	FindByID(ctx context.Context, id string) (model.User, error)
}

// AccountService vraća podatke o nalogu prijavljenog korisnika (US-06).
type AccountService struct {
	users  AccountFinder
	maxAge time.Duration
}

// NewAccountService vraća servis za podatke o nalogu. maxAge je najduže vreme
// važenja lozinke (PASSWORD_MAX_AGE, isto kao u prijavi) i mora biti pozitivan,
// da prikazani rok uvek odgovara pravilu koje prijava stvarno primenjuje.
func NewAccountService(users AccountFinder, maxAge time.Duration) (*AccountService, error) {
	if maxAge <= 0 {
		return nil, errors.New("podaci o nalogu: rok važenja lozinke mora biti pozitivan")
	}
	return &AccountService{users: users, maxAge: maxAge}, nil
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

		PasswordExpiresAt: u.PasswordExpiresAt(s.maxAge).UTC(),
	}, nil
}
