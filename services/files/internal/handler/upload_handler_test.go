package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/client"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/repository"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/service"
)

type memCreator struct{ saved []*model.File }

func (m *memCreator) Create(_ context.Context, f *model.File) error {
	m.saved = append(m.saved, f)
	return nil
}

func (m *memCreator) FindByID(_ context.Context, id string) (*model.File, error) {
	for _, f := range m.saved {
		if f.ID == id {
			found := *f
			return &found, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (m *memCreator) UpdateContent(_ context.Context, id, key string, size int64, ct, hash string, at time.Time) error {
	for _, f := range m.saved {
		if f.ID == id {
			f.ObjectKey, f.Size, f.ContentType, f.Hash, f.UpdatedAt = key, size, ct, hash, at
			return nil
		}
	}
	return repository.ErrNotFound
}

type memObjects struct{ data map[string]string }

func (m *memObjects) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	b, err := io.ReadAll(r)
	m.data[key] = string(b)
	return err
}

func (m *memObjects) Delete(_ context.Context, key string) error {
	delete(m.data, key)
	return nil
}

type memQuota struct {
	free int64
	down bool
}

func (q *memQuota) Reserve(_ context.Context, _ string, n int64) error {
	if q.down {
		return client.ErrUsersUnavailable
	}
	if n > q.free {
		return client.ErrQuotaExceeded
	}
	q.free -= n
	return nil
}

func (q *memQuota) Release(_ context.Context, _ string, n int64) error {
	q.free += n
	return nil
}

type uploadSetup struct {
	h       *Handler
	saved   *memCreator
	objects *memObjects
	quota   *memQuota
}

// newUploadSetup: user "ana" has a root folder and 100 bytes of quota;
// one request may be at most 1 KB.
func newUploadSetup(t *testing.T) *uploadSetup {
	t.Helper()
	folders := service.NewFolderService(&memFolders{}, &memFiles{})
	if _, _, err := folders.CreateRoot(context.Background(), "ana"); err != nil {
		t.Fatal(err)
	}
	s := &uploadSetup{
		saved:   &memCreator{},
		objects: &memObjects{data: map[string]string{}},
		quota:   &memQuota{free: 100},
	}
	files := service.NewFileService(folders, s.saved, s.objects, s.quota)
	s.h = New(folders, files, 1024)
	return s
}

// multipartBody builds a form like a browser does for <input type="file">.
func multipartBody(t *testing.T, field, filename, content string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, mw.FormDataContentType()
}

func upload(h *Handler, folder string, body io.Reader, contentType, user string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/folders/"+folder+"/files", body)
	req.Header.Set("Content-Type", contentType)
	if user != "" {
		req.Header.Set("X-User-ID", user)
	}
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	return rec
}

func TestUploadEndpoint(t *testing.T) {
	s := newUploadSetup(t)
	body, ct := multipartBody(t, "file", "beleske.txt", "moje beleske")

	rec := upload(s.h, "root", body, ct, "ana")

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusCreated, rec.Body)
	}
	var got model.File
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "beleske.txt" || got.Size != int64(len("moje beleske")) {
		t.Errorf("response = %+v", got)
	}
	if len(s.saved.saved) != 1 || len(s.objects.data) != 1 {
		t.Errorf("want 1 metadata record and 1 object, got %d and %d", len(s.saved.saved), len(s.objects.data))
	}
	if strings.Contains(rec.Body.String(), "objectKey") {
		t.Errorf("response must not expose the internal object key")
	}
}

