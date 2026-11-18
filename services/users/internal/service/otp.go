package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

const (
	otpDigits       = 6
	otpSpace        = 1_000_000 // 10^otpDigits
	minOTPSecretLen = 16
)

// ErrInvalidOTP se vraća za pogrešan, istekao, već iskorišćen ili nepostojeći
// OTP i kad je potrošeno dozvoljenih pokušaja. Namerno je jedna greška: ne
// otkriva koji je slučaj.
var ErrInvalidOTP = errors.New("service: nevazeci otp")

// OTPService generiše, čuva i proverava jednokratne kodove (US-05).
type OTPService struct {
	repo        repository.OTPRepository
	secret      []byte
	ttl         time.Duration
	maxAttempts int
	now         func() time.Time
}

// NewOTPService vraća servis. secret je tajna za HMAC: prostor od 6 cifara je
// premali za običan heš, pa bi procurela baza dala kodove brute-force-om.
func NewOTPService(repo repository.OTPRepository, secret string, ttl time.Duration, maxAttempts int) (*OTPService, error) {
	if len(secret) < minOTPSecretLen {
		return nil, fmt.Errorf("otp: tajna mora imati bar %d karaktera", minOTPSecretLen)
	}
	if ttl <= 0 {
		return nil, errors.New("otp: rok vazenja mora biti pozitivan")
	}
	if maxAttempts <= 0 {
		return nil, errors.New("otp: broj pokusaja mora biti pozitivan")
	}
	return &OTPService{
		repo:        repo,
		secret:      []byte(secret),
		ttl:         ttl,
		maxAttempts: maxAttempts,
		now:         func() time.Time { return time.Now().UTC() },
	}, nil
}

// Generate pravi novi 6-cifreni kod za korisnika, upisuje njegov HMAC sa rokom
// važenja i vraća kod pozivaocu (da ga pošalje). Prethodni OTP korisnika se
// zamenjuje. Kod se nikad ne loguje.
func (s *OTPService) Generate(ctx context.Context, userID string) (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(otpSpace))
	if err != nil {
		return "", fmt.Errorf("otp: generisanje koda: %w", err)
	}
	code := fmt.Sprintf("%0*d", otpDigits, n.Int64())

	err = s.repo.Save(ctx, model.OTP{
		UserID:    userID,
		CodeHash:  s.hash(userID, code),
		ExpiresAt: s.now().Add(s.ttl),
	})
	if err != nil {
		return "", fmt.Errorf("otp: upis: %w", err)
	}
	return code, nil
}

// Verify proverava kod koji je korisnik uneo. Brojač pokušaja se povećava
// atomično pre poređenja, pa i tačan i pogrešan pokušaj troše brojač, a
// istovremeni zahtevi ne mogu zaobići ograničenje. Kad se potroše svi pokušaji
// ili OTP istekne, briše se. Tačan kod se troši brisanjem: ako ga je u međuvremenu
// potrošio drugi zahtev, vraća se ErrInvalidOTP. Za svaki neuspeh vraća
// ErrInvalidOTP; ostale greške su interne (baza).
func (s *OTPService) Verify(ctx context.Context, userID, code string) error {
	o, err := s.repo.IncrementAttempts(ctx, userID)
	if errors.Is(err, repository.ErrOTPNotFound) {
		return ErrInvalidOTP
	}
	if err != nil {
		return fmt.Errorf("otp: provera: %w", err)
	}

	if o.Attempts > s.maxAttempts || !s.now().Before(o.ExpiresAt) {
		s.discard(ctx, userID)
		return ErrInvalidOTP
	}

	if !hmac.Equal([]byte(o.CodeHash), []byte(s.hash(userID, code))) {
		if o.Attempts >= s.maxAttempts {
			s.discard(ctx, userID)
		}
		return ErrInvalidOTP
	}

	err = s.repo.Delete(ctx, userID)
	if errors.Is(err, repository.ErrOTPNotFound) {
		return ErrInvalidOTP // potrosio ga je drugi zahtev
	}
	if err != nil {
		return fmt.Errorf("otp: brisanje: %w", err)
	}
	return nil
}

// discard briše OTP koji više ne sme da se koristi. Radi i kad je zahtev
// otkazan (poseban kontekst); greška se ignoriše jer TTL indeks ionako briše.
func (s *OTPService) discard(ctx context.Context, userID string) {
	dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_ = s.repo.Delete(dctx, userID)
}

// hash je HMAC-SHA256 nad userID i kodom; userID u poruci vezuje heš za korisnika.
func (s *OTPService) hash(userID, code string) string {
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte(userID))
	m.Write([]byte{0})
	m.Write([]byte(code))
	return hex.EncodeToString(m.Sum(nil))
}
