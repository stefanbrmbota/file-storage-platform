package service

import (
	"context"
	"errors"
	"strings"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/repository"
)

var (
	ErrFileNotFound     = errors.New("file not found")
	ErrCannotRenameRoot = errors.New("the root folder cannot be renamed")
)

// RenameFolder gives a folder a new name. The rules are the same as when the
// folder was created: valid name, unique in its parent, only the owner.
// The root folder has no parent and is never renamed.
func (s *FolderService) RenameFolder(ctx context.Context, ownerID, folderID, name string) (*model.Folder, error) {
	name = strings.TrimSpace(name)
	if err := validateName(name); err != nil {
		return nil, err
	}

	if err := validateID(folderID); err != nil {
		return nil, err
	}
	folder, err := s.store.FindByID(ctx, folderID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrFolderNotFound
	}
	if err != nil {
		return nil, err
	}
	if folder.OwnerID != ownerID {
		return nil, ErrFolderNotFound
	}
	if folder.ParentID == "" {
		return nil, ErrCannotRenameRoot
	}
	if folder.Name == name {
		return folder, nil
	}

	now := s.now().UTC()
	if err := s.store.Rename(ctx, folder.ID, name, now); err != nil {
		return nil, renameError(err, ErrFolderNotFound)
	}
	folder.Name = name
	folder.UpdatedAt = now
	return folder, nil
}

// RenameFile gives a file a new name with the same rules as RenameFolder.
func (s *FolderService) RenameFile(ctx context.Context, ownerID, fileID, name string) (*model.File, error) {
	name = strings.TrimSpace(name)
	if err := validateName(name); err != nil {
		return nil, err
	}

	if err := validateID(fileID); err != nil {
		return nil, err
	}
	file, err := s.files.FindByID(ctx, fileID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrFileNotFound
	}
	if err != nil {
		return nil, err
	}
	if file.OwnerID != ownerID {
		return nil, ErrFileNotFound
	}
	if file.Name == name {
		return file, nil
	}

	now := s.now().UTC()
	if err := s.files.Rename(ctx, file.ID, name, now); err != nil {
		return nil, renameError(err, ErrFileNotFound)
	}
	file.Name = name
	file.UpdatedAt = now
	return file, nil
}

// renameError turns repository errors into service errors.
func renameError(err, notFound error) error {
	switch {
	case errors.Is(err, repository.ErrDuplicateName):
		return ErrNameTaken
	case errors.Is(err, repository.ErrNotFound):
		return notFound
	default:
		return err
	}
}
