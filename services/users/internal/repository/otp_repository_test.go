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

func newTestOTPRepo(t *testing.T) (*MongoOTPRepository, *mongo.Database) {
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

	repo, err := NewOTPRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	return repo, db
}

func TestOTPRepo_SaveAndGet(t *testing.T) {
	repo, _ := newTestOTPRepo(t)
	ctx := context.Background()
	exp := time.Now().UTC().Add(5 * time.Minute).Truncate(time.Millisecond)

	if err := repo.Save(ctx, model.OTP{UserID: "u1", CodeHash: "h1", ExpiresAt: exp}); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if got.CodeHash != "h1" || got.Attempts != 0 || !got.ExpiresAt.Equal(exp) {
		t.Errorf("neocekivan zapis: %+v", got)
	}
}

func TestOTPRepo_GetMissing(t *testing.T) {
	repo, _ := newTestOTPRepo(t)
	if _, err := repo.Get(context.Background(), "nema"); !errors.Is(err, ErrOTPNotFound) {
		t.Errorf("err = %v, ocekivano ErrOTPNotFound", err)
	}
}

func TestOTPRepo_SaveReplaces(t *testing.T) {
	repo, db := newTestOTPRepo(t)
	ctx := context.Background()
	exp := time.Now().UTC().Add(time.Minute)

	_ = repo.Save(ctx, model.OTP{UserID: "u1", CodeHash: "stari", ExpiresAt: exp, Attempts: 3})
	_ = repo.Save(ctx, model.OTP{UserID: "u1", CodeHash: "novi", ExpiresAt: exp})

	got, err := repo.Get(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if got.CodeHash != "novi" || got.Attempts != 0 {
		t.Errorf("OTP nije zamenjen: %+v", got)
	}
	n, _ := db.Collection("otps").CountDocuments(ctx, bson.D{})
	if n != 1 {
		t.Errorf("ocekivan 1 dokument, ima %d", n)
	}
}

func TestOTPRepo_HasTTLIndex(t *testing.T) {
	_, db := newTestOTPRepo(t)
	ctx := context.Background()

	cur, err := db.Collection("otps").Indexes().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var idx []bson.M
	if err := cur.All(ctx, &idx); err != nil {
		t.Fatal(err)
	}
	for _, i := range idx {
		if i["name"] == "ttl_expiresAt" {
			if v, ok := i["expireAfterSeconds"]; !ok || fmt.Sprint(v) != "0" {
				t.Errorf("expireAfterSeconds = %v, ocekivano 0", v)
			}
			return
		}
	}
	t.Error("TTL indeks ttl_expiresAt ne postoji")
}

func TestOTPRepo_IncrementAttempts(t *testing.T) {
	repo, _ := newTestOTPRepo(t)
	ctx := context.Background()
	_ = repo.Save(ctx, model.OTP{UserID: "u1", CodeHash: "h", ExpiresAt: time.Now().UTC().Add(time.Minute)})

	for want := 1; want <= 3; want++ {
		o, err := repo.IncrementAttempts(ctx, "u1")
		if err != nil {
			t.Fatal(err)
		}
		if o.Attempts != want {
			t.Errorf("Attempts = %d, ocekivano %d", o.Attempts, want)
		}
	}
	if _, err := repo.IncrementAttempts(ctx, "nema"); !errors.Is(err, ErrOTPNotFound) {
		t.Errorf("err = %v, ocekivano ErrOTPNotFound", err)
	}
}

func TestOTPRepo_IncrementAttempts_Concurrent(t *testing.T) {
	repo, _ := newTestOTPRepo(t)
	ctx := context.Background()
	_ = repo.Save(ctx, model.OTP{UserID: "u1", CodeHash: "h", ExpiresAt: time.Now().UTC().Add(time.Minute)})

	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = repo.IncrementAttempts(ctx, "u1")
		}()
	}
	wg.Wait()

	o, _ := repo.Get(ctx, "u1")
	if o.Attempts != n {
		t.Errorf("Attempts = %d, ocekivano %d", o.Attempts, n)
	}
}

func TestOTPRepo_Delete(t *testing.T) {
	repo, _ := newTestOTPRepo(t)
	ctx := context.Background()
	_ = repo.Save(ctx, model.OTP{UserID: "u1", CodeHash: "h", ExpiresAt: time.Now().UTC().Add(time.Minute)})

	if err := repo.Delete(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, "u1"); !errors.Is(err, ErrOTPNotFound) {
		t.Errorf("drugo brisanje: err = %v, ocekivano ErrOTPNotFound", err)
	}
	if _, err := repo.Get(ctx, "u1"); !errors.Is(err, ErrOTPNotFound) {
		t.Errorf("Get posle brisanja: err = %v", err)
	}
}
