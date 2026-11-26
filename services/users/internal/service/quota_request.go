package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

var (
	// ErrQuotaNotIncreased se vraća kada tražena kvota nije veća od trenutne.
	ErrQuotaNotIncreased = errors.New("service: trazena kvota mora biti veca od trenutne")
	// ErrQuotaRequestPending se vraća kada korisnik već ima zahtev na čekanju.
	ErrQuotaRequestPending = errors.New("service: zahtev za kvotu je vec na cekanju")
	// ErrNoQuotaRequest se vraća kada korisnik još nije podneo nijedan zahtev.
	ErrNoQuotaRequest = errors.New("service: nema zahteva za kvotu")
)

// QuotaRequestService prima zahteve korisnika za povećanje kvote (US-21).
// Odluku o zahtevu donosi administrator (US-23).
type QuotaRequestService struct {
	users AccountFinder
	repo  repository.QuotaRequestRepository
	now   func() time.Time
}

// NewQuotaRequestService vraća servis za zahteve za povećanje kvote.
func NewQuotaRequestService(users AccountFinder, repo repository.QuotaRequestRepository) *QuotaRequestService {
	return &QuotaRequestService{users: users, repo: repo, now: func() time.Time { return time.Now().UTC() }}
}

// Submit podnosi zahtev za kvotu od requestedBytes. Vrednost mora biti u
// (0, MaxQuotaRequestBytes] (ErrInvalidQuotaRequest) i veća od trenutne kvote
// (ErrQuotaNotIncreased). Zahtev dobija status PENDING; ako korisnik već ima
// takav, vraća se ErrQuotaRequestPending. Prazan ID, nepostojeći i neaktivan
// nalog daju ErrUserNotFound.
func (s *QuotaRequestService) Submit(ctx context.Context, userID string, requestedBytes int64) (model.QuotaRequest, error) {
	if strings.TrimSpace(userID) == "" {
		return model.QuotaRequest{}, ErrUserNotFound
	}
	if requestedBytes <= 0 || requestedBytes > MaxQuotaRequestBytes {
		return model.QuotaRequest{}, ErrInvalidQuotaRequest
	}

	u, err := s.users.FindByID(ctx, userID)
	switch {
	case err == nil:
	case errors.Is(err, repository.ErrNotFound):
		return model.QuotaRequest{}, ErrUserNotFound
	default:
		return model.QuotaRequest{}, fmt.Errorf("zahtev za kvotu: trazenje naloga: %w", err)
	}
	if !u.Active {
		return model.QuotaRequest{}, ErrUserNotFound
	}
	if requestedBytes <= u.QuotaBytes {
		return model.QuotaRequest{}, ErrQuotaNotIncreased
	}

	id, err := newID()
	if err != nil {
		return model.QuotaRequest{}, fmt.Errorf("zahtev za kvotu: %w", err)
	}
	req := model.QuotaRequest{
		ID:             id,
		UserID:         u.ID,
		RequestedBytes: requestedBytes,
		Status:         model.QuotaRequestPending,
		CreatedAt:      s.now(),
	}
	if err := s.repo.Create(ctx, req); err != nil {
		if errors.Is(err, repository.ErrQuotaRequestPending) {
			return model.QuotaRequest{}, ErrQuotaRequestPending
		}
		return model.QuotaRequest{}, fmt.Errorf("zahtev za kvotu: %w", err)
	}
	slog.Info("zahtev za kvotu podnet", "userId", u.ID, "requestedBytes", requestedBytes)
	return req, nil
}

// Latest vraća najnoviji zahtev korisnika (njegov status). ErrNoQuotaRequest ako
// ga nema; prazan ID daje ErrUserNotFound.
func (s *QuotaRequestService) Latest(ctx context.Context, userID string) (model.QuotaRequest, error) {
	if strings.TrimSpace(userID) == "" {
		return model.QuotaRequest{}, ErrUserNotFound
	}
	req, err := s.repo.Latest(ctx, userID)
	switch {
	case err == nil:
		return req, nil
	case errors.Is(err, repository.ErrQuotaRequestNotFound):
		return model.QuotaRequest{}, ErrNoQuotaRequest
	default:
		return model.QuotaRequest{}, fmt.Errorf("zahtev za kvotu: citanje: %w", err)
	}
}
