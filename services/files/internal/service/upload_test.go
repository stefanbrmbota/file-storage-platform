package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/client"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/repository"
)

// fakeFiles stores file metadata in memory, with the unique-name rule.
type fakeFiles struct{ items []*model.File }

func (f *fakeFiles) Create(_ context.Context, file *model.File) error {
	for _, other := range f.items {
		if other.OwnerID == file.OwnerID && other.ParentID == file.ParentID && other.Name == file.Name {
			return repository.ErrDuplicateName
		}
	}
	f.items = append(f.items, file)
	return nil
}

// fakeObjects stores contents in memory and can be told to fail.
type fakeObjects struct {
	data    map[string]string
	failPut bool
}

func (o *fakeObjects) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	if o.failPut {
		return errors.New("minio is down")
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	o.data[key] = string(b)
	return nil
}

func (o *fakeObjects) Delete(_ context.Context, key string) error {
	delete(o.data, key)
	return nil
}

// fakeQuota keeps one number per user: the free space.
type fakeQuota struct {
	free map[string]int64
	down bool
}

func (q *fakeQuota) Reserve(_ context.Context, user string, n int64) error {
	if q.down {
		return client.ErrUsersUnavailable
	}
	if n > q.free[user] {
		return client.ErrQuotaExceeded
	}
	q.free[user] -= n
	return nil
}

func (q *fakeQuota) Release(_ context.Context, user string, n int64) error {
	q.free[user] += n
	return nil
}

type uploadEnv struct {
	svc     *FileService
	root    *model.Folder
	files   *fakeFiles
	objects *fakeObjects
	quota   *fakeQuota
}

// newUploadEnv: user "ana" has a root folder and 100 bytes of free space.
func newUploadEnv(t *testing.T) *uploadEnv {
	t.Helper()
	folders, root := setup(t)
	env := &uploadEnv{
		root:    root,
		files:   &fakeFiles{},
		objects: &fakeObjects{data: map[string]string{}},
		quota:   &fakeQuota{free: map[string]int64{"ana": 100}},
	}
	env.svc = NewFileService(folders, env.files, env.objects, env.quota)
	return env
}

func input(name, content string) UploadInput {
	return UploadInput{
		OwnerID: "ana", FolderID: "root", Name: name, ContentType: "text/plain",
		Size: int64(len(content)), Content: strings.NewReader(content),
	}
}

func TestUploadSuccess(t *testing.T) {
	env := newUploadEnv(t)
	content := "zdravo svete"

	f, err := env.svc.Upload(context.Background(), input("  pozdrav.txt ", content))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	if f.Name != "pozdrav.txt" || f.ParentID != env.root.ID || f.OwnerID != "ana" || f.Size != 12 {
		t.Errorf("unexpected metadata: %+v", f)
	}
	if env.objects.data[f.ObjectKey] != content {
		t.Errorf("content in MinIO = %q, want %q", env.objects.data[f.ObjectKey], content)
	}
	want := sha256.Sum256([]byte(content))
	if f.Hash != hex.EncodeToString(want[:]) {
		t.Errorf("hash = %s, want SHA-256 of the content", f.Hash)
	}
	if env.quota.free["ana"] != 100-12 {
		t.Errorf("free quota = %d, want 88", env.quota.free["ana"])
	}
	if len(env.files.items) != 1 {
		t.Errorf("metadata saved %d times, want 1", len(env.files.items))
	}
}

func TestUploadRejectedBeforeAnythingIsStored(t *testing.T) {
	tests := []struct {
		name   string
		change func(*uploadEnv, *UploadInput)
		want   error
	}{
		{"quota exceeded", func(_ *uploadEnv, in *UploadInput) {
			in.Size, in.Content = 500, strings.NewReader(strings.Repeat("x", 500))
		}, ErrQuotaExceeded},
		{"users service down", func(e *uploadEnv, _ *UploadInput) { e.quota.down = true }, ErrQuotaUnavailable},
		{"invalid name", func(_ *uploadEnv, in *UploadInput) { in.Name = "a/b.txt" }, ErrInvalidName},
		{"negative size", func(_ *uploadEnv, in *UploadInput) { in.Size = -1 }, ErrInvalidSize},
		{"unknown folder", func(_ *uploadEnv, in *UploadInput) { in.FolderID = "nope" }, ErrFolderNotFound},
		{"someone else's folder", func(_ *uploadEnv, in *UploadInput) { in.OwnerID = "marko" }, ErrFolderNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newUploadEnv(t)
			in := input("a.txt", "abc")
			tc.change(env, &in)

			_, err := env.svc.Upload(context.Background(), in)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if len(env.objects.data) != 0 || len(env.files.items) != 0 {
				t.Errorf("nothing should be stored, got %d objects, %d files", len(env.objects.data), len(env.files.items))
			}
			if env.quota.free["ana"] != 100 {
				t.Errorf("free quota = %d, want 100 (nothing reserved)", env.quota.free["ana"])
			}
		})
	}
}

func TestUploadMinioFailsReleasesQuota(t *testing.T) {
	env := newUploadEnv(t)
	env.objects.failPut = true

	if _, err := env.svc.Upload(context.Background(), input("a.txt", "abc")); err == nil {
		t.Fatal("upload should fail when MinIO fails")
	}
	if env.quota.free["ana"] != 100 {
		t.Errorf("free quota = %d, want 100: reserved space must be given back", env.quota.free["ana"])
	}
	if len(env.files.items) != 0 {
		t.Errorf("no metadata should be saved")
	}
}

func TestUploadDuplicateNameUndoesEverything(t *testing.T) {
	env := newUploadEnv(t)
	ctx := context.Background()
	if _, err := env.svc.Upload(ctx, input("a.txt", "prvi")); err != nil {
		t.Fatal(err)
	}

	_, err := env.svc.Upload(ctx, input("a.txt", "drugi"))
	if !errors.Is(err, ErrNameTaken) {
		t.Fatalf("err = %v, want ErrNameTaken", err)
	}
	if len(env.objects.data) != 1 {
		t.Errorf("MinIO has %d objects, want 1: the second object must be deleted", len(env.objects.data))
	}
	if env.quota.free["ana"] != 100-4 {
		t.Errorf("free quota = %d, want 96: only the first file uses space", env.quota.free["ana"])
	}
}
