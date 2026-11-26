package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/model"
)

// uploadFirst uploads "cv.txt" with the given content for user "ana".
func uploadFirst(t *testing.T, env *uploadEnv, content string) *model.File {
	t.Helper()
	f, err := env.svc.Upload(context.Background(), input("cv.txt", content))
	if err != nil {
		t.Fatal(err)
	}
	// Return a copy: the fake store keeps the same pointer and later updates
	// would otherwise change our "old version" too.
	old := *f
	return &old
}

func version(fileID, content string) NewVersionInput {
	return NewVersionInput{
		OwnerID: "ana", FileID: fileID, ContentType: "text/plain",
		Size: int64(len(content)), Content: strings.NewReader(content),
	}
}

func TestNewVersionBiggerReservesOnlyDifference(t *testing.T) {
	env := newUploadEnv(t)
	old := uploadFirst(t, env, "0123456789") // 10 bytes, 90 left

	got, err := env.svc.UploadNewVersion(context.Background(), version(old.ID, "0123456789ABCDE")) // 15 bytes
	if err != nil {
		t.Fatalf("UploadNewVersion: %v", err)
	}

	if got.ID != old.ID || got.Name != "cv.txt" || got.Size != 15 {
		t.Errorf("same file with new size expected, got %+v", got)
	}
	if env.quota.free["ana"] != 85 {
		t.Errorf("free quota = %d, want 85 (only +5 reserved)", env.quota.free["ana"])
	}
	if _, oldExists := env.objects.data[old.ObjectKey]; oldExists {
		t.Errorf("old object must be deleted")
	}
	if env.objects.data[got.ObjectKey] != "0123456789ABCDE" || len(env.objects.data) != 1 {
		t.Errorf("MinIO should hold only the new content, got %v", env.objects.data)
	}
	if got.Hash == old.Hash {
		t.Errorf("hash must change with the content")
	}
}

func TestNewVersionSmallerReleasesDifference(t *testing.T) {
	env := newUploadEnv(t)
	old := uploadFirst(t, env, "0123456789") // 90 left

	if _, err := env.svc.UploadNewVersion(context.Background(), version(old.ID, "abc")); err != nil {
		t.Fatal(err)
	}
	if env.quota.free["ana"] != 97 {
		t.Errorf("free quota = %d, want 97 (7 bytes given back)", env.quota.free["ana"])
	}
}

func TestNewVersionRejected(t *testing.T) {
	tests := []struct {
		name   string
		change func(*uploadEnv, *NewVersionInput)
		want   error
	}{
		{"not the owner", func(_ *uploadEnv, in *NewVersionInput) { in.OwnerID = "marko" }, ErrFileNotFound},
		{"unknown file", func(_ *uploadEnv, in *NewVersionInput) { in.FileID = "00000000-0000-4000-8000-000000000000" }, ErrFileNotFound},
		{"invalid file id", func(_ *uploadEnv, in *NewVersionInput) { in.FileID = "nope" }, ErrInvalidID},
		{"negative size", func(_ *uploadEnv, in *NewVersionInput) { in.Size = -1 }, ErrInvalidSize},
		{"quota exceeded", func(_ *uploadEnv, in *NewVersionInput) {
			big := strings.Repeat("x", 200)
			in.Size, in.Content = int64(len(big)), strings.NewReader(big)
		}, ErrQuotaExceeded},
		{"users service down", func(e *uploadEnv, in *NewVersionInput) {
			e.quota.down = true
			in.Size, in.Content = 20, strings.NewReader(strings.Repeat("y", 20))
		}, ErrQuotaUnavailable},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newUploadEnv(t)
			old := uploadFirst(t, env, "0123456789")
			in := version(old.ID, "nova verzija")
			tc.change(env, &in)

			_, err := env.svc.UploadNewVersion(context.Background(), in)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			assertOldVersionIntact(t, env, old)
		})
	}
}

func TestNewVersionMinioFailsKeepsOldVersion(t *testing.T) {
	env := newUploadEnv(t)
	old := uploadFirst(t, env, "0123456789")
	env.objects.failPut = true

	if _, err := env.svc.UploadNewVersion(context.Background(), version(old.ID, "0123456789ABCDE")); err == nil {
		t.Fatal("should fail when MinIO fails")
	}
	assertOldVersionIntact(t, env, old)
}

func TestNewVersionDatabaseFailsUndoesEverything(t *testing.T) {
	env := newUploadEnv(t)
	old := uploadFirst(t, env, "0123456789")
	env.files.failUpdate = true

	if _, err := env.svc.UploadNewVersion(context.Background(), version(old.ID, "0123456789ABCDE")); err == nil {
		t.Fatal("should fail when the database fails")
	}
	assertOldVersionIntact(t, env, old)
}

// assertOldVersionIntact checks that a failed new version left no trace:
// same metadata, only the old object in MinIO, quota as after the first upload.
func assertOldVersionIntact(t *testing.T, env *uploadEnv, old *model.File) {
	t.Helper()
	stored, _ := env.files.FindByID(context.Background(), old.ID)
	if stored.ObjectKey != old.ObjectKey || stored.Size != old.Size {
		t.Errorf("metadata changed: %+v", stored)
	}
	if len(env.objects.data) != 1 || env.objects.data[old.ObjectKey] != "0123456789" {
		t.Errorf("MinIO should hold only the old content, got %v", env.objects.data)
	}
	if env.quota.free["ana"] != 90 {
		t.Errorf("free quota = %d, want 90", env.quota.free["ana"])
	}
}
