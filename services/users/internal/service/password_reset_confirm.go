package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/password"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

// ResetPasswordInput je zahtev za postavljanje nove lozinke linkom iz mejla.
type ResetPasswordInput struct {
	Token       string
	NewPassword string
	NewConfirm  string
}

// Reset postavlja novu lozinku na osnovu tokena iz mejla. Redosled: pravila nove
// lozinke se proveravaju PRE trosenja tokena (losa lozinka ne pali link), zatim se
// token trosi atomicno (jednokratnost), pa se upisuje novi hes. Ako upis ne uspe,
// token se vraca da korisnik moze ponovo da pokusa. Za razliku od promene lozinke
// (US-14), reset ne trazi minimalnu starost lozinke: korisnik je lozinku zaboravio.
// Posle uspeha passwordChangedAt je "sada" (pa je lozinka ponovo vazeca), stari
// OTP se brise, a vlasniku se salje obavestenje. Lozinke i token se nikad ne loguju.
func (s *PasswordResetService) Reset(ctx context.Context, in ResetPasswordInput) error {
	raw := strings.TrimSpace(in.Token)
	if raw == "" || len(raw) > maxTokenLen {
		return ErrInvalidToken
	}
	if problems := validatePassword(in.NewPassword, in.NewConfirm); len(problems) > 0 {
		return &PasswordRulesError{Problems: problems}
	}

	now := s.now()
	tok, err := s.tokens.Consume(ctx, hashToken(raw), now)
	if errors.Is(err, repository.ErrTokenNotFound) {
		return ErrInvalidToken
	}
	if err != nil {
		return fmt.Errorf("reset lozinke: uzimanje tokena: %w", err)
	}

	u, err := s.users.FindByID(ctx, tok.UserID)
	switch {
	case err == nil:
	case errors.Is(err, repository.ErrNotFound):
		return ErrInvalidToken // nalog je u medjuvremenu obrisan
	default:
		s.restore(ctx, tok)
		return fmt.Errorf("reset lozinke: trazenje naloga: %w", err)
	}
	if !u.Active {
		return ErrInvalidToken
	}

	newHash, err := password.Hash(in.NewPassword, s.params)
	if err != nil {
		s.restore(ctx, tok)
		return fmt.Errorf("reset lozinke: heširanje: %w", err)
	}
	if err := s.users.UpdatePassword(ctx, u.ID, u.PasswordHash, newHash, now); err != nil {
		switch {
		case errors.Is(err, repository.ErrPasswordConflict):
			s.restore(ctx, tok)
			return ErrPasswordChangeConflict
		case errors.Is(err, repository.ErrNotFound):
			return ErrInvalidToken
		}
		s.restore(ctx, tok)
		return fmt.Errorf("reset lozinke: upis: %w", err)
	}

	// Lozinka je vec promenjena; dalje se greske samo beleze.
	if err := s.otps.Delete(ctx, u.ID); err != nil && !errors.Is(err, repository.ErrOTPNotFound) {
		slog.Error("reset lozinke: brisanje OTP-a nije uspelo", "userId", u.ID, "err", err)
	}
	s.async(func() { s.sendChangedMail(ctx, u) })
	slog.Info("lozinka resetovana", "userId", u.ID)
	return nil
}

// restore vraca potroseni token da korisnik moze da klikne ponovo. Radi i kad je
// zahtev istekao (zato poseban kontekst); sam token se nikad ne loguje.
func (s *PasswordResetService) restore(ctx context.Context, tok model.PasswordResetToken) {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.tokens.Restore(rctx, tok); err != nil {
		slog.Error("reset lozinke: vracanje tokena nije uspelo", "userId", tok.UserID, "err", err)
	}
}

// sendChangedMail obavestava vlasnika da je lozinka resetovana (ako to nije on, odmah
// zna da je nalog u opasnosti). Neuspeh se samo belezi.
func (s *PasswordResetService) sendChangedMail(parent context.Context, u model.User) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), resetMailTimeout)
	defer cancel()

	subject := "Lozinka je promenjena"
	body := fmt.Sprintf("Zdravo %s,\n\nlozinka vašeg naloga je upravo resetovana.\nAko to niste bili vi, odmah se obratite administratoru sistema.\n", mailName(u.FirstName))
	if err := s.mailer.Send(ctx, u.Email, subject, body); err != nil {
		slog.Error("reset lozinke: slanje obavestenja nije uspelo", "userId", u.ID, "err", err)
	}
}
