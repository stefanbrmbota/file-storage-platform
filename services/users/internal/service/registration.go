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

// ErrEmailTaken se vraca kada nalog sa tim emailom vec postoji.
var ErrEmailTaken = errors.New("service: email je vec zauzet")

// ConfirmationIssuer izdaje token i salje mejl za potvrdu naloga.
type ConfirmationIssuer interface {
	Issue(ctx context.Context, u model.User) error
}

// RegistrationService kreira naloge novih korisnika (US-04).
type RegistrationService struct {
	repo       repository.UserRepository
	issuer     ConfirmationIssuer
	params     password.Params
	quotaBytes int64
	now        func() time.Time
}

// NewRegistrationService vraca servis. quotaBytes je podrazumevana kvota
// novog naloga (iz konfiguracije), a issuer salje link za potvrdu.
func NewRegistrationService(repo repository.UserRepository, issuer ConfirmationIssuer, params password.Params, quotaBytes int64) *RegistrationService {
	return &RegistrationService{
		repo:       repo,
		issuer:     issuer,
		params:     params,
		quotaBytes: quotaBytes,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

// NormalizeEmail svodi email na oblik u kom se cuva i trazi (bez razmaka, mala slova).
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// Register kreira neaktivan nalog sa USER ulogom i podrazumevanom kvotom.
// Pretpostavlja da je zahtev vec prosao ValidateRegistration. Lozinka se
// hesira paketom password; nikad se ne cuva ni loguje u izvornom obliku.
// Zauzet email vraca ErrEmailTaken (zastitu daje unique indeks u bazi).
func (s *RegistrationService) Register(ctx context.Context, in RegistrationInput) (model.User, error) {
	hash, err := password.Hash(in.Password, s.params)
	if err != nil {
		return model.User{}, fmt.Errorf("registracija: heširanje lozinke: %w", err)
	}
	id, err := newID()
	if err != nil {
		return model.User{}, err
	}

	u := model.NewUser(
		id,
		strings.TrimSpace(in.FirstName),
		strings.TrimSpace(in.LastName),
		NormalizeEmail(in.Email),
		hash,
		s.quotaBytes,
		s.now(),
	)
	if err := s.repo.Create(ctx, u); err != nil {
		if errors.Is(err, repository.ErrEmailTaken) {
			return model.User{}, ErrEmailTaken
		}
		return model.User{}, fmt.Errorf("registracija: upis korisnika: %w", err)
	}

	if err := s.issuer.Issue(ctx, u); err != nil {
		// Kompenzacija: bez poslatog linka nalog je neupotrebljiv, a email bi
		// ostao zauzet. Brisanjem korisnik moze da pokusa ponovo.
		dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if derr := s.repo.Delete(dctx, u.ID); derr != nil {
			slog.Error("registracija: brisanje naloga posle neuspele potvrde nije uspelo", "userId", u.ID, "err", derr)
		}
		return model.User{}, fmt.Errorf("registracija: slanje potvrde: %w", err)
	}
	return u, nil
}
