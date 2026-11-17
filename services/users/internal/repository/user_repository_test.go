package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
)

// newTestRepo se povezuje na MongoDB (USERS_MONGO_URI, podrazumevano lokalni)
// i koristi privremenu bazu koja se brise na kraju. Bez baze test se preskace.
func newTestRepo(t *testing.T) *MongoUserRepository {
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

	repo, err := NewUserRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestCreateAndFind(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	want := model.NewUser("id-1", "Pera", "Peric", "pera@example.com", "hash", 1<<30, time.Now().UTC().Truncate(time.Millisecond))

	if err := repo.Create(ctx, want); err != nil {
		t.Fatal(err)
	}

	byEmail, err := repo.FindByEmail(ctx, "pera@example.com")
	if err != nil {
		t.Fatal(err)
	}
	byID, err := repo.FindByID(ctx, "id-1")
	if err != nil {
		t.Fatal(err)
	}
	for name, got := range map[string]model.User{"email": byEmail, "id": byID} {
		if got.ID != want.ID || got.FirstName != "Pera" || got.LastName != "Peric" ||
			got.PasswordHash != "hash" || got.Role != model.RoleUser || got.Active ||
			got.QuotaBytes != 1<<30 || got.UsedBytes != 0 ||
			!got.PasswordChangedAt.Equal(want.PasswordChangedAt) {
			t.Errorf("%s: dobijeno %+v, očekivano %+v", name, got, want)
		}
	}
}

func TestCreateDuplicateEmail(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := repo.Create(ctx, model.NewUser("id-1", "Pera", "Peric", "pera@example.com", "h", 1<<30, now)); err != nil {
		t.Fatal(err)
	}
	err := repo.Create(ctx, model.NewUser("id-2", "Mika", "Mikic", "pera@example.com", "h", 1<<30, now))
	if !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("err = %v, očekivano ErrEmailTaken", err)
	}
}

func TestFindNotFound(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	if _, err := repo.FindByEmail(ctx, "nema@example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindByEmail err = %v, očekivano ErrNotFound", err)
	}
	if _, err := repo.FindByID(ctx, "nema"); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindByID err = %v, očekivano ErrNotFound", err)
	}
}
