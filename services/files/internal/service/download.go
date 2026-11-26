package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/objectstore"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/repository"
)

// ErrIntegrity znači da sadržaj u object store-u nije isti kao pri otpremanju:
// SHA-256 se ne poklapa ili objekat fali (US-65). Takav sadržaj se ne isporučuje.
var ErrIntegrity = errors.New("file integrity check failed")

// Download je proveren sadržaj datoteke spreman za slanje. Content je
// privremena kopija na disku; pozivalac mora da pozove Close.
type Download struct {
	File    *model.File
	Content *os.File
}

// Close zatvara i briše privremenu kopiju.
func (d *Download) Close() error {
	name := d.Content.Name()
	err := d.Content.Close()
	_ = os.Remove(name)
	return err
}

// Download vraća sadržaj datoteke vlasniku, tek pošto proveri njen integritet:
//
//  1. samo vlasnik sme da preuzme datoteku (tuđa izgleda kao nepostojeća)
//  2. sadržaj iz MinIO-a se kopira u privremenu datoteku i usput hešira
//  3. heš se poredi sa SHA-256 sačuvanim pri otpremanju
//
// Sadržaj se klijentu šalje tek posle provere, pa izmenjena datoteka nikad
// ne stiže do korisnika ni delimično.
func (s *FileService) Download(ctx context.Context, userID, fileID string) (*Download, error) {
	file, err := s.files.FindByID(ctx, fileID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrFileNotFound
	}
	if err != nil {
		return nil, err
	}
	if file.OwnerID != userID {
		return nil, ErrFileNotFound
	}

	obj, err := s.objects.Get(ctx, file.ObjectKey)
	if errors.Is(err, objectstore.ErrNotFound) {
		slog.Error("integritet: sadržaj datoteke ne postoji u object store-u", "fileId", file.ID)
		return nil, ErrIntegrity
	}
	if err != nil {
		return nil, err
	}
	defer obj.Close()

	tmp, err := os.CreateTemp("", "fsp-download-*")
	if err != nil {
		return nil, fmt.Errorf("download temp file: %w", err)
	}
	d := &Download{File: file, Content: tmp}

	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, hasher), obj); err != nil {
		_ = d.Close()
		return nil, fmt.Errorf("download copy: %w", err)
	}
	if got := hex.EncodeToString(hasher.Sum(nil)); file.Hash == "" || got != file.Hash {
		_ = d.Close()
		slog.Error("integritet: SHA-256 sadržaja se ne poklapa sa sačuvanim, preuzimanje odbijeno",
			"fileId", file.ID, "ownerId", file.OwnerID)
		return nil, ErrIntegrity
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		_ = d.Close()
		return nil, fmt.Errorf("download seek: %w", err)
	}
	return d, nil
}
