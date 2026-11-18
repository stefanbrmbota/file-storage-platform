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

// RevokedTokenRepository je pristup listi opozvanih JWT-ova.
type RevokedTokenRepository interface {
	// Revoke upisuje jti; ponovni opoziv istog jti nije greška (idempotentno).
	Revoke(ctx context.Context, jti string, expiresAt time.Time) error
	// IsRevoked vraća true ako je jti opozvan i zapis još nije istekao.
	IsRevoked(ctx context.Context, jti string, now time.Time) (bool, error)
}

// MongoRevokedTokenRepository je MongoDB implementacija RevokedTokenRepository.
type MongoRevokedTokenRepository struct {
	coll *mongo.Collection
}

// NewRevokedTokenRepository vraća repozitorijum i obezbeđuje TTL indeks koji
// briše zapise kad istekne token.
func NewRevokedTokenRepository(ctx context.Context, db *mongo.Database) (*MongoRevokedTokenRepository, error) {
	coll := db.Collection("revoked_tokens")
	_, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "expiresAt", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(0).SetName("ttl_expiresAt"),
	})
	if err != nil {
		return nil, fmt.Errorf("repository: kreiranje TTL indeksa: %w", err)
	}
	return &MongoRevokedTokenRepository{coll: coll}, nil
}

func (r *MongoRevokedTokenRepository) Revoke(ctx context.Context, jti string, expiresAt time.Time) error {
	_, err := r.coll.InsertOne(ctx, model.RevokedToken{ID: jti, ExpiresAt: expiresAt})
	if err != nil && !mongo.IsDuplicateKeyError(err) {
		return fmt.Errorf("repository: opoziv tokena: %w", err)
	}
	return nil
}

func (r *MongoRevokedTokenRepository) IsRevoked(ctx context.Context, jti string, now time.Time) (bool, error) {
	// TTL monitor briše zapise sa zakašnjenjem, pa se istek proverava i u upitu.
	err := r.coll.FindOne(ctx, bson.D{
		{Key: "_id", Value: jti},
		{Key: "expiresAt", Value: bson.D{{Key: "$gt", Value: now}}},
	}).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("repository: provera opoziva: %w", err)
	}
	return true, nil
}
