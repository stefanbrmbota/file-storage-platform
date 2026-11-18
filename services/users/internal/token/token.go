// Paket token izdaje i proverava JWT (HS256) sa claimovima userId, role, jti i exp.
package token

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
)

// MinSecretLen je najmanja dužina tajne (HS256 traži bar 256 bita).
const MinSecretLen = 32

// ErrInvalidToken se vraća za svaki neispravan token (izmenjen, istekao,
// pogrešno potpisan, bez exp, sa nepoznatom ulogom). Namerno je jedna greška:
// ne otkriva razlog odbijanja.
var ErrInvalidToken = errors.New("token: nevazeci token")

// Claims su podaci iz proverenog tokena. ID (claim jti) i ExpiresAt služe za
// opoziv tokena pri odjavi; ID je prazan kod tokena izdatih pre uvođenja jti.
type Claims struct {
	UserID    string
	Role      model.Role
	ID        string
	ExpiresAt time.Time
}

type claims struct {
	Role model.Role `json:"role"`
	jwt.RegisteredClaims
}

// Manager izdaje i proverava tokene jednim tajnim ključem.
type Manager struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// NewManager vraća Manager. Tajna i trajanje dolaze iz konfiguracije (env);
// slaba tajna ili nepozitivno trajanje je greška.
func NewManager(secret string, ttl time.Duration) (*Manager, error) {
	if len(secret) < MinSecretLen {
		return nil, fmt.Errorf("token: tajna mora imati bar %d karaktera", MinSecretLen)
	}
	if ttl <= 0 {
		return nil, errors.New("token: trajanje mora biti pozitivno")
	}
	return &Manager{secret: []byte(secret), ttl: ttl, now: time.Now}, nil
}

// Issue izdaje potpisan token za korisnika. userId ide u standardni sub claim.
func (m *Manager) Issue(userID string, role model.Role) (string, error) {
	if userID == "" {
		return "", errors.New("token: userId je obavezan")
	}
	if !role.Valid() {
		return "", errors.New("token: nepoznata uloga")
	}
	jti, err := newID()
	if err != nil {
		return "", err
	}
	now := m.now()
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
	})
	s, err := t.SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("token: potpisivanje: %w", err)
	}
	return s, nil
}

// Parse proverava potpis, algoritam i rok važenja i vraća claimove.
// Prihvata samo HS256 (odbija alg=none i zamenu algoritma), traži exp,
// i odbija token bez userId ili sa nepoznatom ulogom.
func (m *Manager) Parse(tokenString string) (Claims, error) {
	var c claims
	_, err := jwt.ParseWithClaims(tokenString, &c,
		func(*jwt.Token) (any, error) { return m.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(m.now),
	)
	if err != nil || c.Subject == "" || !c.Role.Valid() {
		return Claims{}, ErrInvalidToken
	}
	return Claims{UserID: c.Subject, Role: c.Role, ID: c.ID, ExpiresAt: c.ExpiresAt.Time}, nil
}

// newID vraća slučajan 128-bitni identifikator tokena (heksadecimalno) za jti.
func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("token: generisanje jti: %w", err)
	}
	return hex.EncodeToString(b), nil
}
