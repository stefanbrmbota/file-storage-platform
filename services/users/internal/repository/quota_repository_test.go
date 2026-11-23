package repository

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
)

func seedUser(t *testing.T, repo *MongoUserRepository, id string, quota int64) {
	t.Helper()
	u := model.NewUser(id, "Pera", "Peric", id+"@example.com", "hash", quota, time.Now().UTC())
	if err := repo.Create(context.Background(), u); err != nil {
		t.Fatal(err)
	}
}

func TestReserveQuota_Success(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	seedUser(t, repo, "u1", 1000)

	remaining, err := repo.ReserveQuota(ctx, "u1", 300)
	if err != nil || remaining != 700 {
		t.Fatalf("remaining = %d, err = %v", remaining, err)
	}
	u, _ := repo.FindByID(ctx, "u1")
	if u.UsedBytes != 300 {
		t.Errorf("usedBytes = %d, ocekivano 300", u.UsedBytes)
	}
}

func TestReserveQuota_ExactlyToLimit(t *testing.T) {
	repo := newTestRepo(t)
	seedUser(t, repo, "u1", 1000)

	remaining, err := repo.ReserveQuota(context.Background(), "u1", 1000)
	if err != nil || remaining != 0 {
		t.Fatalf("remaining = %d, err = %v", remaining, err)
	}
}

func TestReserveQuota_Exceeded_NoChange(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	seedUser(t, repo, "u1", 1000)
	if _, err := repo.ReserveQuota(ctx, "u1", 900); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.ReserveQuota(ctx, "u1", 101); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("err = %v, ocekivano ErrQuotaExceeded", err)
	}
	u, _ := repo.FindByID(ctx, "u1")
	if u.UsedBytes != 900 {
		t.Errorf("usedBytes = %d, ocekivano 900 (bez promene)", u.UsedBytes)
	}
}

func TestReserveQuota_UnknownUser(t *testing.T) {
	repo := newTestRepo(t)
	if _, err := repo.ReserveQuota(context.Background(), "nema", 10); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, ocekivano ErrNotFound", err)
	}
}

func TestReserveQuota_ConcurrentNeverExceeds(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	seedUser(t, repo, "u1", 1000)

	const workers, each = 40, 100 // zajedno 4000 B za kvotu od 1000 B
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := repo.ReserveQuota(ctx, "u1", each); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	u, _ := repo.FindByID(ctx, "u1")
	if ok != 10 || u.UsedBytes != 1000 {
		t.Errorf("uspesnih = %d, usedBytes = %d; ocekivano 10 i 1000", ok, u.UsedBytes)
	}
}
