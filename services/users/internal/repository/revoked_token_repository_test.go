package repository

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func newTestRevokedRepo(t *testing.T) (*MongoRevokedTokenRepository, *mongo.Database) {
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

	repo, err := NewRevokedTokenRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	return repo, db
}

func TestRevoke_StoresJTI(t *testing.T) {
	repo, _ := newTestRevokedRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if ok, err := repo.IsRevoked(ctx, "jti-1", now); err != nil || ok {
		t.Fatalf("prije opoziva: ok=%v err=%v", ok, err)
	}
	if err := repo.Revoke(ctx, "jti-1", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.IsRevoked(ctx, "jti-1", now); err != nil || !ok {
		t.Fatalf("poslije opoziva: ok=%v err=%v", ok, err)
	}
	if ok, _ := repo.IsRevoked(ctx, "drugi-jti", now); ok {
		t.Error("nepovezan jti ne smije biti opozvan")
	}
}

func TestRevoke_Idempotent(t *testing.T) {
	repo, _ := newTestRevokedRepo(t)
	ctx := context.Background()
	exp := time.Now().UTC().Add(time.Hour)

	if err := repo.Revoke(ctx, "jti-2", exp); err != nil {
		t.Fatal(err)
	}
	if err := repo.Revoke(ctx, "jti-2", exp); err != nil {
		t.Fatalf("ponovni opoziv: %v", err)
	}
}

func TestIsRevoked_IgnoresExpired(t *testing.T) {
	repo, _ := newTestRevokedRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := repo.Revoke(ctx, "jti-3", now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.IsRevoked(ctx, "jti-3", now); err != nil || ok {
		t.Fatalf("istekao zapis: ok=%v err=%v", ok, err)
	}
}

func TestRevokedTokens_HasTTLIndex(t *testing.T) {
	_, db := newTestRevokedRepo(t)
	ctx := context.Background()

	cur, err := db.Collection("revoked_tokens").Indexes().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var idx []bson.M
	if err := cur.All(ctx, &idx); err != nil {
		t.Fatal(err)
	}
	for _, i := range idx {
		if i["name"] == "ttl_expiresAt" {
			if i["expireAfterSeconds"] == nil {
				t.Fatal("indeks nema expireAfterSeconds")
			}
			return
		}
	}
	t.Fatal("TTL indeks ttl_expiresAt ne postoji")
}
