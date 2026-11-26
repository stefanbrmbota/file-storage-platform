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

// PasswordResetRepository je pristup tokenima za reset lozinke (US-15).
// Korisnik ima najviše jedan aktivan token.
type PasswordResetRepository interface {
	// Save upisuje token korisnika; postojeći token istog korisnika se zamenjuje
	// (jedna atomična operacija), pa stari link prestaje da važi.
	Save(ctx context.Context, t model.PasswordResetToken) error
	// Get vraća aktivan token korisnika (npr. za ograničenje učestalosti slanja).
	// ErrTokenNotFound ako ga nema.
	Get(ctx context.Context, userID string) (model.PasswordResetToken, error)
	// Consume atomično uzima i briše token koji nije istekao (jednokratnost).
	// ErrTokenNotFound ako ne postoji, istekao je ili je već iskorišćen.
	Consume(ctx context.Context, tokenHash string, now time.Time) (model.PasswordResetToken, error)
	// Restore vraća potrošen token, ali samo ako korisnik u međuvremenu nije
	// dobio novi (novi token se ne gazi starim). Idempotentno.
	Restore(ctx context.Context, t model.PasswordResetToken) error
	// Delete briše token korisnika; idempotentno.
	Delete(ctx context.Context, userID string) error
}

// MongoPasswordResetRepository je MongoDB implementacija PasswordResetRepository.
type MongoPasswordResetRepository struct {
	coll *mongo.Collection
}

// NewPasswordResetRepository vraća repozitorijum i obezbeđuje TTL indeks koji
// briše istekle tokene i jedinstven indeks na heš tokena.
func NewPasswordResetRepository(ctx context.Context, db *mongo.Database) (*MongoPasswordResetRepository, error) {
	coll := db.Collection("password_reset_tokens")
	_, err := coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "expiresAt", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(0).SetName("ttl_expiresAt"),
		},
		{
			Keys:    bson.D{{Key: "tokenHash", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uniq_tokenHash"),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("repository: kreiranje indeksa za reset lozinke: %w", err)
	}
	return &MongoPasswordResetRepository{coll: coll}, nil
}

func (r *MongoPasswordResetRepository) Save(ctx context.Context, t model.PasswordResetToken) error {
	_, err := r.coll.ReplaceOne(ctx, bson.D{{Key: "_id", Value: t.UserID}}, t, options.Replace().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("repository: upis tokena za reset: %w", err)
	}
	return nil
}

func (r *MongoPasswordResetRepository) Get(ctx context.Context, userID string) (model.PasswordResetToken, error) {
	var t model.PasswordResetToken
	err := r.coll.FindOne(ctx, bson.D{{Key: "_id", Value: userID}}).Decode(&t)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.PasswordResetToken{}, ErrTokenNotFound
	}
	if err != nil {
		return model.PasswordResetToken{}, fmt.Errorf("repository: citanje tokena za reset: %w", err)
	}
	return t, nil
}

func (r *MongoPasswordResetRepository) Consume(ctx context.Context, tokenHash string, now time.Time) (model.PasswordResetToken, error) {
	var t model.PasswordResetToken
	err := r.coll.FindOneAndDelete(ctx, bson.D{
		{Key: "tokenHash", Value: tokenHash},
		{Key: "expiresAt", Value: bson.D{{Key: "$gt", Value: now}}},
	}).Decode(&t)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.PasswordResetToken{}, ErrTokenNotFound
	}
	if err != nil {
		return model.PasswordResetToken{}, fmt.Errorf("repository: uzimanje tokena za reset: %w", err)
	}
	return t, nil
}

func (r *MongoPasswordResetRepository) Restore(ctx context.Context, t model.PasswordResetToken) error {
	if _, err := r.coll.InsertOne(ctx, t); err != nil && !mongo.IsDuplicateKeyError(err) {
		return fmt.Errorf("repository: vracanje tokena za reset: %w", err)
	}
	return nil
}

func (r *MongoPasswordResetRepository) Delete(ctx context.Context, userID string) error {
	if _, err := r.coll.DeleteOne(ctx, bson.D{{Key: "_id", Value: userID}}); err != nil {
		return fmt.Errorf("repository: brisanje tokena za reset: %w", err)
	}
	return nil
}
