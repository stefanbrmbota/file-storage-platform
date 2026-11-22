package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/service"
)

// Parts of an upload up to this size stay in memory; bigger files are
// written to a temporary file on disk, so memory use stays small.
const uploadMemoryLimit = 8 << 20 // 8 MB

// POST /folders/{id}/files   (multipart/form-data, field "file")
// Use "root" as the id to upload into the user's root folder.
func (h *Handler) uploadFile(w http.ResponseWriter, r *http.Request) {
	owner, ok := userID(w, r)
	if !ok {
		return
	}

	// Refuse requests bigger than the limit before reading them completely.
	r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadBytes)
	if err := r.ParseMultipartForm(uploadMemoryLimit); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "file is too large")
			return
		}
		writeError(w, http.StatusBadRequest, "request must be multipart/form-data with a file field")
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }() // deletes temporary files

	part, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer part.Close()

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	file, err := h.files.Upload(r.Context(), service.UploadInput{
		OwnerID:     owner,
		FolderID:    r.PathValue("id"),
		Name:        header.Filename,
		ContentType: contentType,
		Size:        header.Size,
		Content:     part,
	})
	if err != nil {
		writeUploadError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, file)
}

// writeUploadError maps upload errors to HTTP status codes.
func writeUploadError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrQuotaExceeded):
		writeError(w, http.StatusInsufficientStorage, err.Error())
	case errors.Is(err, service.ErrQuotaUnavailable):
		slog.Error("upload refused, quota check failed", "err", err)
		writeError(w, http.StatusServiceUnavailable, service.ErrQuotaUnavailable.Error())
	case errors.Is(err, service.ErrInvalidSize):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeFolderError(w, err)
	}
}
