package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/mail"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/password"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

// ErrInvalidCredentials se vraća za pogrešan email, pogrešnu lozinku i neaktivan
// nalog. Namerno je jedna greška: ne otkriva da li nalog postoji.
var ErrInvalidCredentials = errors.New("service: pogresni podaci za prijavu")

// OTPIssuer generiše i proverava OTP korisnika. Generate vraća kod; Verify
// vraća ErrInvalidOTP za svaki neuspeh i troši OTP pri uspehu.
type OTPIssuer interface {
	Generate(ctx context.Context, userID string) (string, error)
	Verify(ctx context.Context, userID, code string) error
}

// TokenIssuer izdaje JWT za prijavljenog korisnika.
type TokenIssuer interface {
	Issue(userID string, role model.Role) (string, error)
}

// ErrPasswordExpired se vraća kada je lozinka starija od dozvoljenog (US-16).
// Vraća se tek pošto je dokazano da korisnik zna lozinku (prvi korak) odnosno OTP
// (drugi korak), pa ne otkriva ništa o nalogu onome ko te podatke nema. Prijava je
// onemogućena dok se lozinka ne promeni resetom (US-15).
var ErrPasswordExpired = errors.New("service: lozinka je istekla")

// LoginService izvodi prijavu (US-05): prvi korak je provera lozinke i slanje
// OTP-a, drugi provera OTP-a i izdavanje JWT-a. Nalog sa isteklom lozinkom
// (US-16) se ne može prijaviti.
type LoginService struct {
	users     repository.UserRepository
	otp       OTPIssuer
	tokens    TokenIssuer
	mailer    mail.Mailer
	otpTTL    time.Duration
	maxAge    time.Duration
	dummyHash string
	now       func() time.Time
}

// NewLoginService vraća servis. otpTTL se koristi samo za tekst mejla. maxAge je
// najduže vreme važenja lozinke (PASSWORD_MAX_AGE) i mora biti pozitivan: ne
// može da se izostavi, pa se istek ne isključuje slučajno. Lažni heš se računa
// jednom, sa istim parametrima kao prave lozinke: kad nalog ne postoji, lozinka
// se ipak proverava protiv njega, pa vreme odgovora ne otkriva da li je email
// registrovan.
func NewLoginService(users repository.UserRepository, otp OTPIssuer, tokens TokenIssuer, mailer mail.Mailer, otpTTL time.Duration, params password.Params, maxAge time.Duration) (*LoginService, error) {
	if maxAge <= 0 {
		return nil, errors.New("prijava: rok važenja lozinke mora biti pozitivan")
	}
	dummy, err := password.Hash("lazna-lozinka-za-izjednacavanje-vremena", params)
	if err != nil {
		return nil, fmt.Errorf("prijava: lazni heš: %w", err)
	}
	return &LoginService{
		users: users, otp: otp, tokens: tokens, mailer: mailer, otpTTL: otpTTL, maxAge: maxAge, dummyHash: dummy,
		now: func() time.Time { return time.Now().UTC() },
	}, nil
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
	// Tek posle tačne lozinke: pogrešna lozinka isteklog naloga i dalje daje isti
	// odgovor kao svaka druga. OTP se ne generiše ni ne šalje.
	if u.PasswordExpired(s.now(), s.maxAge) {
		return ErrPasswordExpired
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

// Finish je drugi korak prijave: proverava OTP i, ako je tačan, izdaje JWT sa
// ulogom korisnika. Nepostojeći ili neaktivan nalog i svaki neispravan OTP
// vraćaju ErrInvalidOTP. Kod i token se ne loguju.
func (s *LoginService) Finish(ctx context.Context, email, code string) (string, error) {
	u, err := s.users.FindByEmail(ctx, NormalizeEmail(email))
	if errors.Is(err, repository.ErrNotFound) {
		return "", ErrInvalidOTP
	}
	if err != nil {
		return "", fmt.Errorf("prijava: trazenje korisnika: %w", err)
	}
	if !u.Active {
		return "", ErrInvalidOTP
	}

	if err := s.otp.Verify(ctx, u.ID, code); err != nil {
		if errors.Is(err, ErrInvalidOTP) {
			return "", ErrInvalidOTP
		}
		return "", fmt.Errorf("prijava: %w", err)
	}
	// Lozinka je mogla isteći između dva koraka. Provera je posle OTP-a, a ne pre:
	// ko nema važeći OTP ne sme da sazna stanje naloga.
	if u.PasswordExpired(s.now(), s.maxAge) {
		return "", ErrPasswordExpired
	}

	tok, err := s.tokens.Issue(u.ID, u.Role)
	if err != nil {
		return "", fmt.Errorf("prijava: izdavanje tokena: %w", err)
	}
	return tok, nil
}
