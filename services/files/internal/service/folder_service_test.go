package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/repository"
)

// fakeStore keeps folders in memory and behaves like the MongoDB repository,
// including the unique (owner, parent, name) index.
type fakeStore struct {
	folders map[string]*model.Folder
}

func newFakeStore() *fakeStore {
	return &fakeStore{folders: map[string]*model.Folder{}}
}

func (s *fakeStore) Create(_ context.Context, f *model.Folder) error {
	for _, existing := range s.folders {
		if existing.OwnerID == f.OwnerID && existing.ParentID == f.ParentID && existing.Name == f.Name {
			return repository.ErrDuplicateName
		}
	}
	s.folders[f.ID] = f
	return nil
}

func (s *fakeStore) FindByID(_ context.Context, id string) (*model.Folder, error) {
	if f, ok := s.folders[id]; ok {
		return f, nil
	}
	return nil, repository.ErrNotFound
}

func (s *fakeStore) FindRoot(_ context.Context, ownerID string) (*model.Folder, error) {
	for _, f := range s.folders {
		if f.OwnerID == ownerID && f.ParentID == "" {
			return f, nil
		}
	}
	return nil, repository.ErrNotFound
}

// setup returns a service and the root folder of user "ana".
func setup(t *testing.T) (*FolderService, *model.Folder) {
	t.Helper()
	svc := NewFolderService(newFakeStore())
	root, _, err := svc.CreateRoot(context.Background(), "ana")
	if err != nil {
		t.Fatalf("CreateRoot: %v", err)
	}
	return svc, root
}

func TestCreateRootIsIdempotent(t *testing.T) {
	svc, root := setup(t)

	again, created, err := svc.CreateRoot(context.Background(), "ana")
	if err != nil {
		t.Fatalf("CreateRoot: %v", err)
	}
	if created || again.ID != root.ID {
		t.Fatalf("second CreateRoot made a new root folder")
	}
}

func TestCreateInRootWhenParentIsEmpty(t *testing.T) {
	svc, root := setup(t)

	f, err := svc.Create(context.Background(), "ana", "  Fakultet  ", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if f.ParentID != root.ID {
		t.Errorf("parent = %q, want root %q", f.ParentID, root.ID)
	}
	if f.Name != "Fakultet" {
		t.Errorf("name = %q, want trimmed name", f.Name)
	}
	if f.CreatedAt.IsZero() || f.UpdatedAt.IsZero() {
		t.Errorf("timestamps are not set")
	}
}

func TestCreateNestedFolders(t *testing.T) {
	svc, _ := setup(t)
	ctx := context.Background()

	parentID := ""
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		f, err := svc.Create(ctx, "ana", name, parentID)
		if err != nil {
			t.Fatalf("Create %q: %v", name, err)
		}
		parentID = f.ID
	}
}

func TestCreateErrors(t *testing.T) {
	svc, root := setup(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "ana", "Fakultet", root.ID); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, _, err := svc.CreateRoot(ctx, "marko"); err != nil {
		t.Fatalf("CreateRoot: %v", err)
	}
	markoRoot, _ := svc.store.FindRoot(ctx, "marko")

	tests := []struct {
		name     string
		owner    string
		folder   string
		parentID string
		want     error
	}{
		{"empty name", "ana", "   ", "", ErrInvalidName},
		{"dot name", "ana", "..", "", ErrInvalidName},
		{"slash in name", "ana", "a/b", "", ErrInvalidName},
		{"backslash in name", "ana", `a\b`, "", ErrInvalidName},
		{"control char", "ana", "a\nb", "", ErrInvalidName},
		{"name too long", "ana", strings.Repeat("x", 256), "", ErrInvalidName},
		{"duplicate in same parent", "ana", "Fakultet", root.ID, ErrNameTaken},
		{"unknown parent", "ana", "x", "does-not-exist", ErrParentNotFound},
		{"someone else's parent", "ana", "x", markoRoot.ID, ErrParentNotFound},
		{"user without root", "petar", "x", "", ErrParentNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Create(ctx, tc.owner, tc.folder, tc.parentID)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestSameNameAllowedInDifferentParents(t *testing.T) {
	svc, _ := setup(t)
	ctx := context.Background()

	a, err := svc.Create(ctx, "ana", "A", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, "ana", "Slike", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, "ana", "Slike", a.ID); err != nil {
		t.Fatalf("same name in another parent should work: %v", err)
	}
}
