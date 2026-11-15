package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/repository"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/service"
)

// memFolders is a tiny in-memory folder store for handler tests.
type memFolders struct{ items []*model.Folder }

func (m *memFolders) Create(_ context.Context, f *model.Folder) error {
	m.items = append(m.items, f)
	return nil
}

func (m *memFolders) FindByID(_ context.Context, id string) (*model.Folder, error) {
	for _, f := range m.items {
		if f.ID == id {
			return f, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (m *memFolders) FindRoot(_ context.Context, ownerID string) (*model.Folder, error) {
	for _, f := range m.items {
		if f.OwnerID == ownerID && f.ParentID == "" {
			return f, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (m *memFolders) ListByParent(_ context.Context, ownerID, parentID string) ([]model.Folder, error) {
	out := []model.Folder{}
	for _, f := range m.items {
		if f.OwnerID == ownerID && f.ParentID == parentID {
			out = append(out, *f)
		}
	}
	return out, nil
}

type memFiles struct{}

func (memFiles) ListByParent(context.Context, string, string) ([]model.File, error) {
	return []model.File{}, nil
}

// newTestHandler returns a handler where user "ana" has root/Fakultet.
func newTestHandler(t *testing.T) (*Handler, *model.Folder) {
	t.Helper()
	svc := service.NewFolderService(&memFolders{}, memFiles{})
	ctx := context.Background()
	if _, _, err := svc.CreateRoot(ctx, "ana"); err != nil {
		t.Fatal(err)
	}
	fakultet, err := svc.Create(ctx, "ana", "Fakultet", "")
	if err != nil {
		t.Fatal(err)
	}
	return New(svc), fakultet
}

func getContents(h *Handler, id, user string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/folders/"+id+"/contents", nil)
	if user != "" {
		req.Header.Set("X-User-ID", user)
	}
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	return rec
}

func TestFolderContentsOfRoot(t *testing.T) {
	h, _ := newTestHandler(t)

	rec := getContents(h, "root", "ana")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body)
	}
	var body service.FolderContents
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Folders) != 1 || body.Folders[0].Name != "Fakultet" {
		t.Errorf("folders = %+v, want [Fakultet]", body.Folders)
	}
	if len(body.Path) != 1 || body.Path[0].Name != "root" {
		t.Errorf("path = %+v, want [root]", body.Path)
	}
}

func TestFolderContentsOfSubfolder(t *testing.T) {
	h, fakultet := newTestHandler(t)

	rec := getContents(h, fakultet.ID, "ana")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body service.FolderContents
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Path) != 2 || body.Path[1].Name != "Fakultet" {
		t.Errorf("path = %+v, want [root Fakultet]", body.Path)
	}
}

func TestFolderContentsErrors(t *testing.T) {
	h, fakultet := newTestHandler(t)

	tests := []struct {
		name string
		id   string
		user string
		want int
	}{
		{"no user", "root", "", http.StatusUnauthorized},
		{"unknown folder", "does-not-exist", "ana", http.StatusNotFound},
		{"someone else's folder", fakultet.ID, "marko", http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := getContents(h, tc.id, tc.user)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
