package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/client"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/repository"
)

var (
	ErrInvalidSize      = errors.New("invalid file size")
	ErrQuotaExceeded    = errors.New("storage quota exceeded")
	ErrQuotaUnavailable = errors.New("quota check is temporarily unavailable, try again later")
)

// FileCreator saves file metadata (repository.FileRepository).
type FileCreator interface {
	Create(ctx context.Context, f *model.File) error
}

// ObjectStore keeps file contents (objectstore.Store, MinIO).
type ObjectStore interface {
	Put(ctx context.Context, key string, content io.Reader, size int64, contentType string) error
	Delete(ctx context.Context, key string) error
}

// QuotaClient reserves and releases storage space (client.UsersClient).
type QuotaClient interface {
	Reserve(ctx context.Context, userID string, bytes int64) error
	Release(ctx context.Context, userID string, bytes int64) error
}

// FileService handles operations on file contents.
type FileService struct {
	folders *FolderService
	files   FileCreator
	objects ObjectStore
	quota   QuotaClient
	now     func() time.Time
}

func NewFileService(folders *FolderService, files FileCreator, objects ObjectStore, quota QuotaClient) *FileService {
	return &FileService{folders: folders, files: files, objects: objects, quota: quota, now: time.Now}
}

// UploadInput is one file the user wants to upload.
type UploadInput struct {
	OwnerID     string
	FolderID    string // "root" or empty means the user's root folder
	Name        string
	ContentType string
	Size        int64
	Content     io.Reader
}

// Upload stores a new file in one of the user's folders (US-02):
//
//  1. check the name and that the folder is the user's own
//  2. reserve quota in the Users service (sync call, design 5.4 S3)
//  3. stream the content to MinIO and compute its SHA-256 on the way
//  4. save the metadata in MongoDB
//
// If step 3 or 4 fails, the steps before it are undone (compensation):
// the object is deleted and the reserved quota is released.
func (s *FileService) Upload(ctx context.Context, in UploadInput) (*model.File, error) {
	name := strings.TrimSpace(in.Name)
	if err := validateName(name); err != nil {
		return nil, err
	}
	if in.Size < 0 {
		return nil, ErrInvalidSize
	}

	folder, err := s.folders.ownFolder(ctx, in.OwnerID, in.FolderID)
	if err != nil {
		return nil, err
	}

	// Step 2: reserve space before anything is written.
	if err := s.quota.Reserve(ctx, in.OwnerID, in.Size); err != nil {
		if errors.Is(err, client.ErrQuotaExceeded) {
			return nil, ErrQuotaExceeded
		}
		return nil, errors.Join(ErrQuotaUnavailable, err)
	}

	// Step 3: content to MinIO. The hash is computed while the bytes flow
	// through, so the file is never held in memory as a whole.
	key := uuid.NewString()
	hasher := sha256.New()
	if err := s.objects.Put(ctx, key, io.TeeReader(in.Content, hasher), in.Size, in.ContentType); err != nil {
		s.releaseQuota(ctx, in.OwnerID, in.Size)
		return nil, err
	}

	// Step 4: metadata to MongoDB.
	now := s.now().UTC()
	file := &model.File{
		ID:          uuid.NewString(),
		Name:        name,
		ContentType: in.ContentType,
		Size:        in.Size,
		OwnerID:     in.OwnerID,
		ParentID:    folder.ID,
		ObjectKey:   key,
		Hash:        hex.EncodeToString(hasher.Sum(nil)),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.files.Create(ctx, file); err != nil {
		s.deleteObject(ctx, key)
		s.releaseQuota(ctx, in.OwnerID, in.Size)
		if errors.Is(err, repository.ErrDuplicateName) {
			return nil, ErrNameTaken
		}
		return nil, err
	}
	return file, nil
}

// Compensations run even if the user's request was cancelled, so they use a
// context without the request's cancellation. Their errors are only logged:
// the user already gets the original error.
func (s *FileService) releaseQuota(ctx context.Context, userID string, bytes int64) {
	if err := s.quota.Release(context.WithoutCancel(ctx), userID, bytes); err != nil {
		slog.Error("compensation: quota release failed", "userId", userID, "bytes", bytes, "err", err)
	}
}

func (s *FileService) deleteObject(ctx context.Context, key string) {
	if err := s.objects.Delete(context.WithoutCancel(ctx), key); err != nil {
		slog.Error("compensation: object delete failed", "key", key, "err", err)
	}
}
