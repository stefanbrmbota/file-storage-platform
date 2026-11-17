package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
)

// ErrTokenNotFound se vraća kada token ne postoji, istekao je ili je već iskorišćen.
var ErrTokenNotFound = errors.New("repository: token nije pronadjen")

// TokenRepository je pristup tokenima za potvrdu registracije.
type TokenRepository interface {
	Create(ctx context.Context, t model.ConfirmationToken) error
	// Consume atomično uzima i briše token koji nije istekao (jednokratnost).
	Consume(ctx context.Context, hash string, now time.Time) (model.ConfirmationToken, error)
}

// MongoTokenRepository je MongoDB implementacija TokenRepository.
type MongoTokenRepository struct {
	coll *mongo.Collection
}

// NewTokenRepository vraća repozitorijum i obezbeđuje TTL indeks koji
// briše istekle tokene.
func NewTokenRepository(ctx context.Context, db *mongo.Database) (*MongoTokenRepository, error) {
	coll := db.Collection("confirmation_tokens")
	_, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "expiresAt", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(0).SetName("ttl_expiresAt"),
	})
	if err != nil {
		return nil, fmt.Errorf("repository: kreiranje TTL indeksa: %w", err)
	}
	return &MongoTokenRepository{coll: coll}, nil
}

func (r *MongoTokenRepository) Create(ctx context.Context, t model.ConfirmationToken) error {
	if _, err := r.coll.InsertOne(ctx, t); err != nil {
		return fmt.Errorf("repository: upis tokena: %w", err)
	}
	return nil
}

func (r *MongoTokenRepository) Consume(ctx context.Context, hash string, now time.Time) (model.ConfirmationToken, error) {
	var t model.ConfirmationToken
	err := r.coll.FindOneAndDelete(ctx, bson.D{
		{Key: "_id", Value: hash},
		{Key: "expiresAt", Value: bson.D{{Key: "$gt", Value: now}}},
	}).Decode(&t)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.ConfirmationToken{}, ErrTokenNotFound
	}
	if err != nil {
		return model.ConfirmationToken{}, fmt.Errorf("repository: uzimanje tokena: %w", err)
	}
	return t, nil
}
