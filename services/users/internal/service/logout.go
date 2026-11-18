package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/token"
)

// ErrInvalidAuthToken se vraća za svaki token koji se ne može odjaviti: neispravan,
// istekao, pogrešno potpisan ili bez jti. Namerno je jedna greška.
var ErrInvalidAuthToken = errors.New("service: nevazeci token")

// TokenParser proverava JWT i vraća njegove claimove.
type TokenParser interface {
	Parse(tokenString string) (token.Claims, error)
}

// LogoutService izvodi odjavu (US-13): opoziva token upisom njegovog jti.
type LogoutService struct {
	tokens  TokenParser
	revoked repository.RevokedTokenRepository
}

// NewLogoutService vraća servis za odjavu.
func NewLogoutService(tokens TokenParser, revoked repository.RevokedTokenRepository) *LogoutService {
	return &LogoutService{tokens: tokens, revoked: revoked}
}

// Logout proverava token i upisuje njegov jti u listu opozvanih; zapis nestaje
// kad token istekne. Ponovna odjava istim tokenom je uspešna (idempotentno).
// Token se ne čuva ni ne loguje, samo jti.
func (s *LogoutService) Logout(ctx context.Context, tokenString string) error {
	c, err := s.tokens.Parse(tokenString)
	if err != nil || c.ID == "" {
		return ErrInvalidAuthToken
	}
	if err := s.revoked.Revoke(ctx, c.ID, c.ExpiresAt); err != nil {
		return fmt.Errorf("odjava: %w", err)
	}
	return nil
}
