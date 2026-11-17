package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
)

func newTestTokenRepo(t *testing.T) *MongoTokenRepository {
	t.Helper()
	uri := os.Getenv("USERS_MONGO_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri).SetServerSelectionTimeout(2 * time.Second))
	if err != nil {
		t.Skipf("MongoDB nije dostupan: %v", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		t.Skipf("MongoDB nije dostupan: %v", err)
	}
	db := client.Database(fmt.Sprintf("users_test_%d", time.Now().UnixNano()))
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	repo, err := NewTokenRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestConsume_OnlyOnce(t *testing.T) {
	repo := newTestTokenRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	tok := model.ConfirmationToken{ID: "hash-1", UserID: "u1", ExpiresAt: now.Add(time.Hour)}
	if err := repo.Create(ctx, tok); err != nil {
		t.Fatal(err)
	}

	got, err := repo.Consume(ctx, "hash-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if got.UserID != "u1" {
		t.Errorf("userId = %q", got.UserID)
	}
	if _, err := repo.Consume(ctx, "hash-1", now); !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("drugo korišćenje: err = %v, očekivano ErrTokenNotFound", err)
	}
}

func TestConsume_ExpiredToken(t *testing.T) {
	repo := newTestTokenRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := repo.Create(ctx, model.ConfirmationToken{ID: "hash-2", UserID: "u1", ExpiresAt: now.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Consume(ctx, "hash-2", now); !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("err = %v, očekivano ErrTokenNotFound", err)
	}
}

func TestConsume_ConcurrentOnlyOneWins(t *testing.T) {
	repo := newTestTokenRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if err := repo.Create(ctx, model.ConfirmationToken{ID: "hash-3", UserID: "u1", ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := repo.Consume(ctx, "hash-3", now); err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("uspešnih: %d, očekivano 1", wins)
	}
}
