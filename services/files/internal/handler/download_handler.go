package handler

import (
	"errors"
	"log/slog"
	"mime"
	"net/http"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/service"
)

// GET /files/{id}/content
// Preuzimanje datoteke (US-65): sadržaj se šalje samo ako se njegov SHA-256
// poklapa sa hešom sačuvanim pri otpremanju.
func (h *Handler) downloadFile(w http.ResponseWriter, r *http.Request) {
	owner, ok := userID(w, r)
	if !ok {
		return
	}

	d, err := h.files.Download(r.Context(), owner, r.PathValue("id"))
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	defer d.Close()

	// Naziv datoteke je korisnički podatak: FormatMediaType ga enkoduje
	// (navodnici, ne-ASCII znakovi po RFC 2231), pa ne može da razbije zaglavlje.
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": d.File.Name})
	if disposition == "" {
		disposition = "attachment"
	}
	w.Header().Set("Content-Disposition", disposition)
	w.Header().Set("Content-Type", d.File.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, "", d.File.UpdatedAt, d.Content)
}

func writeDownloadError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrIntegrity):
		writeError(w, http.StatusInternalServerError, "file is damaged and cannot be downloaded")
	case errors.Is(err, service.ErrFileNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		slog.Error("download failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
