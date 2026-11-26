package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"

	"github.com/google/uuid"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/client"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/repository"
)

// NewVersionInput is new content for a file that already exists.
type NewVersionInput struct {
	OwnerID     string
	FileID      string
	ContentType string
	Size        int64
	Content     io.Reader
}

// UploadNewVersion replaces the content of an existing file (US-10). The file
// keeps its id, name and folder; size, type, hash and update time change.
//
//  1. only the owner may upload a new version
//  2. quota: reserve only the extra bytes if the new version is bigger
//  3. store the new content under a NEW object key (the old one stays)
//  4. switch the metadata to the new key
//  5. delete the old object and, if the file got smaller, release the difference
//
// Writing to a new key means the old version is never damaged: if step 3 or
// 4 fails, the file still points to the old, complete content.
func (s *FileService) UploadNewVersion(ctx context.Context, in NewVersionInput) (*model.File, error) {
	if in.Size < 0 {
		return nil, ErrInvalidSize
	}

	if err := validateID(in.FileID); err != nil {
		return nil, err
	}
	file, err := s.files.FindByID(ctx, in.FileID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrFileNotFound
	}
	if err != nil {
		return nil, err
	}
	if file.OwnerID != in.OwnerID {
		return nil, ErrFileNotFound
	}

	// Step 2: only the difference counts. A file that grows from 10 MB to
	// 12 MB needs 2 MB more; one that shrinks needs nothing reserved.
	diff := in.Size - file.Size
	if diff > 0 {
		if err := s.quota.Reserve(ctx, in.OwnerID, diff); err != nil {
			if errors.Is(err, client.ErrQuotaExceeded) {
				return nil, ErrQuotaExceeded
			}
			return nil, errors.Join(ErrQuotaUnavailable, err)
		}
	}

	// Step 3: new content under a new key.
	newKey := uuid.NewString()
	hasher := sha256.New()
	if err := s.objects.Put(ctx, newKey, io.TeeReader(in.Content, hasher), in.Size, in.ContentType); err != nil {
		if diff > 0 {
			s.releaseQuota(ctx, in.OwnerID, diff)
		}
		return nil, err
	}

	// Step 4: point the metadata to the new content.
	now := s.now().UTC()
	hash := hex.EncodeToString(hasher.Sum(nil))
	if err := s.files.UpdateContent(ctx, file.ID, newKey, in.Size, in.ContentType, hash, now); err != nil {
		s.deleteObject(ctx, newKey)
		if diff > 0 {
			s.releaseQuota(ctx, in.OwnerID, diff)
		}
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrFileNotFound
		}
		return nil, err
	}

	// Step 5: clean up. The new version is already saved, so a failure here
	// only leaves an unused object or unreleased bytes, and is logged.
	s.deleteObject(ctx, file.ObjectKey)
	if diff < 0 {
		s.releaseQuota(ctx, in.OwnerID, -diff)
	}

	file.ObjectKey = newKey
	file.Size = in.Size
	file.ContentType = in.ContentType
	file.Hash = hash
	file.UpdatedAt = now
	slog.Info("new file version uploaded", "fileId", file.ID, "size", file.Size)
	return file, nil
}
