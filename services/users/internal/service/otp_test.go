package service

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

type fakeOTPRepo struct {
	saved map[string]model.OTP
}

func newFakeOTPRepo() *fakeOTPRepo { return &fakeOTPRepo{saved: map[string]model.OTP{}} }

func (f *fakeOTPRepo) Save(_ context.Context, o model.OTP) error {
	f.saved[o.UserID] = o
	return nil
}

func (f *fakeOTPRepo) IncrementAttempts(_ context.Context, id string) (model.OTP, error) {
	o, ok := f.saved[id]
	if !ok {
		return model.OTP{}, repository.ErrOTPNotFound
	}
	o.Attempts++
	f.saved[id] = o
	return o, nil
}

func (f *fakeOTPRepo) Delete(_ context.Context, id string) error {
	if _, ok := f.saved[id]; !ok {
		return repository.ErrOTPNotFound
	}
	delete(f.saved, id)
	return nil
}

func (f *fakeOTPRepo) Get(_ context.Context, id string) (model.OTP, error) {
	o, ok := f.saved[id]
	if !ok {
		return model.OTP{}, repository.ErrOTPNotFound
	}
	return o, nil
}

const testOTPSecret = "test-secret-0123456789"

func newTestOTPService(t *testing.T, repo repository.OTPRepository) *OTPService {
	t.Helper()
	s, err := NewOTPService(repo, testOTPSecret, 5*time.Minute, 3)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNewOTPService_Validates(t *testing.T) {
	if _, err := NewOTPService(newFakeOTPRepo(), "kratka", time.Minute, 3); err == nil {
		t.Error("ocekivana greska za kratku tajnu")
	}
	if _, err := NewOTPService(newFakeOTPRepo(), testOTPSecret, 0, 3); err == nil {
		t.Error("ocekivana greska za nepozitivan rok")
	}
}

func TestGenerate_FormatAndStorage(t *testing.T) {
	repo := newFakeOTPRepo()
	s := newTestOTPService(t, repo)
	fixed := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return fixed }

	code, err := s.Generate(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^\d{6}$`).MatchString(code) {
		t.Fatalf("kod %q nije 6 cifara", code)
	}

	o := repo.saved["u1"]
	if o.CodeHash == "" || o.CodeHash == code {
		t.Errorf("heš nije ispravan: %q", o.CodeHash)
	}
	if o.CodeHash != s.hash("u1", code) {
		t.Error("heš se ne poklapa sa HMAC-om koda")
	}
	if want := fixed.Add(5 * time.Minute); !o.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v, ocekivano %v", o.ExpiresAt, want)
	}
	if o.Attempts != 0 {
		t.Errorf("Attempts = %d, ocekivano 0", o.Attempts)
	}
}

func TestGenerate_ReplacesPrevious(t *testing.T) {
	repo := newFakeOTPRepo()
	s := newTestOTPService(t, repo)
	ctx := context.Background()

	var first, second string
	// Dva uzastopna koda mogu slucajno biti jednaka (1 : 10^6); probaj dok se ne razlikuju.
	for i := 0; i < 5; i++ {
		first, _ = s.Generate(ctx, "u1")
		second, _ = s.Generate(ctx, "u1")
		if first != second {
			break
		}
	}
	if first == second {
		t.Skip("kodovi se nisu razlikovali u 5 pokusaja")
	}
	if repo.saved["u1"].CodeHash != s.hash("u1", second) {
		t.Error("novi OTP nije zamenio stari")
	}
	if len(repo.saved) != 1 {
		t.Errorf("ocekivan jedan OTP, ima %d", len(repo.saved))
	}
}

func TestHash_BoundToUserAndSecret(t *testing.T) {
	s := newTestOTPService(t, newFakeOTPRepo())
	if s.hash("u1", "123456") == s.hash("u2", "123456") {
		t.Error("isti kod za razlicite korisnike mora dati razlicit heš")
	}
	other, _ := NewOTPService(newFakeOTPRepo(), "druga-tajna-0123456789", time.Minute, 3)
	if s.hash("u1", "123456") == other.hash("u1", "123456") {
		t.Error("razlicita tajna mora dati razlicit heš")
	}
}

// otherCode vraća šestocifreni kod različit od zadatog.
func otherCode(code string) string {
	if code == "000000" {
		return "000001"
	}
	return "000000"
}

func TestNewOTPService_RejectsBadMaxAttempts(t *testing.T) {
	if _, err := NewOTPService(newFakeOTPRepo(), testOTPSecret, time.Minute, 0); err == nil {
		t.Error("ocekivana greska za nepozitivan broj pokusaja")
	}
}

func TestVerify_CorrectCodeConsumesOTP(t *testing.T) {
	repo := newFakeOTPRepo()
	s := newTestOTPService(t, repo)
	ctx := context.Background()
	code, _ := s.Generate(ctx, "u1")

	if err := s.Verify(ctx, "u1", code); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if _, ok := repo.saved["u1"]; ok {
		t.Error("OTP mora biti obrisan posle uspesne provere")
	}
	if err := s.Verify(ctx, "u1", code); !errors.Is(err, ErrInvalidOTP) {
		t.Errorf("ponovna upotreba: err = %v, ocekivano ErrInvalidOTP", err)
	}
}

func TestVerify_WrongCodeCountsAttempts(t *testing.T) {
	repo := newFakeOTPRepo()
	s := newTestOTPService(t, repo) // najvise 3 pokusaja
	ctx := context.Background()
	code, _ := s.Generate(ctx, "u1")
	wrong := otherCode(code)

	for i := 1; i <= 2; i++ {
		if err := s.Verify(ctx, "u1", wrong); !errors.Is(err, ErrInvalidOTP) {
			t.Fatalf("pokusaj %d: err = %v", i, err)
		}
		if repo.saved["u1"].Attempts != i {
			t.Errorf("Attempts = %d, ocekivano %d", repo.saved["u1"].Attempts, i)
		}
	}
	// Treci (poslednji) pogresan pokusaj brise OTP.
	if err := s.Verify(ctx, "u1", wrong); !errors.Is(err, ErrInvalidOTP) {
		t.Fatalf("treci pokusaj: err = %v", err)
	}
	if _, ok := repo.saved["u1"]; ok {
		t.Error("OTP mora biti obrisan kad se potrose pokusaji")
	}
	// Ni tacan kod vise ne prolazi.
	if err := s.Verify(ctx, "u1", code); !errors.Is(err, ErrInvalidOTP) {
		t.Errorf("tacan kod posle potrosenih pokusaja: err = %v", err)
	}
}

func TestVerify_CorrectCodeOnLastAttemptStillWorks(t *testing.T) {
	repo := newFakeOTPRepo()
	s := newTestOTPService(t, repo)
	ctx := context.Background()
	code, _ := s.Generate(ctx, "u1")
	wrong := otherCode(code)

	_ = s.Verify(ctx, "u1", wrong)
	_ = s.Verify(ctx, "u1", wrong)
	if err := s.Verify(ctx, "u1", code); err != nil { // 3. pokusaj, tacan
		t.Errorf("tacan kod na poslednjem pokusaju: %v", err)
	}
}

func TestVerify_Expired(t *testing.T) {
	repo := newFakeOTPRepo()
	s := newTestOTPService(t, repo)
	ctx := context.Background()
	code, _ := s.Generate(ctx, "u1")

	s.now = func() time.Time { return time.Now().UTC().Add(6 * time.Minute) }
	if err := s.Verify(ctx, "u1", code); !errors.Is(err, ErrInvalidOTP) {
		t.Errorf("err = %v, ocekivano ErrInvalidOTP", err)
	}
	if _, ok := repo.saved["u1"]; ok {
		t.Error("istekao OTP mora biti obrisan")
	}
}

func TestVerify_NoOTP(t *testing.T) {
	s := newTestOTPService(t, newFakeOTPRepo())
	if err := s.Verify(context.Background(), "nema", "123456"); !errors.Is(err, ErrInvalidOTP) {
		t.Errorf("err = %v, ocekivano ErrInvalidOTP", err)
	}
}

func TestVerify_CodeBoundToUser(t *testing.T) {
	repo := newFakeOTPRepo()
	s := newTestOTPService(t, repo)
	ctx := context.Background()
	code, _ := s.Generate(ctx, "u1")
	_, _ = s.Generate(ctx, "u2")

	// Kod korisnika u1 ne sme da prodje za u2 (osim ako su slucajno isti).
	if repo.saved["u2"].CodeHash == repo.saved["u1"].CodeHash {
		t.Skip("kodovi su slucajno isti")
	}
	if err := s.Verify(ctx, "u2", code); !errors.Is(err, ErrInvalidOTP) {
		t.Errorf("err = %v, ocekivano ErrInvalidOTP", err)
	}
}
