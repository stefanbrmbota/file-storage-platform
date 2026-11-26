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

// MagicLinkRepository je pristup tokenima magičnog linka za povraćaj naloga
// (US-17). Korisnik ima najviše jedan aktivan token. Greška za token koji ne
// postoji, istekao je ili je već iskorišćen je ErrTokenNotFound.
type MagicLinkRepository interface {
	// Save upisuje token korisnika; postojeći token istog korisnika se zamenjuje
	// (jedna atomična operacija), pa stari link prestaje da važi.
	Save(ctx context.Context, t model.MagicLinkToken) error
	// Get vraća aktivan token korisnika (npr. za ograničenje učestalosti slanja).
	// ErrTokenNotFound ako ga nema.
	Get(ctx context.Context, userID string) (model.MagicLinkToken, error)
	// Consume atomično uzima i briše token koji nije istekao (jednokratnost).
	// ErrTokenNotFound ako ne postoji, istekao je ili je već iskorišćen.
	Consume(ctx context.Context, tokenHash string, now time.Time) (model.MagicLinkToken, error)
	// Restore vraća potrošen token, ali samo ako korisnik u međuvremenu nije
	// dobio novi (novi token se ne gazi starim). Idempotentno.
	Restore(ctx context.Context, t model.MagicLinkToken) error
	// Delete briše token korisnika; idempotentno.
	Delete(ctx context.Context, userID string) error
}

// MongoMagicLinkRepository je MongoDB implementacija MagicLinkRepository.
type MongoMagicLinkRepository struct {
	coll *mongo.Collection
}

// NewMagicLinkRepository vraća repozitorijum i obezbeđuje TTL indeks koji
// briše istekle tokene i jedinstven indeks na heš tokena.
func NewMagicLinkRepository(ctx context.Context, db *mongo.Database) (*MongoMagicLinkRepository, error) {
	coll := db.Collection("magic_link_tokens")
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
		return nil, fmt.Errorf("repository: kreiranje indeksa za magicni link: %w", err)
	}
	return &MongoMagicLinkRepository{coll: coll}, nil
}

func (r *MongoMagicLinkRepository) Save(ctx context.Context, t model.MagicLinkToken) error {
	_, err := r.coll.ReplaceOne(ctx, bson.D{{Key: "_id", Value: t.UserID}}, t, options.Replace().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("repository: upis tokena magicnog linka: %w", err)
	}
	return nil
}

func (r *MongoMagicLinkRepository) Get(ctx context.Context, userID string) (model.MagicLinkToken, error) {
	var t model.MagicLinkToken
	err := r.coll.FindOne(ctx, bson.D{{Key: "_id", Value: userID}}).Decode(&t)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.MagicLinkToken{}, ErrTokenNotFound
	}
	if err != nil {
		return model.MagicLinkToken{}, fmt.Errorf("repository: citanje tokena magicnog linka: %w", err)
	}
	return t, nil
}

func (r *MongoMagicLinkRepository) Consume(ctx context.Context, tokenHash string, now time.Time) (model.MagicLinkToken, error) {
	var t model.MagicLinkToken
	err := r.coll.FindOneAndDelete(ctx, bson.D{
		{Key: "tokenHash", Value: tokenHash},
		{Key: "expiresAt", Value: bson.D{{Key: "$gt", Value: now}}},
	}).Decode(&t)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.MagicLinkToken{}, ErrTokenNotFound
	}
	if err != nil {
		return model.MagicLinkToken{}, fmt.Errorf("repository: uzimanje tokena magicnog linka: %w", err)
	}
	return t, nil
}

func (r *MongoMagicLinkRepository) Restore(ctx context.Context, t model.MagicLinkToken) error {
	if _, err := r.coll.InsertOne(ctx, t); err != nil && !mongo.IsDuplicateKeyError(err) {
		return fmt.Errorf("repository: vracanje tokena magicnog linka: %w", err)
	}
	return nil
}

func (r *MongoMagicLinkRepository) Delete(ctx context.Context, userID string) error {
	if _, err := r.coll.DeleteOne(ctx, bson.D{{Key: "_id", Value: userID}}); err != nil {
		return fmt.Errorf("repository: brisanje tokena magicnog linka: %w", err)
	}
	return nil
}
