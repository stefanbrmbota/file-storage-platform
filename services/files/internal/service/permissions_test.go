package service

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

// captureLog preusmerava podrazumevani logger u bafer dok traje test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func TestUploadIntoSomeoneElsesFolderIsRefusedAndLogged(t *testing.T) {
	env := newUploadEnv(t)
	env.quota.free["marko"] = 100
	logs := captureLog(t)

	// Marko zna ID Aninog korenskog foldera i pokušava da otpremi u njega.
	in := input("a.txt", "abc")
	in.OwnerID = "marko"
	in.FolderID = env.root.ID

	_, err := env.svc.Upload(context.Background(), in)
	if !errors.Is(err, ErrFolderNotFound) {
		t.Fatalf("err = %v, want ErrFolderNotFound", err)
	}
	if len(env.objects.data) != 0 || len(env.files.items) != 0 {
		t.Errorf("ništa ne sme da se upiše, ima %d objekata i %d datoteka", len(env.objects.data), len(env.files.items))
	}
	if env.quota.free["marko"] != 100 {
		t.Errorf("kvota ne sme da se rezerviše pre provere prava, slobodno = %d", env.quota.free["marko"])
	}
	if !strings.Contains(logs.String(), "otpremanje odbijeno") || !strings.Contains(logs.String(), "userId=marko") {
		t.Errorf("odbijeno otpremanje nije zabeleženo: %s", logs.String())
	}
}

func TestNewVersionOfSomeoneElsesFileIsRefusedAndLogged(t *testing.T) {
	env := newUploadEnv(t)
	old := uploadFirst(t, env, "0123456789")
	logs := captureLog(t)

	in := version(old.ID, "tudja verzija")
	in.OwnerID = "marko"
	if _, err := env.svc.UploadNewVersion(context.Background(), in); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("err = %v, want ErrFileNotFound", err)
	}
	if !strings.Contains(logs.String(), "nova verzija odbijena") {
		t.Errorf("odbijena nova verzija nije zabeležena: %s", logs.String())
	}
}

func TestCanWrite(t *testing.T) {
	if !canWrite("ana", "ana") {
		t.Error("vlasnik mora da sme da upisuje")
	}
	if canWrite("marko", "ana") || canWrite("", "") {
		t.Error("niko osim vlasnika ne sme da upisuje")
	}
}
