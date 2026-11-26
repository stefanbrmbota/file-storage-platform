package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/mail"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

// magicMailTimeout ogranicava slanje mejla koje se radi u pozadini.
const magicMailTimeout = 30 * time.Second

// MagicLinkUsers je pristup nalozima potreban za slanje magičnog linka.
type MagicLinkUsers interface {
	FindByEmail(ctx context.Context, email string) (model.User, error)
}

// MagicLinkConfig su podešavanja magičnog linka (iz env promenljivih).
type MagicLinkConfig struct {
	BaseURL  string        // klijentska stranica za prijavu linkom; token se dodaje kao #token=...
	TTL      time.Duration // rok važenja linka
	Cooldown time.Duration // najkraće vreme između dva mejla istom korisniku
}

// MagicLinkService šalje magični link za povraćaj naloga (US-17). Prijavu
// linkom izvodi MagicLoginService.
type MagicLinkService struct {
	users    MagicLinkUsers
	tokens   repository.MagicLinkRepository
	mailer   mail.Mailer
	baseURL  string
	ttl      time.Duration
	cooldown time.Duration
	now      func() time.Time
	// async izvršava slanje mejla. Podrazumevano u pozadini, pa vreme odgovora
	// ne zavisi od toga da li nalog postoji (nema otkrivanja naloga).
	async func(func())
}

// NewMagicLinkService vraća servis za magični link.
func NewMagicLinkService(users MagicLinkUsers, tokens repository.MagicLinkRepository, mailer mail.Mailer, cfg MagicLinkConfig) (*MagicLinkService, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("magicni link: MAGIC_LINK_URL_BASE je obavezan")
	}
	if cfg.TTL <= 0 {
		return nil, errors.New("magicni link: rok vazenja linka mora biti pozitivan")
	}
	if cfg.Cooldown < 0 {
		return nil, errors.New("magicni link: ogranicenje ucestalosti ne sme biti negativno")
	}
	return &MagicLinkService{
		users: users, tokens: tokens, mailer: mailer,
		baseURL: cfg.BaseURL, ttl: cfg.TTL, cooldown: cfg.Cooldown,
		now:   func() time.Time { return time.Now().UTC() },
		async: func(f func()) { go f() },
	}, nil
}

// Request pravi kratkotrajan jednokratan token i šalje mejl sa magičnim linkom.
// Za nepoznat ili neaktivan nalog, kao i kad je mejl već nedavno poslat
// (ograničenje učestalosti), ne radi ništa i vraća nil: pozivalac uvek odgovara
// isto, pa se ne otkriva da li nalog postoji. Greška se vraća samo za neispravan
// oblik emaila i kvar baze. Novi token zamenjuje prethodni. Token se nikad ne loguje.
func (s *MagicLinkService) Request(ctx context.Context, email string) error {
	if len(validateEmail(email)) > 0 {
		return ErrInvalidEmail
	}
	u, err := s.users.FindByEmail(ctx, NormalizeEmail(email))
	switch {
	case err == nil:
	case errors.Is(err, repository.ErrNotFound):
		return nil
	default:
		return fmt.Errorf("magicni link: trazenje naloga: %w", err)
	}
	if !u.Active {
		return nil
	}

	now := s.now()
	existing, err := s.tokens.Get(ctx, u.ID)
	switch {
	case err == nil:
		if now.Sub(existing.CreatedAt) < s.cooldown {
			return nil
		}
	case errors.Is(err, repository.ErrTokenNotFound):
	default:
		return fmt.Errorf("magicni link: provera prethodnog tokena: %w", err)
	}

	raw, err := newToken()
	if err != nil {
		return err
	}
	expires := now.Add(s.ttl)
	tok := model.MagicLinkToken{UserID: u.ID, TokenHash: hashToken(raw), CreatedAt: now, ExpiresAt: expires}
	if err := s.tokens.Save(ctx, tok); err != nil {
		return fmt.Errorf("magicni link: upis tokena: %w", err)
	}

	link := s.baseURL + "#token=" + raw
	s.async(func() { s.sendLinkMail(ctx, u, link, expires) })
	slog.Info("magicni link zatrazen", "userId", u.ID)
	return nil
}

// sendLinkMail šalje mejl sa linkom. Radi u pozadini, pa ima sopstveni kontekst
// sa rokom. Ako slanje ne uspe, token se briše: bez mejla je beskoristan, a
// ograničenje učestalosti bi korisniku blokiralo novi pokušaj. Link se ne loguje.
func (s *MagicLinkService) sendLinkMail(parent context.Context, u model.User, link string, expires time.Time) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), magicMailTimeout)
	defer cancel()

	subject := "Povraćaj naloga"
	body := fmt.Sprintf("Zdravo %s,\n\nprimili smo zahtev za povraćaj naloga. Za prijavu bez lozinke otvorite sledeći link:\n\n%s\n\nLink važi do %s i može se iskoristiti samo jednom.\nAko niste tražili prijavu, ignorišite ovaj mejl: niko ne može da uđe u vaš nalog bez ovog linka.\n",
		mailName(u.FirstName), link, expires.Format("02.01.2006. 15:04 MST"))
	if err := s.mailer.Send(ctx, u.Email, subject, body); err != nil {
		slog.Error("magicni link: slanje mejla nije uspelo", "userId", u.ID, "err", err)
		if derr := s.tokens.Delete(ctx, u.ID); derr != nil {
			slog.Error("magicni link: brisanje tokena nije uspelo", "userId", u.ID, "err", derr)
		}
	}
}
