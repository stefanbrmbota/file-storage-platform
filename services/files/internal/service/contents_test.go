package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/model"
)

func TestContentsOfRoot(t *testing.T) {
	svc, root := setup(t)
	ctx := context.Background()
	for _, name := range []string{"Slike", "Fakultet"} {
		if _, err := svc.Create(ctx, "ana", name, ""); err != nil {
			t.Fatal(err)
		}
	}
	svc.files.(*fakeFileStore).files = []model.File{
		{ID: "11111111-1111-4111-8111-111111111111", Name: "cv.pdf", OwnerID: "ana", ParentID: root.ID},
		{ID: "22222222-2222-4222-8222-222222222222", Name: "tudji.txt", OwnerID: "marko", ParentID: root.ID},
	}

	for _, id := range []string{"", RootFolderID, root.ID} {
		c, err := svc.Contents(ctx, "ana", id)
		if err != nil {
			t.Fatalf("Contents(%q): %v", id, err)
		}
		if c.Folder.ID != root.ID {
			t.Errorf("Contents(%q) returned folder %q, want root", id, c.Folder.ID)
		}
		if len(c.Folders) != 2 || c.Folders[0].Name != "Fakultet" || c.Folders[1].Name != "Slike" {
			t.Errorf("folders = %+v, want Fakultet and Slike sorted by name", c.Folders)
		}
		if len(c.Files) != 1 || c.Files[0].Name != "cv.pdf" {
			t.Errorf("files = %+v, want only ana's cv.pdf", c.Files)
		}
		if len(c.Path) != 1 || c.Path[0].ID != root.ID {
			t.Errorf("path = %+v, want only root", c.Path)
		}
	}
}

func TestContentsPathFromRoot(t *testing.T) {
	svc, root := setup(t)
	ctx := context.Background()

	parentID := ""
	names := []string{"a", "b", "c", "d", "e"}
	for _, name := range names {
		f, err := svc.Create(ctx, "ana", name, parentID)
		if err != nil {
			t.Fatal(err)
		}
		parentID = f.ID
	}

	c, err := svc.Contents(ctx, "ana", parentID)
	if err != nil {
		t.Fatalf("Contents: %v", err)
	}
	want := append([]string{root.Name}, names...)
	if len(c.Path) != len(want) {
		t.Fatalf("path has %d steps, want %d", len(c.Path), len(want))
	}
	for i, name := range want {
		if c.Path[i].Name != name {
			t.Errorf("path[%d] = %q, want %q", i, c.Path[i].Name, name)
		}
	}
	if len(c.Folders) != 0 || len(c.Files) != 0 {
		t.Errorf("empty folder should have empty lists, got %+v", c)
	}
	if c.Folders == nil || c.Files == nil {
		t.Errorf("lists must be empty, not nil, so JSON shows [] instead of null")
	}
}

func TestContentsNotFound(t *testing.T) {
	svc, _ := setup(t)
	ctx := context.Background()
	if _, _, err := svc.CreateRoot(ctx, "marko"); err != nil {
		t.Fatal(err)
	}
	marko, err := svc.Create(ctx, "marko", "Privatno", "")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		owner    string
		folderID string
	}{
		{"unknown folder", "ana", "00000000-0000-4000-8000-000000000000"},
		{"someone else's folder", "ana", marko.ID},
		{"user without root", "petar", RootFolderID},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Contents(ctx, tc.owner, tc.folderID)
			if !errors.Is(err, ErrFolderNotFound) {
				t.Fatalf("err = %v, want ErrFolderNotFound", err)
			}
		})
	}
}
