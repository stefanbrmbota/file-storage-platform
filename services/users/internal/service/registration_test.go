package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/password"
)

const testQuota int64 = 5 << 30

func TestRegister_CreatesInactiveAccount(t *testing.T) {
	repo := newFakeRepo()
	svc := NewRegistrationService(repo, testParams, testQuota)

	in := validInput()
	in.Email = "  Pera@Example.COM "
	u, err := svc.Register(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}

	stored, ok := repo.users["pera@example.com"]
	if !ok {
		t.Fatalf("nalog nije upisan pod normalizovanim emailom: %v", repo.users)
	}
	if stored.ID != u.ID || stored.ID == "" {
		t.Error("vraćeni i upisani nalog se razlikuju")
	}
	if stored.Active {
		t.Error("nalog mora biti neaktivan")
	}
	if stored.Role != model.RoleUser {
		t.Errorf("uloga = %q, očekivano USER", stored.Role)
	}
	if stored.QuotaBytes != testQuota || stored.UsedBytes != 0 {
		t.Errorf("kvota = %d, iskorišćeno = %d", stored.QuotaBytes, stored.UsedBytes)
	}
	if stored.PasswordChangedAt.IsZero() {
		t.Error("passwordChangedAt nije postavljen")
	}
	if stored.PasswordHash == in.Password || strings.Contains(stored.PasswordHash, in.Password) {
		t.Fatal("lozinka nije heširana")
	}
	if ok, err := password.Verify(in.Password, stored.PasswordHash); err != nil || !ok {
		t.Errorf("Verify = %v, %v", ok, err)
	}
}

func TestRegister_EmailTaken(t *testing.T) {
	repo := newFakeRepo()
	svc := NewRegistrationService(repo, testParams, testQuota)
	ctx := context.Background()

	if _, err := svc.Register(ctx, validInput()); err != nil {
		t.Fatal(err)
	}
	in := validInput()
	in.Email = "PERA@example.com" // isti email, druga velika/mala slova
	if _, err := svc.Register(ctx, in); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("err = %v, očekivano ErrEmailTaken", err)
	}
	if len(repo.users) != 1 {
		t.Errorf("upisano %d naloga, očekivano 1", len(repo.users))
	}
}

func TestRegister_RaceReturnsEmailTaken(t *testing.T) {
	repo := newFakeRepo()
	repo.raceOnNew = true
	svc := NewRegistrationService(repo, testParams, testQuota)
	if _, err := svc.Register(context.Background(), validInput()); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("err = %v, očekivano ErrEmailTaken", err)
	}
}
