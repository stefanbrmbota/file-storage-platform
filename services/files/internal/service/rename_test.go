package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/model"
)

func TestRenameFolder(t *testing.T) {
	svc, _ := setup(t)
	ctx := context.Background()
	fakultet, err := svc.Create(ctx, "ana", "Fakultet", "")
	if err != nil {
		t.Fatal(err)
	}
	before := fakultet.UpdatedAt

	got, err := svc.RenameFolder(ctx, "ana", fakultet.ID, "  Faks  ")
	if err != nil {
		t.Fatalf("RenameFolder: %v", err)
	}
	if got.Name != "Faks" {
		t.Errorf("name = %q, want trimmed Faks", got.Name)
	}
	if got.UpdatedAt.Before(before) {
		t.Errorf("updatedAt did not move forward")
	}
	stored, _ := svc.store.FindByID(ctx, fakultet.ID)
	if stored.Name != "Faks" {
		t.Errorf("stored name = %q, want Faks", stored.Name)
	}
}

func TestRenameFolderErrors(t *testing.T) {
	svc, root := setup(t)
	ctx := context.Background()
	fakultet, _ := svc.Create(ctx, "ana", "Fakultet", "")
	if _, err := svc.Create(ctx, "ana", "Slike", ""); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		owner    string
		folderID string
		newName  string
		want     error
	}{
		{"invalid name", "ana", fakultet.ID, "a/b", ErrInvalidName},
		{"empty name", "ana", fakultet.ID, "  ", ErrInvalidName},
		{"name taken in same parent", "ana", fakultet.ID, "Slike", ErrNameTaken},
		{"root folder", "ana", root.ID, "Moj root", ErrCannotRenameRoot},
		{"unknown folder", "ana", "does-not-exist", "x", ErrFolderNotFound},
		{"someone else's folder", "marko", fakultet.ID, "x", ErrFolderNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.RenameFolder(ctx, tc.owner, tc.folderID, tc.newName)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestRenameFolderToSameNameIsNoop(t *testing.T) {
	svc, _ := setup(t)
	ctx := context.Background()
	fakultet, _ := svc.Create(ctx, "ana", "Fakultet", "")

	got, err := svc.RenameFolder(ctx, "ana", fakultet.ID, "Fakultet")
	if err != nil {
		t.Fatalf("renaming to the same name should not fail: %v", err)
	}
	if !got.UpdatedAt.Equal(fakultet.UpdatedAt) {
		t.Errorf("same name should not change updatedAt")
	}
}

func TestRenameFile(t *testing.T) {
	svc, root := setup(t)
	ctx := context.Background()
	files := svc.files.(*fakeFileStore)
	files.files = []model.File{
		{ID: "f1", Name: "cv.pdf", OwnerID: "ana", ParentID: root.ID},
		{ID: "f2", Name: "slika.png", OwnerID: "ana", ParentID: root.ID},
	}

	got, err := svc.RenameFile(ctx, "ana", "f1", "biografija.pdf")
	if err != nil {
		t.Fatalf("RenameFile: %v", err)
	}
	if got.Name != "biografija.pdf" || files.files[0].Name != "biografija.pdf" {
		t.Errorf("file was not renamed: %+v", files.files[0])
	}

	tests := []struct {
		name    string
		owner   string
		fileID  string
		newName string
		want    error
	}{
		{"invalid name", "ana", "f1", `a\b`, ErrInvalidName},
		{"name taken in same folder", "ana", "f1", "slika.png", ErrNameTaken},
		{"unknown file", "ana", "missing", "x", ErrFileNotFound},
		{"someone else's file", "marko", "f1", "x", ErrFileNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.RenameFile(ctx, tc.owner, tc.fileID, tc.newName)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}
