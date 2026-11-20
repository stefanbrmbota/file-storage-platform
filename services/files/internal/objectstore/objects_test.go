package objectstore

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestPutGetDelete(t *testing.T) {
	ctx := context.Background()
	s, err := New(ctx, testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	content := "zdravo, ovo je sadrzaj datoteke"

	if err := s.Put(ctx, "ana/cv.txt", strings.NewReader(content), int64(len(content)), "text/plain"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	obj, err := s.Get(ctx, "ana/cv.txt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, err := io.ReadAll(obj)
	_ = obj.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Errorf("content = %q, want %q", got, content)
	}
	if obj.Size != int64(len(content)) || obj.ContentType != "text/plain" {
		t.Errorf("size = %d, type = %q", obj.Size, obj.ContentType)
	}

	if err := s.Delete(ctx, "ana/cv.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, "ana/cv.txt"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete: err = %v, want ErrNotFound", err)
	}
}

func TestPutReplacesContent(t *testing.T) {
	ctx := context.Background()
	s, err := New(ctx, testConfig(t))
	if err != nil {
		t.Fatal(err)
	}

	for _, v := range []string{"verzija 1", "verzija 2 je duza"} {
		if err := s.Put(ctx, "k", strings.NewReader(v), int64(len(v)), "text/plain"); err != nil {
			t.Fatal(err)
		}
	}
	obj, err := s.Get(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	defer obj.Close()
	got, _ := io.ReadAll(obj)
	if string(got) != "verzija 2 je duza" {
		t.Errorf("content = %q, want the second version", got)
	}
}

func TestGetMissingAndDeleteMissing(t *testing.T) {
	ctx := context.Background()
	s, err := New(ctx, testConfig(t))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.Get(ctx, "ne-postoji"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get missing: err = %v, want ErrNotFound", err)
	}
	if err := s.Delete(ctx, "ne-postoji"); err != nil {
		t.Errorf("Delete missing should not fail: %v", err)
	}
}
