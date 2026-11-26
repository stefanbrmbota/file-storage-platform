package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/objectstore"
)

func (o *fakeObjects) Get(_ context.Context, key string) (*objectstore.Object, error) {
	content, ok := o.data[key]
	if !ok {
		return nil, objectstore.ErrNotFound
	}
	return &objectstore.Object{ReadCloser: io.NopCloser(strings.NewReader(content)), Size: int64(len(content))}, nil
}

func TestDownloadReturnsVerifiedContent(t *testing.T) {
	env := newUploadEnv(t)
	f := uploadFirst(t, env, "sadržaj datoteke")

	d, err := env.svc.Download(context.Background(), "ana", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(d.Content)
	tmp := d.Content.Name()
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if string(got) != "sadržaj datoteke" {
		t.Errorf("sadržaj = %q", got)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf("privremena kopija nije obrisana: %v", err)
	}
}

func TestDownloadRefusesTamperedContent(t *testing.T) {
	env := newUploadEnv(t)
	f := uploadFirst(t, env, "originalni sadržaj")
	logs := captureDownloadLog(t)

	// Neko je izmenio objekat direktno u MinIO-u, mimo servisa.
	env.objects.data[f.ObjectKey] = "izmenjeni sadržaj"

	if _, err := env.svc.Download(context.Background(), "ana", f.ID); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity", err)
	}
	if !strings.Contains(logs.String(), "SHA-256") {
		t.Errorf("neuspela provera integriteta nije zabeležena: %s", logs.String())
	}
}

func TestDownloadMissingObjectIsIntegrityError(t *testing.T) {
	env := newUploadEnv(t)
	f := uploadFirst(t, env, "abc")
	delete(env.objects.data, f.ObjectKey)

	if _, err := env.svc.Download(context.Background(), "ana", f.ID); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity", err)
	}
}

func TestDownloadOnlyForOwner(t *testing.T) {
	env := newUploadEnv(t)
	f := uploadFirst(t, env, "abc")

	if _, err := env.svc.Download(context.Background(), "marko", f.ID); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("tuđa datoteka: err = %v, want ErrFileNotFound", err)
	}
	if _, err := env.svc.Download(context.Background(), "ana", "nepostojeci"); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("nepostojeća datoteka: err = %v, want ErrFileNotFound", err)
	}
}

// captureDownloadLog preusmerava podrazumevani logger u bafer dok traje test.
func captureDownloadLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}
