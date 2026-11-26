package handler

import (
	"errors"
	"log/slog"
	"mime/multipart"
	"net/http"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/service"
)

// Parts of an upload up to this size stay in memory; bigger files are
// written to a temporary file on disk, so memory use stays small.
const uploadMemoryLimit = 8 << 20 // 8 MB

// uploadedFile is the "file" field of a multipart/form-data request.
type uploadedFile struct {
	content     multipart.File
	name        string
	size        int64
	contentType string
}

// readUploadedFile reads the "file" field and writes the error response
// itself if something is wrong. The caller must call cleanup when done: it
// closes the file and deletes temporary files.
func (h *Handler) readUploadedFile(w http.ResponseWriter, r *http.Request) (f uploadedFile, cleanup func(), ok bool) {
	// Refuse requests bigger than the limit before reading them completely.
	r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadBytes)
	if err := r.ParseMultipartForm(uploadMemoryLimit); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "file is too large")
			return f, nil, false
		}
		writeError(w, http.StatusBadRequest, "request must be multipart/form-data with a file field")
		return f, nil, false
	}

	part, header, err := r.FormFile("file")
	if err != nil {
		_ = r.MultipartForm.RemoveAll()
		writeError(w, http.StatusBadRequest, "missing file field")
		return f, nil, false
	}

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	cleanup = func() {
		_ = part.Close()
		_ = r.MultipartForm.RemoveAll()
	}
	return uploadedFile{content: part, name: header.Filename, size: header.Size, contentType: contentType}, cleanup, true
}

// POST /folders/{id}/files   (multipart/form-data, field "file")
// Use "root" as the id to upload into the user's root folder.
func (h *Handler) uploadFile(w http.ResponseWriter, r *http.Request) {
	owner, ok := userID(w, r)
	if !ok {
		return
	}
	upload, cleanup, ok := h.readUploadedFile(w, r)
	if !ok {
		return
	}
	defer cleanup()

	file, err := h.files.Upload(r.Context(), service.UploadInput{
		OwnerID:     owner,
		FolderID:    r.PathValue("id"),
		Name:        upload.name,
		ContentType: upload.contentType,
		Size:        upload.size,
		Content:     upload.content,
	})
	if err != nil {
		writeUploadError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, file)
}

// PUT /files/{id}/content   (multipart/form-data, field "file")
// Replaces the content of an existing file with a new version (US-10).
// The file keeps its name; the name of the uploaded file is ignored.
func (h *Handler) uploadNewVersion(w http.ResponseWriter, r *http.Request) {
	owner, ok := userID(w, r)
	if !ok {
		return
	}
	upload, cleanup, ok := h.readUploadedFile(w, r)
	if !ok {
		return
	}
	defer cleanup()

	file, err := h.files.UploadNewVersion(r.Context(), service.NewVersionInput{
		OwnerID:     owner,
		FileID:      r.PathValue("id"),
		ContentType: upload.contentType,
		Size:        upload.size,
		Content:     upload.content,
	})
	if err != nil {
		writeUploadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, file)
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
	case errors.Is(err, service.ErrFileTypeNotAllowed):
		writeError(w, http.StatusUnsupportedMediaType, err.Error())
	default:
		writeFolderError(w, err)
	}
}