func TestUploadEndpointErrors(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*uploadSetup)
		body  func(t *testing.T) (io.Reader, string)
		user  string
		want  int
	}{
		{"no user", nil, func(t *testing.T) (io.Reader, string) {
			return multipartBody(t, "file", "a.txt", "x")
		}, "", http.StatusUnauthorized},
		{"not multipart", nil, func(*testing.T) (io.Reader, string) {
			return strings.NewReader(`{"name":"a"}`), "application/json"
		}, "ana", http.StatusBadRequest},
		{"missing file field", nil, func(t *testing.T) (io.Reader, string) {
			return multipartBody(t, "pogresno", "a.txt", "x")
		}, "ana", http.StatusBadRequest},
		{"request too large", nil, func(t *testing.T) (io.Reader, string) {
			return multipartBody(t, "file", "big.bin", strings.Repeat("x", 2048))
		}, "ana", http.StatusRequestEntityTooLarge},
		{"quota exceeded", nil, func(t *testing.T) (io.Reader, string) {
			return multipartBody(t, "file", "a.txt", strings.Repeat("x", 200))
		}, "ana", http.StatusInsufficientStorage},
		{"users service down", func(s *uploadSetup) { s.quota.down = true }, func(t *testing.T) (io.Reader, string) {
			return multipartBody(t, "file", "a.txt", "x")
		}, "ana", http.StatusServiceUnavailable},
		{"someone else's folder", nil, func(t *testing.T) (io.Reader, string) {
			return multipartBody(t, "file", "a.txt", "x")
		}, "marko", http.StatusNotFound},
		{"file type not allowed", nil, func(t *testing.T) (io.Reader, string) {
			return multipartBody(t, "file", "skripta.exe", "MZ")
		}, "ana", http.StatusUnsupportedMediaType},
		{"content does not match extension", nil, func(t *testing.T) (io.Reader, string) {
			return multipartBody(t, "file", "slika.png", "<html><script>alert(1)</script>")
		}, "ana", http.StatusUnsupportedMediaType},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newUploadSetup(t)
			if tc.setup != nil {
				tc.setup(s)
			}
			body, ct := tc.body(t)
			rec := upload(s.h, "root", body, ct, tc.user)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d, body %s", rec.Code, tc.want, rec.Body)
			}
			if len(s.objects.data) != 0 || len(s.saved.saved) != 0 {
				t.Errorf("nothing should be stored on error")
			}
		})
	}
}

func putVersion(h *Handler, fileID string, body io.Reader, contentType, user string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, "/files/"+fileID+"/content", body)
	req.Header.Set("Content-Type", contentType)
	if user != "" {
		req.Header.Set("X-User-ID", user)
	}
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	return rec
}

func TestNewVersionEndpoint(t *testing.T) {
	s := newUploadSetup(t)
	body, ct := multipartBody(t, "file", "cv.txt", "stari sadrzaj")
	if rec := upload(s.h, "root", body, ct, "ana"); rec.Code != http.StatusCreated {
		t.Fatalf("first upload: %d %s", rec.Code, rec.Body)
	}
	fileID := s.saved.saved[0].ID

	body, ct = multipartBody(t, "file", "bilo-koje-ime.txt", "novi, duzi sadrzaj")
	rec := putVersion(s.h, fileID, body, ct, "ana")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body)
	}
	var got model.File
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.ID != fileID || got.Name != "cv.txt" || got.Size != int64(len("novi, duzi sadrzaj")) {
		t.Errorf("same file with the old name and the new size expected, got %+v", got)
	}
	if len(s.objects.data) != 1 {
		t.Errorf("MinIO should hold only the new version, got %d objects", len(s.objects.data))
	}
}

func TestNewVersionEndpointErrors(t *testing.T) {
	s := newUploadSetup(t)
	body, ct := multipartBody(t, "file", "cv.txt", "x")
	upload(s.h, "root", body, ct, "ana")
	fileID := s.saved.saved[0].ID

	tests := []struct {
		name, fileID, user string
		want               int
	}{
		{"no user", fileID, "", http.StatusUnauthorized},
		{"unknown file", "nope", "ana", http.StatusNotFound},
		{"not the owner", fileID, "marko", http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, ct := multipartBody(t, "file", "a.txt", "nova")
			if rec := putVersion(s.h, tc.fileID, body, ct, tc.user); rec.Code != tc.want {
				t.Fatalf("status = %d, want %d, body %s", rec.Code, tc.want, rec.Body)
			}
		})
	}
}
