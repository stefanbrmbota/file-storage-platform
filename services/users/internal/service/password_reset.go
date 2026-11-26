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
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/password"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

// resetMailTimeout ogranicava slanje mejla koje se radi u pozadini.
const resetMailTimeout = 30 * time.Second

// ErrInvalidEmail se vraca kada email nema ispravan oblik. Oblik ne otkriva da
// li nalog postoji, pa se ova greska sme prikazati korisniku.
var ErrInvalidEmail = errors.New("service: neispravan email")

// PasswordResetUsers je pristup nalozima potreban za reset lozinke.
type PasswordResetUsers interface {
	FindByEmail(ctx context.Context, email string) (model.User, error)
	AccountFinder
	repository.PasswordRepository
}

// OTPDeleter brise OTP korisnika (posle reseta lozinke stari OTP ne vazi).
type OTPDeleter interface {
	Delete(ctx context.Context, userID string) error
}

// PasswordResetConfig su podesavanja reseta lozinke (iz env promenljivih).
type PasswordResetConfig struct {
	BaseURL  string          // klijentska stranica za reset; token se dodaje kao #token=...
	TTL      time.Duration   // rok vazenja linka
	Cooldown time.Duration   // najkrace vreme izmedju dva mejla istom korisniku
	Params   password.Params // Argon2 parametri za novu lozinku
}

// PasswordResetService izvodi reset lozinke mejlom (US-15): zahtev za link
// (Request) i postavljanje nove lozinke linkom (Reset).
type PasswordResetService struct {
	users    PasswordResetUsers
	tokens   repository.PasswordResetRepository
	otps     OTPDeleter
	mailer   mail.Mailer
	params   password.Params
	baseURL  string
	ttl      time.Duration
	cooldown time.Duration
	now      func() time.Time
	// async izvrsava slanje mejla. Podrazumevano u pozadini, pa vreme odgovora
	// ne zavisi od toga da li nalog postoji (nema otkrivanja naloga).
	async func(func())
}

// NewPasswordResetService vraca servis za reset lozinke.
func NewPasswordResetService(users PasswordResetUsers, tokens repository.PasswordResetRepository, otps OTPDeleter, mailer mail.Mailer, cfg PasswordResetConfig) (*PasswordResetService, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("reset lozinke: RESET_URL_BASE je obavezan")
	}
	if cfg.TTL <= 0 {
		return nil, errors.New("reset lozinke: rok vazenja linka mora biti pozitivan")
	}
	if cfg.Cooldown < 0 {
		return nil, errors.New("reset lozinke: ogranicenje ucestalosti ne sme biti negativno")
	}
	if err := cfg.Params.Validate(); err != nil {
		return nil, fmt.Errorf("reset lozinke: %w", err)
	}
	return &PasswordResetService{
		users: users, tokens: tokens, otps: otps, mailer: mailer, params: cfg.Params,
		baseURL: cfg.BaseURL, ttl: cfg.TTL, cooldown: cfg.Cooldown,
		now:   func() time.Time { return time.Now().UTC() },
		async: func(f func()) { go f() },
	}, nil
}

// Request pravi kratkotrajan jednokratan token i salje mejl sa linkom za reset.
// Za nepoznat ili neaktivan nalog, kao i kad je mejl vec nedavno poslat
// (ogranicenje ucestalosti), ne radi nista i vraca nil: pozivalac uvek odgovara
// isto, pa se ne otkriva da li nalog postoji. Greska se vraca samo za neispravan
// oblik emaila i kvar baze. Novi token zamenjuje prethodni. Token se nikad ne loguje.
func (s *PasswordResetService) Request(ctx context.Context, email string) error {
	if len(validateEmail(email)) > 0 {
		return ErrInvalidEmail
	}
	u, err := s.users.FindByEmail(ctx, NormalizeEmail(email))
	switch {
	case err == nil:
	case errors.Is(err, repository.ErrNotFound):
		return nil
	default:
		return fmt.Errorf("reset lozinke: trazenje naloga: %w", err)
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
		return fmt.Errorf("reset lozinke: provera prethodnog tokena: %w", err)
	}

	raw, err := newToken()
	if err != nil {
		return err
	}
	expires := now.Add(s.ttl)
	tok := model.PasswordResetToken{UserID: u.ID, TokenHash: hashToken(raw), CreatedAt: now, ExpiresAt: expires}
	if err := s.tokens.Save(ctx, tok); err != nil {
		return fmt.Errorf("reset lozinke: upis tokena: %w", err)
	}

	link := s.baseURL + "#token=" + raw
	s.async(func() { s.sendResetMail(ctx, u, link, expires) })
	slog.Info("reset lozinke zatrazen", "userId", u.ID)
	return nil
}

// sendResetMail salje mejl sa linkom. Radi u pozadini, pa ima sopstveni kontekst
// sa rokom. Ako slanje ne uspe, token se brise: bez mejla je beskoristan, a
// ogranicenje ucestalosti bi korisniku blokiralo novi pokusaj. Link se ne loguje.
func (s *PasswordResetService) sendResetMail(parent context.Context, u model.User, link string, expires time.Time) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), resetMailTimeout)
	defer cancel()

	subject := "Reset lozinke"
	body := fmt.Sprintf("Zdravo %s,\n\nprimili smo zahtev za reset lozinke. Za izbor nove lozinke otvorite sledeći link:\n\n%s\n\nLink važi do %s i može se iskoristiti samo jednom.\nAko niste tražili reset, ignorišite ovaj mejl: vaša lozinka ostaje ista.\n",
		u.FirstName, link, expires.Format("02.01.2006. 15:04 MST"))
	if err := s.mailer.Send(ctx, u.Email, subject, body); err != nil {
		slog.Error("reset lozinke: slanje mejla nije uspelo", "userId", u.ID, "err", err)
		if derr := s.tokens.Delete(ctx, u.ID); derr != nil {
			slog.Error("reset lozinke: brisanje tokena nije uspelo", "userId", u.ID, "err", derr)
		}
	}
}
