package repository

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/model"
)

// Errors the service layer checks for. The service never sees MongoDB errors directly.
var (
	ErrNotFound      = errors.New("not found")
	ErrDuplicateName = errors.New("duplicate name")
)

// FolderRepository stores folders in the "folders" MongoDB collection.
type FolderRepository struct {
	coll *mongo.Collection
}

// NewFolderRepository also creates a unique index on (ownerId, parentId, name),
// so the database itself rejects two folders with the same name in one parent.
func NewFolderRepository(ctx context.Context, db *mongo.Database) (*FolderRepository, error) {
	coll := db.Collection("folders")
	_, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "ownerId", Value: 1},
			{Key: "parentId", Value: 1},
			{Key: "name", Value: 1},
		},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return nil, err
	}
	return &FolderRepository{coll: coll}, nil
}

func (r *FolderRepository) Create(ctx context.Context, f *model.Folder) error {
	_, err := r.coll.InsertOne(ctx, f)
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicateName
	}
	return err
}

func (r *FolderRepository) FindByID(ctx context.Context, id string) (*model.Folder, error) {
	return r.findOne(ctx, bson.M{"_id": id})
}

// FindRoot returns the user's root folder (the only folder without a parent).
func (r *FolderRepository) FindRoot(ctx context.Context, ownerID string) (*model.Folder, error) {
	return r.findOne(ctx, bson.M{"ownerId": ownerID, "parentId": ""})
}

func (r *FolderRepository) findOne(ctx context.Context, filter bson.M) (*model.Folder, error) {
	var f model.Folder
	err := r.coll.FindOne(ctx, filter).Decode(&f)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}
