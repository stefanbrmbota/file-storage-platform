package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/password"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

var (
	// ErrWrongPassword se vraca kada trenutna lozinka nije tacna (ili je prazna/predugacka).
	ErrWrongPassword = errors.New("service: pogresna trenutna lozinka")
	// ErrPasswordChangeConflict se vraca kada je lozinka u medjuvremenu vec promenjena.
	ErrPasswordChangeConflict = errors.New("service: lozinka je u medjuvremenu promenjena")
)

// PasswordRulesError se vraca kada nova lozinka ne zadovoljava pravila
// (jaka lozinka, poklapanje sa ponovljenom, razlicita od trenutne). Problems
// su poruke za korisnika i nikad ne sadrze vrednost lozinke.
type PasswordRulesError struct {
	Problems []string
}

func (e *PasswordRulesError) Error() string {
	return "service: nova lozinka nije prihvatljiva: " + strings.Join(e.Problems, "; ")
}

// PasswordTooNewError se vraca kada od poslednje promene lozinke nije proslo
// dovoljno vremena. AllowedAt je najraniji trenutak sledece promene (UTC).
type PasswordTooNewError struct {
	AllowedAt time.Time
}

func (e *PasswordTooNewError) Error() string {
	return "service: lozinka je nedavno promenjena, sledeca promena je moguca tek " + e.AllowedAt.Format(time.RFC3339)
}

// PasswordStore je pristup nalogu i njegovoj lozinki.
type PasswordStore interface {
	AccountFinder
	repository.PasswordRepository
}

// ChangePasswordInput je zahtev za promenu lozinke prijavljenog korisnika.
// UserID dolazi iz zaglavlja koje postavlja gateway, a Token je JWT kojim je
// zahtev poslat (mora pripadati istom korisniku i biva opozvan posle uspeha).
type ChangePasswordInput struct {
	UserID          string
	Token           string
	CurrentPassword string
	NewPassword     string
	NewConfirm      string
}

// PasswordService menja lozinku prijavljenog korisnika (US-14).
type PasswordService struct {
	store   PasswordStore
	tokens  TokenParser
	revoked repository.RevokedTokenRepository
	params  password.Params
	minAge  time.Duration
	now     func() time.Time
}

// NewPasswordService vraca servis. minAge je najkrace vreme izmedju dve
// promene lozinke (PASSWORD_MIN_AGE; specifikacija: najmanje 1 dan).
func NewPasswordService(store PasswordStore, tokens TokenParser, revoked repository.RevokedTokenRepository, params password.Params, minAge time.Duration) (*PasswordService, error) {
	if err := params.Validate(); err != nil {
		return nil, fmt.Errorf("promena lozinke: %w", err)
	}
	if minAge < 0 {
		return nil, errors.New("promena lozinke: najmanja starost lozinke ne sme biti negativna")
	}
	return &PasswordService{
		store: store, tokens: tokens, revoked: revoked, params: params, minAge: minAge,
		now: func() time.Time { return time.Now().UTC() },
	}, nil
}

// Change menja lozinku. Redosled provera: token (isti korisnik, ima jti), pravila
// nove lozinke, tacna trenutna lozinka, najmanja starost lozinke, nova razlicita
// od trenutne. Tek tada se upisuje novi heš (compare-and-set), a token kojim je
// zahtev poslat se opoziva, pa se korisnik ponovo prijavljuje novom lozinkom.
// Lozinke se nikad ne loguju.
func (s *PasswordService) Change(ctx context.Context, in ChangePasswordInput) error {
	claims, err := s.tokens.Parse(in.Token)
	if err != nil || claims.ID == "" || claims.UserID != in.UserID {
		return ErrInvalidAuthToken
	}

	if problems := validatePassword(in.NewPassword, in.NewConfirm); len(problems) > 0 {
		return &PasswordRulesError{Problems: problems}
	}

	u, err := s.store.FindByID(ctx, in.UserID)
	switch {
	case err == nil:
	case errors.Is(err, repository.ErrNotFound):
		return ErrUserNotFound
	default:
		return fmt.Errorf("promena lozinke: trazenje naloga: %w", err)
	}
	if !u.Active {
		return ErrUserNotFound
	}

	// Pogresna ili besmislena trenutna lozinka se odbija pre skupog heširanja.
	if in.CurrentPassword == "" || utf8.RuneCountInString(in.CurrentPassword) > maxPasswordLen {
		return ErrWrongPassword
	}
	ok, err := password.Verify(in.CurrentPassword, u.PasswordHash)
	if err != nil {
		return fmt.Errorf("promena lozinke: provera trenutne lozinke: %w", err)
	}
	if !ok {
		return ErrWrongPassword
	}

	now := s.now()
	if allowedAt := u.PasswordChangedAt.Add(s.minAge); now.Before(allowedAt) {
		return &PasswordTooNewError{AllowedAt: allowedAt.UTC()}
	}

	same, err := password.Verify(in.NewPassword, u.PasswordHash)
	if err != nil {
		return fmt.Errorf("promena lozinke: poredjenje sa trenutnom: %w", err)
	}
	if same {
		return &PasswordRulesError{Problems: []string{"nova lozinka mora biti drugačija od trenutne"}}
	}

	newHash, err := password.Hash(in.NewPassword, s.params)
	if err != nil {
		return fmt.Errorf("promena lozinke: heširanje: %w", err)
	}
	if err := s.store.UpdatePassword(ctx, u.ID, u.PasswordHash, newHash, now); err != nil {
		switch {
		case errors.Is(err, repository.ErrPasswordConflict):
			return ErrPasswordChangeConflict
		case errors.Is(err, repository.ErrNotFound):
			return ErrUserNotFound
		}
		return fmt.Errorf("promena lozinke: upis: %w", err)
	}

	// Lozinka je vec promenjena; ako opoziv tokena ne uspe, token ionako istice
	// (JWT_TTL), pa se zahtev ne odbija, ali se greska belezi.
	if err := s.revoked.Revoke(ctx, claims.ID, claims.ExpiresAt); err != nil {
		slog.Error("opoziv tokena posle promene lozinke nije uspeo", "err", err, "userId", u.ID)
	}
	slog.Info("lozinka promenjena", "userId", u.ID)
	return nil
}
