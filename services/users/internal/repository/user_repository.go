package repository

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
)

var (
	// ErrNotFound se vraca kada korisnik ne postoji.
	ErrNotFound = errors.New("repository: korisnik nije pronadjen")
	// ErrEmailTaken se vraca kada korisnik sa tim emailom vec postoji.
	ErrEmailTaken = errors.New("repository: email je vec zauzet")
)

// UserRepository je pristup kolekciji korisnika.
type UserRepository interface {
	FindByEmail(ctx context.Context, email string) (model.User, error)
	FindByID(ctx context.Context, id string) (model.User, error)
	Create(ctx context.Context, u model.User) error
}

// MongoUserRepository je MongoDB implementacija UserRepository.
type MongoUserRepository struct {
	coll *mongo.Collection
}

// NewUserRepository vraca repozitorijum i obezbedjuje unique indeks na email.
// Indeks je prava zastita od duplikata, i kad se dve instance podignu istovremeno.
func NewUserRepository(ctx context.Context, db *mongo.Database) (*MongoUserRepository, error) {
	coll := db.Collection("users")
	_, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_email"),
	})
	if err != nil {
		return nil, fmt.Errorf("repository: kreiranje indeksa: %w", err)
	}
	return &MongoUserRepository{coll: coll}, nil
}

func (r *MongoUserRepository) FindByEmail(ctx context.Context, email string) (model.User, error) {
	var u model.User
	err := r.coll.FindOne(ctx, bson.D{{Key: "email", Value: email}}).Decode(&u)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.User{}, ErrNotFound
	}
	if err != nil {
		return model.User{}, fmt.Errorf("repository: trazenje korisnika: %w", err)
	}
	return u, nil
}

func (r *MongoUserRepository) FindByID(ctx context.Context, id string) (model.User, error) {
	var u model.User
	err := r.coll.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&u)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.User{}, ErrNotFound
	}
	if err != nil {
		return model.User{}, fmt.Errorf("repository: trazenje korisnika: %w", err)
	}
	return u, nil
}

func (r *MongoUserRepository) Create(ctx context.Context, u model.User) error {
	if _, err := r.coll.InsertOne(ctx, u); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrEmailTaken
		}
		return fmt.Errorf("repository: upis korisnika: %w", err)
	}
	return nil
}
