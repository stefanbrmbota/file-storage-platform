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

// MagicLoginService izvodi prijavu magičnim linkom (US-17): troši token iz
// linka i izdaje JWT. Link se traži kroz MagicLinkService.Request.
type MagicLoginService struct {
	users  AccountFinder
	tokens repository.MagicLinkRepository
	issuer TokenIssuer
	now    func() time.Time
}

// NewMagicLoginService vraća servis za prijavu magičnim linkom.
func NewMagicLoginService(users AccountFinder, tokens repository.MagicLinkRepository, issuer TokenIssuer) *MagicLoginService {
	return &MagicLoginService{
		users: users, tokens: tokens, issuer: issuer,
		now: func() time.Time { return time.Now().UTC() },
	}
}

// Login prijavljuje korisnika na osnovu tokena iz magičnog linka. Token se
// troši atomično (jednokratnost): drugi klik na isti link, kao i istekao ili
// nepoznat token, vraćaju ErrInvalidToken (jedna greška za sve slučajeve, da se
// ne otkriva zašto link ne važi). Isto važi ako je nalog u međuvremenu obrisan
// ili deaktiviran. Ako izdavanje JWT-a ili čitanje naloga zataji zbog kvara,
// token se vraća da korisnik može ponovo da klikne.
//
// Istekla lozinka (US-16) ne blokira prijavu: magični link je upravo način da se
// pristup povrati bez lozinke, a korisnik posle prijave menja lozinku. Token i
// JWT se nikad ne loguju.
func (s *MagicLoginService) Login(ctx context.Context, rawToken string) (string, error) {
	raw := strings.TrimSpace(rawToken)
	if raw == "" || len(raw) > maxTokenLen {
		return "", ErrInvalidToken
	}

	tok, err := s.tokens.Consume(ctx, hashToken(raw), s.now())
	if errors.Is(err, repository.ErrTokenNotFound) {
		return "", ErrInvalidToken
	}
	if err != nil {
		return "", fmt.Errorf("magicni link: uzimanje tokena: %w", err)
	}

	u, err := s.users.FindByID(ctx, tok.UserID)
	switch {
	case err == nil:
	case errors.Is(err, repository.ErrNotFound):
		return "", ErrInvalidToken // nalog je u medjuvremenu obrisan
	default:
		s.restore(ctx, tok)
		return "", fmt.Errorf("magicni link: trazenje naloga: %w", err)
	}
	if !u.Active {
		return "", ErrInvalidToken
	}

	jwt, err := s.issuer.Issue(u.ID, u.Role)
	if err != nil {
		s.restore(ctx, tok)
		return "", fmt.Errorf("magicni link: izdavanje tokena: %w", err)
	}
	slog.Info("prijava magicnim linkom", "userId", u.ID)
	return jwt, nil
}

// restore vraća potrošen token da korisnik može da klikne ponovo. Radi i kad je
// zahtev istekao (zato poseban kontekst); sam token se nikad ne loguje.
func (s *MagicLoginService) restore(ctx context.Context, tok model.MagicLinkToken) {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.tokens.Restore(rctx, tok); err != nil {
		slog.Error("magicni link: vracanje tokena nije uspelo", "userId", tok.UserID, "err", err)
	}
}
