package repository

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/model"
)

// FileRepository stores file metadata in the "files" MongoDB collection.
type FileRepository struct {
	coll *mongo.Collection
}

// NewFileRepository also creates a unique index on (ownerId, parentId, name).
func NewFileRepository(ctx context.Context, db *mongo.Database) (*FileRepository, error) {
	coll := db.Collection("files")
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

	return &FileRepository{coll: coll}, nil
}

func (r *FileRepository) FindByID(ctx context.Context, id string) (*model.File, error) {
	var f model.File
	err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&f)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// Rename changes a file's name and its last-modified time.
func (r *FileRepository) Rename(ctx context.Context, id, name string, updatedAt time.Time) error {
	return rename(ctx, r.coll, id, name, updatedAt)
}

// ListByParent returns all files in one folder, sorted by name.
func (r *FileRepository) ListByParent(ctx context.Context, ownerID, parentID string) ([]model.File, error) {
	filter := bson.M{"ownerId": ownerID, "parentId": parentID}
	opts := options.Find().SetSort(bson.D{{Key: "name", Value: 1}})

	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}

	files := []model.File{}
	if err := cursor.All(ctx, &files); err != nil {
		return nil, err
	}
	return files, nil
}
