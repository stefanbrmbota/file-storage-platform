package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
)

func newTestResetRepo(t *testing.T) (*MongoPasswordResetRepository, *mongo.Database) {
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

	repo, err := NewPasswordResetRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	return repo, db
}

func resetToken(userID, hash string, now time.Time, ttl time.Duration) model.PasswordResetToken {
	return model.PasswordResetToken{UserID: userID, TokenHash: hash, CreatedAt: now, ExpiresAt: now.Add(ttl)}
}

func TestResetSaveAndGet(t *testing.T) {
	repo, _ := newTestResetRepo(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	if _, err := repo.Get(ctx, "u1"); !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("pre upisa: err = %v, ocekivano ErrTokenNotFound", err)
	}
	want := resetToken("u1", "hash-1", now, 15*time.Minute)
	if err := repo.Save(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if got.TokenHash != "hash-1" || !got.CreatedAt.Equal(now) || !got.ExpiresAt.Equal(want.ExpiresAt) {
		t.Errorf("got %+v, ocekivano %+v", got, want)
	}
}

func TestResetSave_ReplacesPreviousToken(t *testing.T) {
	repo, _ := newTestResetRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_ = repo.Save(ctx, resetToken("u1", "stari", now, time.Hour))
	if err := repo.Save(ctx, resetToken("u1", "novi", now.Add(time.Second), time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Consume(ctx, "stari", now); !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("stari token mora prestati da vazi: err = %v", err)
	}
	if _, err := repo.Consume(ctx, "novi", now); err != nil {
		t.Fatalf("novi token mora da vazi: %v", err)
	}
}

func TestResetSave_OtherUsersIndependent(t *testing.T) {
	repo, _ := newTestResetRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_ = repo.Save(ctx, resetToken("u1", "h1", now, time.Hour))
	_ = repo.Save(ctx, resetToken("u2", "h2", now, time.Hour))
	if _, err := repo.Consume(ctx, "h1", now); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, "u2"); err != nil {
		t.Fatalf("token drugog korisnika nije smeo biti dirnut: %v", err)
	}
}

func TestResetConsume_OnlyOnce(t *testing.T) {
	repo, _ := newTestResetRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	_ = repo.Save(ctx, resetToken("u1", "h1", now, time.Hour))

	got, err := repo.Consume(ctx, "h1", now)
	if err != nil || got.UserID != "u1" {
		t.Fatalf("got %+v, err = %v", got, err)
	}
	if _, err := repo.Consume(ctx, "h1", now); !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("drugo koriscenje: err = %v, ocekivano ErrTokenNotFound", err)
	}
	if _, err := repo.Get(ctx, "u1"); !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("iskoriscen token ne sme ostati u bazi: err = %v", err)
	}
}

func TestResetConsume_ExpiredToken(t *testing.T) {
	repo, _ := newTestResetRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	_ = repo.Save(ctx, resetToken("u1", "h1", now.Add(-time.Hour), 30*time.Minute)) // istekao pre 30 min

	if _, err := repo.Consume(ctx, "h1", now); !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("err = %v, ocekivano ErrTokenNotFound", err)
	}
}

func TestResetConsume_UnknownToken(t *testing.T) {
	repo, _ := newTestResetRepo(t)
	if _, err := repo.Consume(context.Background(), "nepoznat", time.Now().UTC()); !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("err = %v, ocekivano ErrTokenNotFound", err)
	}
}

func TestResetConsume_ConcurrentOnlyOneWins(t *testing.T) {
	repo, _ := newTestResetRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	_ = repo.Save(ctx, resetToken("u1", "h1", now, time.Hour))

	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := repo.Consume(ctx, "h1", now); err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("uspesnih: %d, ocekivano 1", wins)
	}
}

func TestResetRestore(t *testing.T) {
	repo, _ := newTestResetRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	tok := resetToken("u1", "h1", now, time.Hour)

	_ = repo.Save(ctx, tok)
	consumed, err := repo.Consume(ctx, "h1", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Restore(ctx, consumed); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Consume(ctx, "h1", now); err != nil {
		t.Fatalf("vraceni token mora ponovo da vazi: %v", err)
	}
}

func TestResetRestore_DoesNotOverwriteNewerToken(t *testing.T) {
	repo, _ := newTestResetRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	old := resetToken("u1", "stari", now, time.Hour)
	_ = repo.Save(ctx, old)
	_, _ = repo.Consume(ctx, "stari", now)
	// U medjuvremenu korisnik trazi novi link.
	_ = repo.Save(ctx, resetToken("u1", "novi", now.Add(time.Second), time.Hour))

	if err := repo.Restore(ctx, old); err != nil {
		t.Fatalf("Restore ne sme da vrati gresku kad vec postoji noviji token: %v", err)
	}
	got, err := repo.Get(ctx, "u1")
	if err != nil || got.TokenHash != "novi" {
		t.Fatalf("noviji token mora ostati: got %+v, err = %v", got, err)
	}
	if _, err := repo.Consume(ctx, "stari", now); !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("stari token ne sme biti vracen preko novijeg: err = %v", err)
	}
}

func TestResetDelete_Idempotent(t *testing.T) {
	repo, _ := newTestResetRepo(t)
	ctx := context.Background()
	_ = repo.Save(ctx, resetToken("u1", "h1", time.Now().UTC(), time.Hour))

	if err := repo.Delete(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, "u1"); err != nil {
		t.Fatalf("ponovno brisanje: %v", err)
	}
	if _, err := repo.Get(ctx, "u1"); !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("err = %v, ocekivano ErrTokenNotFound", err)
	}
}

func TestResetTokens_Indexes(t *testing.T) {
	_, db := newTestResetRepo(t)
	ctx := context.Background()

	cur, err := db.Collection("password_reset_tokens").Indexes().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var idx []bson.M
	if err := cur.All(ctx, &idx); err != nil {
		t.Fatal(err)
	}
	var ttl, unique bool
	for _, i := range idx {
		switch i["name"] {
		case "ttl_expiresAt":
			ttl = i["expireAfterSeconds"] != nil
		case "uniq_tokenHash":
			unique = i["unique"] == true
		}
	}
	if !ttl || !unique {
		t.Fatalf("indeksi: ttl=%v unique=%v; ocekivano oba", ttl, unique)
	}
}
