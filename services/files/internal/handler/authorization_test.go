package handler

// Autorizacija svakog zahteva u Files servisu (US-62, T-62.2).

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/client"
)

const testInternalToken = "test-internal-token-at-least-32-chars!"

func postRoot(h *Handler, token string, set bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/internal/folders/root", strings.NewReader(`{"userId":"marko"}`))
	if set {
		req.Header.Set(client.InternalTokenHeader, token)
	}
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	return rec
}

func TestInternalRouteNeedsToken(t *testing.T) {
	h, _ := newTestHandler(t)
	h.WithInternalToken(testInternalToken)

	cases := []struct {
		name  string
		token string
		set   bool
	}{
		{"bez zaglavlja", "", false},
		{"prazan token", "", true},
		{"pogresan token", "pogresan-token-koji-je-dovoljno-dugacak-1", true},
		{"prefiks tokena", testInternalToken[:len(testInternalToken)-1], true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := postRoot(h, c.token, c.set)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, ocekivano 401", rec.Code)
			}
			if strings.Contains(rec.Body.String(), testInternalToken) {
				t.Error("odgovor otkriva tajnu")
			}
		})
	}

	if rec := postRoot(h, testInternalToken, true); rec.Code != http.StatusCreated {
		t.Errorf("ispravan token: status = %d, ocekivano 201", rec.Code)
	}
}

// Bez podesenog INTERNAL_TOKEN interna ruta je zatvorena za sve (fail closed).
func TestInternalRouteClosedWithoutConfiguredToken(t *testing.T) {
	for _, configured := range []string{"", "kratko"} {
		h, _ := newTestHandler(t)
		h.WithInternalToken(configured)
		if rec := postRoot(h, configured, true); rec.Code != http.StatusUnauthorized {
			t.Errorf("tajna %q: status = %d, ocekivano 401", configured, rec.Code)
		}
	}
}

// Svaka korisnicka ruta odbija zahtev bez ispravnog X-User-ID.
func TestEveryUserRouteNeedsUserID(t *testing.T) {
	routes := []struct{ method, path string }{
		{http.MethodPost, "/folders"},
		{http.MethodGet, "/folders/root/contents"},
		{http.MethodPatch, "/folders/x"},
		{http.MethodPatch, "/files/x"},
		{http.MethodPost, "/folders/x/files"},
		{http.MethodPut, "/files/x/content"},
	}
	ids := map[string]string{
		"bez zaglavlja": "",
		"samo razmaci":  "   ",
		"predugacak id": strings.Repeat("a", maxUserIDLen+1),
	}
	for _, rt := range routes {
		for name, id := range ids {
			t.Run(rt.method+" "+rt.path+"/"+name, func(t *testing.T) {
				h, _ := newTestHandler(t)
				req := httptest.NewRequest(rt.method, rt.path, strings.NewReader(`{}`))
				if id != "" {
					req.Header.Set("X-User-ID", id)
				}
				rec := httptest.NewRecorder()
				h.Routes().ServeHTTP(rec, req)
				if rec.Code != http.StatusUnauthorized {
					t.Errorf("status = %d, ocekivano 401", rec.Code)
				}
			})
		}
	}
}

// Tudj folder izgleda kao da ne postoji (404), i za citanje i za izmenu.
func TestOtherUsersFolderIsNotFound(t *testing.T) {
	h, fakultet := newTestHandler(t)

	if rec := getContents(h, fakultet.ID, "marko"); rec.Code != http.StatusNotFound {
		t.Errorf("sadrzaj tudjeg foldera: status = %d, ocekivano 404", rec.Code)
	}

	req := httptest.NewRequest(http.MethodPatch, "/folders/"+fakultet.ID, strings.NewReader(`{"name":"Hakovano"}`))
	req.Header.Set("X-User-ID", "marko")
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("preimenovanje tudjeg foldera: status = %d, ocekivano 404", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/folders", strings.NewReader(`{"name":"Upad","parentId":"`+fakultet.ID+`"}`))
	req.Header.Set("X-User-ID", "marko")
	rec = httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("folder u tudjem folderu: status = %d, ocekivano 404", rec.Code)
	}
}
