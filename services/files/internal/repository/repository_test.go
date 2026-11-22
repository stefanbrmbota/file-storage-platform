package repository

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/model"
)

// testDB connects to a real MongoDB and gives each test its own empty
// database, deleted at the end. Set FILES_TEST_MONGO_URI to run these tests:
//
//	$env:FILES_TEST_MONGO_URI="mongodb://localhost:27017"; go test ./...
//
// Without it they are skipped, so `go test ./...` works without Docker.
func testDB(t *testing.T) *mongo.Database {
	t.Helper()
	uri := os.Getenv("FILES_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("FILES_TEST_MONGO_URI not set, skipping MongoDB test")
	}
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	db := client.Database("files_test_" + uuid.NewString()[:8])
	t.Cleanup(func() {
		ctx := context.Background()
		_ = db.Drop(ctx)
		_ = client.Disconnect(ctx)
	})
	return db
}

func TestFolderRename(t *testing.T) {
	ctx := context.Background()
	repo, err := NewFolderRepository(ctx, testDB(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []*model.Folder{
		{ID: "a", Name: "Fakultet", OwnerID: "ana", ParentID: "root"},
		{ID: "b", Name: "Slike", OwnerID: "ana", ParentID: "root"},
	} {
		if err := repo.Create(ctx, f); err != nil {
			t.Fatal(err)
		}
	}
	later := time.Now().UTC().Add(time.Hour).Truncate(time.Millisecond)

	if err := repo.Rename(ctx, "a", "Faks", later); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	got, err := repo.FindByID(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Faks" || !got.UpdatedAt.Equal(later) {
		t.Errorf("after rename got name %q, updatedAt %v", got.Name, got.UpdatedAt)
	}

	if err := repo.Rename(ctx, "a", "Slike", later); !errors.Is(err, ErrDuplicateName) {
		t.Errorf("rename to existing name: err = %v, want ErrDuplicateName", err)
	}
	if err := repo.Rename(ctx, "missing", "x", later); !errors.Is(err, ErrNotFound) {
		t.Errorf("rename missing folder: err = %v, want ErrNotFound", err)
	}
}

func TestFileCreate(t *testing.T) {
	ctx := context.Background()
	repo, err := NewFileRepository(ctx, testDB(t))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	f := &model.File{
		ID: "1", Name: "cv.pdf", ContentType: "application/pdf", Size: 1234,
		OwnerID: "ana", ParentID: "root", ObjectKey: "obj-1", Hash: "abc",
		CreatedAt: now, UpdatedAt: now,
	}

	if err := repo.Create(ctx, f); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.FindByID(ctx, "1")
	if err != nil {
		t.Fatal(err)
	}
	if *got != *f {
		t.Errorf("stored file differs:\n got %+v\nwant %+v", *got, *f)
	}

	dup := *f
	dup.ID = "2"
	if err := repo.Create(ctx, &dup); !errors.Is(err, ErrDuplicateName) {
		t.Errorf("same name in same folder: err = %v, want ErrDuplicateName", err)
	}

	other := *f
	other.ID, other.ParentID = "3", "drugi-folder"
	if err := repo.Create(ctx, &other); err != nil {
		t.Errorf("same name in another folder should work: %v", err)
	}
}

func TestFileFindAndRename(t *testing.T) {
	ctx := context.Background()
	repo, err := NewFileRepository(ctx, testDB(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []model.File{
		{ID: "1", Name: "cv.pdf", OwnerID: "ana", ParentID: "root"},
		{ID: "2", Name: "slika.png", OwnerID: "ana", ParentID: "root"},
	} {
		if _, err := repo.coll.InsertOne(ctx, f); err != nil {
			t.Fatal(err)
		}
	}

	if err := repo.Rename(ctx, "1", "biografija.pdf", time.Now()); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	got, err := repo.FindByID(ctx, "1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "biografija.pdf" {
		t.Errorf("name = %q, want biografija.pdf", got.Name)
	}

	if err := repo.Rename(ctx, "1", "slika.png", time.Now()); !errors.Is(err, ErrDuplicateName) {
		t.Errorf("rename to existing name: err = %v, want ErrDuplicateName", err)
	}
	if _, err := repo.FindByID(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindByID missing: err = %v, want ErrNotFound", err)
	}
}
