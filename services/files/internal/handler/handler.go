package handler

import (
	"encoding/json"
	"net/http"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/service"
)

type Handler struct {
	folders *service.FolderService
}

func New(folders *service.FolderService) *Handler {
	return &Handler{folders: folders}
}

// Routes registers all HTTP routes of the Files service.
// Routes under /internal are for other services only; the API gateway must not expose them.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("POST /folders", h.createFolder)
	mux.HandleFunc("POST /internal/folders/root", h.createRootFolder)
	return mux
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError returns a short message to the client, without internal details.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// decodeJSON reads a small JSON body and rejects unknown fields.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

// userID returns the logged-in user's ID. The API gateway checks the token
// and forwards the ID in the X-User-ID header.
func userID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.Header.Get("X-User-ID")
	if id == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return "", false
	}
	return id, true
}
