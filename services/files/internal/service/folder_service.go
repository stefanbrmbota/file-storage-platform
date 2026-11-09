package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/repository"
)

const (
	rootFolderName    = "root"
	maxFolderNameRune = 255
)

// Errors the handler turns into HTTP status codes.
var (
	ErrInvalidName    = errors.New("invalid folder name")
	ErrParentNotFound = errors.New("parent folder not found")
	ErrNameTaken      = errors.New("folder with this name already exists")
)

// FolderStore is what the service needs from the database. The real
// implementation is repository.FolderRepository; tests use a fake.
type FolderStore interface {
	Create(ctx context.Context, f *model.Folder) error
	FindByID(ctx context.Context, id string) (*model.Folder, error)
	FindRoot(ctx context.Context, ownerID string) (*model.Folder, error)
}

type FolderService struct {
	store FolderStore
	now   func() time.Time
}

func NewFolderService(store FolderStore) *FolderService {
	return &FolderService{store: store, now: time.Now}
}

// CreateRoot gives a newly registered user a root folder. Calling it again
// returns the existing root, so a retry from the Users service is safe.
// The bool reports whether a new folder was created.
func (s *FolderService) CreateRoot(ctx context.Context, ownerID string) (*model.Folder, bool, error) {
	root, err := s.store.FindRoot(ctx, ownerID)
	if err == nil {
		return root, false, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, false, err
	}

	root = s.newFolder(rootFolderName, ownerID, "")
	if err := s.store.Create(ctx, root); err != nil {
		return nil, false, err
	}
	return root, true, nil
}

// Create makes a folder inside parentID. An empty parentID means the user's
// root folder. The parent must exist and belong to the same user.
func (s *FolderService) Create(ctx context.Context, ownerID, name, parentID string) (*model.Folder, error) {
	name = strings.TrimSpace(name)
	if err := validateName(name); err != nil {
		return nil, err
	}

	parent, err := s.findParent(ctx, ownerID, parentID)
	if err != nil {
		return nil, err
	}

	folder := s.newFolder(name, ownerID, parent.ID)
	if err := s.store.Create(ctx, folder); err != nil {
		if errors.Is(err, repository.ErrDuplicateName) {
			return nil, ErrNameTaken
		}
		return nil, err
	}
	return folder, nil
}

func (s *FolderService) findParent(ctx context.Context, ownerID, parentID string) (*model.Folder, error) {
	var parent *model.Folder
	var err error
	if parentID == "" {
		parent, err = s.store.FindRoot(ctx, ownerID)
	} else {
		parent, err = s.store.FindByID(ctx, parentID)
	}
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrParentNotFound
	}
	if err != nil {
		return nil, err
	}
	// Someone else's folder looks exactly like a missing one, so the
	// response does not reveal that the folder exists.
	if parent.OwnerID != ownerID {
		return nil, ErrParentNotFound
	}
	return parent, nil
}

func (s *FolderService) newFolder(name, ownerID, parentID string) *model.Folder {
	now := s.now().UTC()
	return &model.Folder{
		ID:        uuid.NewString(),
		Name:      name,
		OwnerID:   ownerID,
		ParentID:  parentID,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// validateName allows any readable name up to 255 characters, except
// path separators, control characters and the special names "." and "..".
func validateName(name string) error {
	if name == "" || name == "." || name == ".." {
		return ErrInvalidName
	}
	if utf8.RuneCountInString(name) > maxFolderNameRune {
		return ErrInvalidName
	}
	for _, r := range name {
		if r == '/' || r == '\\' || unicode.IsControl(r) {
			return ErrInvalidName
		}
	}
	return nil
}
