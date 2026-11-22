package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func patch(h *Handler, path, body, user string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPatch, path, strings.NewReader(body))
	if user != "" {
		req.Header.Set("X-User-ID", user)
	}
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	return rec
}

func TestRenameFolderEndpoint(t *testing.T) {
	h, fakultet := newTestHandler(t)

	rec := patch(h, "/folders/"+fakultet.ID, `{"name":"Faks"}`, "ana")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body)
	}
	var body struct{ Name string }
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Name != "Faks" {
		t.Errorf("name = %q, want Faks", body.Name)
	}
}

func TestRenameFolderEndpointErrors(t *testing.T) {
	h, fakultet := newTestHandler(t)
	slike, err := h.folders.Create(context.Background(), "ana", "Slike", "")
	if err != nil {
		t.Fatal(err)
	}
	root, _ := h.folders.Contents(context.Background(), "ana", "root")

	tests := []struct {
		name string
		path string
		body string
		user string
		want int
	}{
		{"no user", "/folders/" + fakultet.ID, `{"name":"x"}`, "", http.StatusUnauthorized},
		{"bad json", "/folders/" + fakultet.ID, `{"name":`, "ana", http.StatusBadRequest},
		{"invalid name", "/folders/" + fakultet.ID, `{"name":"a/b"}`, "ana", http.StatusBadRequest},
		{"root folder", "/folders/" + root.Folder.ID, `{"name":"x"}`, "ana", http.StatusBadRequest},
		{"name taken", "/folders/" + slike.ID, `{"name":"Fakultet"}`, "ana", http.StatusConflict},
		{"unknown folder", "/folders/nope", `{"name":"x"}`, "ana", http.StatusNotFound},
		{"someone else's folder", "/folders/" + fakultet.ID, `{"name":"x"}`, "marko", http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := patch(h, tc.path, tc.body, tc.user)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d, body %s", rec.Code, tc.want, rec.Body)
			}
		})
	}
}

func TestRenameFileEndpoint(t *testing.T) {
	h, _, files := newTestHandlerWithFiles(t)

	rec := patch(h, "/files/f1", `{"name":"biografija.pdf"}`, "ana")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body)
	}
	if files.items[0].Name != "biografija.pdf" {
		t.Errorf("stored name = %q, want biografija.pdf", files.items[0].Name)
	}

	for _, tc := range []struct {
		name, path, user string
		want             int
	}{
		{"unknown file", "/files/nope", "ana", http.StatusNotFound},
		{"someone else's file", "/files/f1", "marko", http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := patch(h, tc.path, `{"name":"x.txt"}`, tc.user)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
