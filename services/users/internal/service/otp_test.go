package service

import (
	"context"
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
	s, err := NewOTPService(repo, testOTPSecret, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNewOTPService_Validates(t *testing.T) {
	if _, err := NewOTPService(newFakeOTPRepo(), "kratka", time.Minute); err == nil {
		t.Error("ocekivana greska za kratku tajnu")
	}
	if _, err := NewOTPService(newFakeOTPRepo(), testOTPSecret, 0); err == nil {
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
	other, _ := NewOTPService(newFakeOTPRepo(), "druga-tajna-0123456789", time.Minute)
	if s.hash("u1", "123456") == other.hash("u1", "123456") {
		t.Error("razlicita tajna mora dati razlicit heš")
	}
}
