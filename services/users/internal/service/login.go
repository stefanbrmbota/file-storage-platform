package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/mail"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/password"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

// ErrInvalidCredentials se vraća za pogrešan email, pogrešnu lozinku i neaktivan
// nalog. Namerno je jedna greška: ne otkriva da li nalog postoji.
var ErrInvalidCredentials = errors.New("service: pogresni podaci za prijavu")

// OTPIssuer generiše i čuva OTP za korisnika i vraća kod.
type OTPIssuer interface {
	Generate(ctx context.Context, userID string) (string, error)
}

// LoginService izvodi prvi korak prijave (US-05): provera lozinke i slanje OTP-a.
type LoginService struct {
	users     repository.UserRepository
	otp       OTPIssuer
	mailer    mail.Mailer
	otpTTL    time.Duration
	dummyHash string
}

// NewLoginService vraća servis. otpTTL se koristi samo za tekst mejla. Lažni
// heš se računa jednom, sa istim parametrima kao prave lozinke: kad nalog ne
// postoji, lozinka se ipak proverava protiv njega, pa vreme odgovora ne
// otkriva da li je email registrovan.
func NewLoginService(users repository.UserRepository, otp OTPIssuer, mailer mail.Mailer, otpTTL time.Duration, params password.Params) (*LoginService, error) {
	dummy, err := password.Hash("lazna-lozinka-za-izjednacavanje-vremena", params)
	if err != nil {
		return nil, fmt.Errorf("prijava: lazni heš: %w", err)
	}
	return &LoginService{users: users, otp: otp, mailer: mailer, otpTTL: otpTTL, dummyHash: dummy}, nil
}

// Start proverava email, lozinku i aktivnost naloga; ako je sve ispravno,
// generiše OTP i šalje ga na mejl korisnika. Za svaki neispravan slučaj
// vraća ErrInvalidCredentials. Lozinka i OTP se ne loguju.
func (s *LoginService) Start(ctx context.Context, email, plainPassword string) error {
	u, err := s.users.FindByEmail(ctx, NormalizeEmail(email))
	found := err == nil
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return fmt.Errorf("prijava: trazenje korisnika: %w", err)
	}

	hash := s.dummyHash
	if found {
		hash = u.PasswordHash
	}
	ok, verr := password.Verify(plainPassword, hash)
	if verr != nil {
		if found {
			return fmt.Errorf("prijava: provera lozinke: %w", verr)
		}
		return ErrInvalidCredentials
	}
	if !found || !ok || !u.Active {
		return ErrInvalidCredentials
	}

	code, err := s.otp.Generate(ctx, u.ID)
	if err != nil {
		return fmt.Errorf("prijava: %w", err)
	}

	subject := "Kod za prijavu"
	body := fmt.Sprintf("Zdravo %s,\n\nvaš kod za prijavu je: %s\n\nKod važi %d min i može se iskoristiti samo jednom.\nAko se niste prijavljivali, ignorišite ovaj mejl i razmislite o promeni lozinke.\n",
		u.FirstName, code, int(s.otpTTL.Minutes()))
	if err := s.mailer.Send(ctx, u.Email, subject, body); err != nil {
		return fmt.Errorf("prijava: slanje mejla: %w", err)
	}
	return nil
}
