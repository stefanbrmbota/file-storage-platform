package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

var pngHeader = "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"

func TestFileTypePolicyCheck(t *testing.T) {
	p := NewFileTypePolicy(nil)
	tests := []struct {
		name, file, content string
		wantType            string
		wantErr             bool
	}{
		{"txt", "beleske.txt", "zdravo", "text/plain; charset=utf-8", false},
		{"png", "slika.PNG", pngHeader, "image/png", false},
		{"pdf", "cv.pdf", "%PDF-1.7\n...", "application/pdf", false},
		{"exe preimenovan u pdf", "cv.pdf", "MZ\x90\x00\x03\x00\x00\x00", "", true},
		{"html preimenovan u txt", "a.txt", "<html><script>alert(1)</script>", "", true},
		{"png preimenovan u jpg", "a.jpg", pngHeader, "", true},
		{"ekstenzija nije na listi", "skripta.exe", "MZ", "", true},
		{"html nije dozvoljen", "a.html", "<html></html>", "", true},
		{"svg nije dozvoljen", "a.svg", "<svg></svg>", "", true},
		{"bez ekstenzije", "README", "tekst", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ct, r, err := p.Check(tc.file, strings.NewReader(tc.content))
			if tc.wantErr {
				if !errors.Is(err, ErrFileTypeNotAllowed) {
					t.Fatalf("err = %v, want ErrFileTypeNotAllowed", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if ct != tc.wantType {
				t.Errorf("content type = %q, want %q", ct, tc.wantType)
			}
			// Reader mora da vrati ceo sadržaj, i deo pročitan za proveru.
			got, _ := io.ReadAll(r)
			if string(got) != tc.content {
				t.Errorf("sadržaj je izmenjen: %q", got)
			}
		})
	}
}

func TestFileTypePolicyFromConfig(t *testing.T) {
	p := NewFileTypePolicy([]string{" PDF", ".txt", "exe", ""})
	if _, _, err := p.Check("a.pdf", strings.NewReader("%PDF-1.4")); err != nil {
		t.Errorf("pdf je na listi: %v", err)
	}
	if _, _, err := p.Check("a.png", strings.NewReader(pngHeader)); !errors.Is(err, ErrFileTypeNotAllowed) {
		t.Errorf("png nije na listi, err = %v", err)
	}
	if _, _, err := p.Check("a.exe", strings.NewReader("MZ")); !errors.Is(err, ErrFileTypeNotAllowed) {
		t.Errorf("exe se ne sme dozvoliti ni preko konfiguracije, err = %v", err)
	}
}

func TestUploadRejectsDisallowedTypeBeforeAnythingIsStored(t *testing.T) {
	env := newUploadEnv(t)
	in := input("virus.pdf", "MZ\x90\x00")
	in.ContentType = "application/pdf" // klijent tvrdi da je PDF

	if _, err := env.svc.Upload(context.Background(), in); !errors.Is(err, ErrFileTypeNotAllowed) {
		t.Fatalf("err = %v, want ErrFileTypeNotAllowed", err)
	}
	if len(env.objects.data) != 0 || env.quota.free["ana"] != 100 {
		t.Errorf("ništa ne sme da se upiše ni rezerviše")
	}
}

func TestUploadStoresDetectedContentType(t *testing.T) {
	env := newUploadEnv(t)
	in := input("a.txt", "abc")
	in.ContentType = "text/html" // zaglavlje od klijenta se ignoriše

	f, err := env.svc.Upload(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if f.ContentType != "text/plain; charset=utf-8" {
		t.Errorf("content type = %q, sačuvan je tip od klijenta", f.ContentType)
	}
}

func TestNewVersionMustMatchFileType(t *testing.T) {
	env := newUploadEnv(t)
	old := uploadFirst(t, env, "0123456789") // cv.txt
	in := version(old.ID, "<html><script>x</script>")

	if _, err := env.svc.UploadNewVersion(context.Background(), in); !errors.Is(err, ErrFileTypeNotAllowed) {
		t.Fatalf("err = %v, want ErrFileTypeNotAllowed", err)
	}
}
