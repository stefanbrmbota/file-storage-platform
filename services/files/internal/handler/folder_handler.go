package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/service"
)

type createFolderRequest struct {
	Name     string `json:"name"`
	ParentID string `json:"parentId"`
}

type createRootRequest struct {
	UserID string `json:"userId"`
}

// POST /folders  {"name": "Fakultet", "parentId": "..."}
// An empty or missing parentId creates the folder in the user's root folder.
func (h *Handler) createFolder(w http.ResponseWriter, r *http.Request) {
	owner, ok := userID(w, r)
	if !ok {
		return
	}
	var req createFolderRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	folder, err := h.folders.Create(r.Context(), owner, req.Name, req.ParentID)
	if err != nil {
		writeFolderError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, folder)
}

// POST /internal/folders/root  {"userId": "..."}
// Called by the Users service after registration (US-04).
func (h *Handler) createRootFolder(w http.ResponseWriter, r *http.Request) {
	var req createRootRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.UserID == "" {
		writeError(w, http.StatusBadRequest, "userId is required")
		return
	}

	root, created, err := h.folders.CreateRoot(r.Context(), req.UserID)
	if err != nil {
		writeFolderError(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, root)
}

// writeFolderError maps service errors to HTTP status codes.
func writeFolderError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidName):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrParentNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrNameTaken):
		writeError(w, http.StatusConflict, err.Error())
	default:
		slog.Error("folder request failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
