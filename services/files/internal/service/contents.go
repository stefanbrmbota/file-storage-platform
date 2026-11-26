package service

import (
	"context"
	"errors"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/repository"
)

// RootFolderID is the alias clients use for "my root folder".
const RootFolderID = "root"

// maxDepth stops the path walk if the data is ever broken (for example a
// folder that points to itself), so the request cannot loop forever.
const maxDepth = 1000

var ErrFolderNotFound = errors.New("folder not found")

// PathItem is one step in the path from the root to the current folder.
type PathItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// FolderContents is everything the client needs to show one folder.
type FolderContents struct {
	Folder  *model.Folder  `json:"folder"`
	Path    []PathItem     `json:"path"`
	Folders []model.Folder `json:"folders"`
	Files   []model.File   `json:"files"`
}

// Contents returns the subfolders and files of one folder and the path from
// the root to it. folderID "root" (or empty) means the user's root folder.
// Only the owner can see a folder for now; shared access comes with US-38.
func (s *FolderService) Contents(ctx context.Context, ownerID, folderID string) (*FolderContents, error) {
	folder, err := s.ownFolder(ctx, ownerID, folderID)
	if err != nil {
		return nil, err
	}

	path, err := s.pathTo(ctx, folder)
	if err != nil {
		return nil, err
	}

	folders, err := s.store.ListByParent(ctx, ownerID, folder.ID)
	if err != nil {
		return nil, err
	}

	files, err := s.files.ListByParent(ctx, ownerID, folder.ID)
	if err != nil {
		return nil, err
	}

	return &FolderContents{Folder: folder, Path: path, Folders: folders, Files: files}, nil
}

// ownFolder loads a folder and checks that it belongs to the user.
// Someone else's folder is reported as not found, same as in Create.
func (s *FolderService) ownFolder(ctx context.Context, ownerID, folderID string) (*model.Folder, error) {
	var folder *model.Folder
	var err error
	if folderID == "" || folderID == RootFolderID {
		folder, err = s.store.FindRoot(ctx, ownerID)
	} else {
		if err := validateID(folderID); err != nil {
			return nil, err
		}
		folder, err = s.store.FindByID(ctx, folderID)
	}
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrFolderNotFound
	}
	if err != nil {
		return nil, err
	}
	if folder.OwnerID != ownerID {
		return nil, ErrFolderNotFound
	}
	return folder, nil
}

// pathTo walks from the folder up to the root through ParentID and returns
// the steps in order root -> ... -> folder.
func (s *FolderService) pathTo(ctx context.Context, folder *model.Folder) ([]PathItem, error) {
	path := []PathItem{{ID: folder.ID, Name: folder.Name}}
	current := folder

	for current.ParentID != "" {
		if len(path) > maxDepth {
			return nil, errors.New("folder path is too deep")
		}
		parent, err := s.store.FindByID(ctx, current.ParentID)
		if err != nil {
			return nil, err
		}
		path = append(path, PathItem{ID: parent.ID, Name: parent.Name})
		current = parent
	}

	// We collected folder -> root, the client needs root -> folder.
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path, nil
}
