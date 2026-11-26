package handler

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/objectstore"
)

func (m *memObjects) Get(_ context.Context, key string) (*objectstore.Object, error) {
	content, ok := m.data[key]
	if !ok {
		return nil, objectstore.ErrNotFound
	}
	return &objectstore.Object{ReadCloser: io.NopCloser(strings.NewReader(content)), Size: int64(len(content))}, nil
}

// uploadOne otprema datoteku kao "ana" i vraća njen ID.
func uploadOne(t *testing.T, s *uploadSetup, name, content string) string {
	t.Helper()
	body, ct := multipartBody(t, "file", name, content)
	rec := upload(s.h, "root", body, ct, "ana")
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body)
	}
	var f struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&f); err != nil {
		t.Fatal(err)
	}
	return f.ID
}

func download(h *Handler, id, user string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/files/"+id+"/content", nil)
	if user != "" {
		req.Header.Set("X-User-ID", user)
	}
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	return rec
}

func TestDownloadEndpoint(t *testing.T) {
	s := newUploadSetup(t)
	name := "izveštaj; finalni (1).txt"
	id := uploadOne(t, s, name, "abc")

	rec := download(s.h, id, "ana")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if rec.Body.String() != "abc" {
		t.Errorf("body = %q", rec.Body.String())
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("nedostaje X-Content-Type-Options: nosniff")
	}
	// Naziv sa ; , razmacima i dijakriticima mora ostati jedan, ispravan parametar.
	_, params, err := mime.ParseMediaType(rec.Header().Get("Content-Disposition"))
	if err != nil || params["filename"] != name {
		t.Errorf("Content-Disposition = %q (err %v), filename = %q", rec.Header().Get("Content-Disposition"), err, params["filename"])
	}
}

func TestDownloadTamperedFileIsRefused(t *testing.T) {
	s := newUploadSetup(t)
	id := uploadOne(t, s, "a.txt", "originalno")
	for key := range s.objects.data {
		s.objects.data[key] = "izmenjeno"
	}

	rec := download(s.h, id, "ana")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "izmenjeno") {
		t.Error("izmenjen sadržaj ne sme da stigne do klijenta")
	}
}

func TestDownloadErrors(t *testing.T) {
	s := newUploadSetup(t)
	id := uploadOne(t, s, "a.txt", "abc")

	if rec := download(s.h, id, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("bez korisnika: status = %d", rec.Code)
	}
	if rec := download(s.h, id, "marko"); rec.Code != http.StatusNotFound {
		t.Errorf("tuđa datoteka: status = %d", rec.Code)
	}
	if rec := download(s.h, "nepostojeci", "ana"); rec.Code != http.StatusNotFound {
		t.Errorf("nepostojeća datoteka: status = %d", rec.Code)
	}
}
