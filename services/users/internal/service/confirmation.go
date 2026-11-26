package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/mail"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

const maxTokenLen = 128

var (
	// ErrInvalidToken se vraća za token koji ne postoji, istekao je ili je već
	// iskorišćen. Namerno je jedna greška: ne otkriva koji je slučaj.
	ErrInvalidToken = errors.New("service: nevazeci token")
	// ErrUpstream se vraća kada zavisni servis (Files) nije uspeo; token ostaje
	// iskoristiv, pa korisnik može da pokuša ponovo.
	ErrUpstream = errors.New("service: zavisni servis nije dostupan")
)

// FilesClient je poziv ka Files servisu koji kreira korenski folder (idempotentno).
type FilesClient interface {
	CreateRootFolder(ctx context.Context, userID string) error
}

// ConfirmationService izdaje i troši tokene za potvrdu registracije (US-04).
type ConfirmationService struct {
	users   repository.UserRepository
	tokens  repository.TokenRepository
	mailer  mail.Mailer
	files   FilesClient
	ttl     time.Duration
	baseURL string
	now     func() time.Time
}

// NewConfirmationService vraća servis. baseURL je adresa klijentske stranice
// za potvrdu; token ide u fragment (#token=...), da ne završi u logovima.
func NewConfirmationService(users repository.UserRepository, tokens repository.TokenRepository, mailer mail.Mailer, files FilesClient, ttl time.Duration, baseURL string) *ConfirmationService {
	return &ConfirmationService{
		users:   users,
		tokens:  tokens,
		mailer:  mailer,
		files:   files,
		ttl:     ttl,
		baseURL: baseURL,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// Issue pravi jednokratni token za nalog i šalje mejl sa linkom.
func (s *ConfirmationService) Issue(ctx context.Context, u model.User) error {
	raw, err := newToken()
	if err != nil {
		return err
	}
	expires := s.now().Add(s.ttl)
	if err := s.tokens.Create(ctx, model.ConfirmationToken{ID: hashToken(raw), UserID: u.ID, ExpiresAt: expires}); err != nil {
		return fmt.Errorf("potvrda: upis tokena: %w", err)
	}

	link := s.baseURL + "#token=" + raw
	subject := "Potvrda registracije"
	body := fmt.Sprintf("Zdravo %s,\n\nza aktivaciju naloga otvorite sledeći link:\n\n%s\n\nLink važi do %s i može se iskoristiti samo jednom.\nAko se niste registrovali, ignorišite ovaj mejl.\n",
		mailName(u.FirstName), link, expires.Format("02.01.2006. 15:04 MST"))
	if err := s.mailer.Send(ctx, u.Email, subject, body); err != nil {
		return fmt.Errorf("potvrda: slanje mejla: %w", err)
	}
	return nil
}

// Confirm troši token, traži korenski folder od Files servisa i aktivira nalog.
// Token se troši atomično, pa ga dva istovremena klika ne mogu oba iskoristiti.
// Ako Files ili aktivacija ne uspeju, token se vraća i nalog ostaje neaktivan;
// pošto je kreiranje korena idempotentno, ponovni pokušaj je bezbedan.
func (s *ConfirmationService) Confirm(ctx context.Context, rawToken string) error {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" || len(rawToken) > maxTokenLen {
		return ErrInvalidToken
	}

	tok, err := s.tokens.Consume(ctx, hashToken(rawToken), s.now())
	if errors.Is(err, repository.ErrTokenNotFound) {
		return ErrInvalidToken
	}
	if err != nil {
		return fmt.Errorf("potvrda: uzimanje tokena: %w", err)
	}

	if err := s.files.CreateRootFolder(ctx, tok.UserID); err != nil {
		s.restore(ctx, tok)
		slog.Error("potvrda: kreiranje korenskog foldera nije uspelo", "userId", tok.UserID, "err", err)
		return fmt.Errorf("%w: %v", ErrUpstream, err)
	}

	err = s.users.Activate(ctx, tok.UserID)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrInvalidToken // nalog je u međuvremenu obrisan
	}
	if err != nil {
		s.restore(ctx, tok)
		return fmt.Errorf("potvrda: aktivacija: %w", err)
	}
	return nil
}

// restore vraća potrošeni token da korisnik može da klikne ponovo. Radi i kad
// je zahtev istekao (zato poseban kontekst); sam token se nikad ne loguje.
func (s *ConfirmationService) restore(ctx context.Context, tok model.ConfirmationToken) {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.tokens.Create(rctx, tok); err != nil {
		slog.Error("potvrda: vraćanje tokena nije uspelo", "userId", tok.UserID, "err", err)
	}
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("potvrda: generisanje tokena: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
