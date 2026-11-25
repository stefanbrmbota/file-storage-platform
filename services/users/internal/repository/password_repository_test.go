package repository

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestUpdatePassword_Success(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	seedUser(t, repo, "u1", 1000) // passwordHash = "hash", passwordChangedAt = trenutak registracije
	changedAt := time.Now().UTC().Truncate(time.Millisecond).Add(time.Hour)

	if err := repo.UpdatePassword(ctx, "u1", "hash", "novi-hash", changedAt); err != nil {
		t.Fatal(err)
	}
	u, err := repo.FindByID(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if u.PasswordHash != "novi-hash" {
		t.Errorf("passwordHash = %q", u.PasswordHash)
	}
	if !u.PasswordChangedAt.Equal(changedAt) {
		t.Errorf("passwordChangedAt = %v, ocekivano %v", u.PasswordChangedAt, changedAt)
	}
}

func TestUpdatePassword_OtherFieldsUntouched(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	seedUser(t, repo, "u1", 1000)
	before, _ := repo.FindByID(ctx, "u1")

	if err := repo.UpdatePassword(ctx, "u1", "hash", "novi-hash", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	after, _ := repo.FindByID(ctx, "u1")
	if after.Email != before.Email || after.Role != before.Role || after.QuotaBytes != before.QuotaBytes ||
		after.UsedBytes != before.UsedBytes || after.Active != before.Active || !after.CreatedAt.Equal(before.CreatedAt) {
		t.Errorf("izmenjena su i druga polja: pre %+v, posle %+v", before, after)
	}
}

func TestUpdatePassword_WrongOldHash_NoChange(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	seedUser(t, repo, "u1", 1000)
	before, _ := repo.FindByID(ctx, "u1")

	err := repo.UpdatePassword(ctx, "u1", "pogresan-stari-hash", "novi-hash", time.Now().UTC())
	if !errors.Is(err, ErrPasswordConflict) {
		t.Fatalf("err = %v, ocekivano ErrPasswordConflict", err)
	}
	after, _ := repo.FindByID(ctx, "u1")
	if after.PasswordHash != before.PasswordHash || !after.PasswordChangedAt.Equal(before.PasswordChangedAt) {
		t.Errorf("lozinka je izmenjena uprkos konfliktu: %+v", after)
	}
}

func TestUpdatePassword_UnknownUser(t *testing.T) {
	repo := newTestRepo(t)
	err := repo.UpdatePassword(context.Background(), "nema", "hash", "novi", time.Now().UTC())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, ocekivano ErrNotFound", err)
	}
}

func TestUpdatePassword_ConcurrentOnlyOneWins(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	seedUser(t, repo, "u1", 1000)

	var wg sync.WaitGroup
	var mu sync.Mutex
	wins, conflicts := 0, 0
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := repo.UpdatePassword(ctx, "u1", "hash", "novi-hash", time.Now().UTC())
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				wins++
			case errors.Is(err, ErrPasswordConflict):
				conflicts++
			}
		}()
	}
	wg.Wait()
	if wins != 1 || conflicts != 9 {
		t.Fatalf("uspesnih %d, konflikata %d; ocekivano 1 i 9", wins, conflicts)
	}
}
