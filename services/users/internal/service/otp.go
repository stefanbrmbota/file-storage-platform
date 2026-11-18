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

// OTPService generiše i čuva jednokratne kodove (US-05).
type OTPService struct {
	repo   repository.OTPRepository
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// NewOTPService vraća servis. secret je tajna za HMAC: prostor od 6 cifara je
// premali za običan heš, pa bi procurela baza dala kodove brute-force-om.
func NewOTPService(repo repository.OTPRepository, secret string, ttl time.Duration) (*OTPService, error) {
	if len(secret) < minOTPSecretLen {
		return nil, fmt.Errorf("otp: tajna mora imati bar %d karaktera", minOTPSecretLen)
	}
	if ttl <= 0 {
		return nil, errors.New("otp: rok vazenja mora biti pozitivan")
	}
	return &OTPService{
		repo:   repo,
		secret: []byte(secret),
		ttl:    ttl,
		now:    func() time.Time { return time.Now().UTC() },
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

// hash je HMAC-SHA256 nad userID i kodom; userID u poruci vezuje heš za korisnika.
func (s *OTPService) hash(userID, code string) string {
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte(userID))
	m.Write([]byte{0})
	m.Write([]byte(code))
	return hex.EncodeToString(m.Sum(nil))
}
