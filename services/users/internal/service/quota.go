package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

const (
	// MaxQuotaRequestBytes je gornja granica jedne rezervacije (1 PiB); stiti od prekoracenja int64.
	MaxQuotaRequestBytes int64 = 1 << 50
	maxQuotaUserIDLen          = 128
)

var (
	// ErrInvalidQuotaRequest se vraca za prazan ili predug id, odnosno bytes van (0, MaxQuotaRequestBytes].
	ErrInvalidQuotaRequest = errors.New("service: neispravan zahtev za kvotu")
	// ErrUserNotFound se vraca kada korisnik ne postoji.
	ErrUserNotFound = errors.New("service: korisnik nije pronadjen")
	// ErrQuotaExceeded se vraca kada rezervacija ne staje u kvotu.
	ErrQuotaExceeded = errors.New("service: kvota prekoracena")
)

// QuotaService upravlja kvotom skladistenja (US-20).
type QuotaService struct {
	repo repository.QuotaRepository
}

// NewQuotaService vraca servis za kvotu.
func NewQuotaService(repo repository.QuotaRepository) *QuotaService {
	return &QuotaService{repo: repo}
}

// ReserveQuota rezervise bytes u kvoti korisnika i vraca preostali prostor.
func (s *QuotaService) ReserveQuota(ctx context.Context, userID string, bytes int64) (int64, error) {
	if strings.TrimSpace(userID) == "" || len(userID) > maxQuotaUserIDLen || bytes <= 0 || bytes > MaxQuotaRequestBytes {
		return 0, ErrInvalidQuotaRequest
	}
	remaining, err := s.repo.ReserveQuota(ctx, userID, bytes)
	switch {
	case err == nil:
		return remaining, nil
	case errors.Is(err, repository.ErrNotFound):
		return 0, ErrUserNotFound
	case errors.Is(err, repository.ErrQuotaExceeded):
		return 0, ErrQuotaExceeded
	default:
		return 0, fmt.Errorf("rezervacija kvote: %w", err)
	}
}
