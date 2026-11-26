package handler

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/client"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/service"
)

type Handler struct {
	folders        *service.FolderService
	files          *service.FileService
	maxUploadBytes int64
	// internalToken štiti rute /internal/... (US-62, T-62.2).
	internalToken string
}

// New creates the handler. maxUploadBytes limits the size of one upload request.
func New(folders *service.FolderService, files *service.FileService, maxUploadBytes int64) *Handler {
	return &Handler{folders: folders, files: files, maxUploadBytes: maxUploadBytes}
}

// WithInternalToken postavlja tajnu (INTERNAL_TOKEN) koju drugi servisi
// šalju u X-Internal-Token kad zovu /internal/... rute. Bez nje su interne
// rute uvek zatvorene (fail closed).
func (h *Handler) WithInternalToken(token string) *Handler {
	h.internalToken = token
	return h
}

// Routes registers all HTTP routes of the Files service.
// Routes under /internal are for other services only; the API gateway must not expose them.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("POST /folders", h.createFolder)
	mux.HandleFunc("GET /folders/{id}/contents", h.folderContents)
	mux.HandleFunc("GET /files/{id}/content", h.downloadFile)
	mux.HandleFunc("PATCH /folders/{id}", h.renameFolder)
	mux.HandleFunc("PATCH /files/{id}", h.renameFile)
	mux.HandleFunc("POST /folders/{id}/files", h.uploadFile)
	mux.HandleFunc("PUT /files/{id}/content", h.uploadNewVersion)
	mux.Handle("POST /internal/folders/root", requireInternalToken(h.internalToken, http.HandlerFunc(h.createRootFolder)))
	return mux
}

// MinInternalTokenLen je ista granica kao u Users servisu.
const MinInternalTokenLen = 32

// requireInternalToken propušta samo zahteve sa ispravnim X-Internal-Token.
// Ako tajna nije podešena ili je prekratka, odbija sve (fail closed).
// Poredi se SHA-256 u konstantnom vremenu, pa vreme odgovora ne otkriva tajnu.
func requireInternalToken(token string, next http.Handler) http.Handler {
	want := sha256.Sum256([]byte(token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := sha256.Sum256([]byte(r.Header.Get(client.InternalTokenHeader)))
		if len(token) < MinInternalTokenLen || subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	setJSONHeaders(w)
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

// maxUserIDLen je ista granica kao u Users servisu.
const maxUserIDLen = 128

// userID returns the logged-in user's ID. The API gateway checks the token
// and forwards the ID in the X-User-ID header. Identitet nikad ne dolazi iz
// URL-a ni tela zahteva (US-62).
func userID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.Header.Get("X-User-ID")
	if strings.TrimSpace(id) == "" || len(id) > maxUserIDLen {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return "", false
	}
	return id, true
}
