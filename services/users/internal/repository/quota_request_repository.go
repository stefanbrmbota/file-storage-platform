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
	// ErrQuotaRequestPending se vraća kada korisnik već ima zahtev na čekanju.
	ErrQuotaRequestPending = errors.New("repository: zahtev za kvotu je vec na cekanju")
	// ErrQuotaRequestNotFound se vraća kada korisnik nema nijedan zahtev.
	ErrQuotaRequestNotFound = errors.New("repository: zahtev za kvotu nije pronadjen")
)

// QuotaRequestRepository je pristup zahtevima za povećanje kvote (US-21).
type QuotaRequestRepository interface {
	// Create upisuje novi zahtev. ErrQuotaRequestPending ako korisnik već ima
	// zahtev na čekanju (garantuje ga indeks u bazi, pa i istovremeni zahtevi
	// ne mogu stvoriti dva).
	Create(ctx context.Context, r model.QuotaRequest) error
	// Latest vraća najnoviji zahtev korisnika. ErrQuotaRequestNotFound ako ga nema.
	Latest(ctx context.Context, userID string) (model.QuotaRequest, error)
}

// MongoQuotaRequestRepository je MongoDB implementacija QuotaRequestRepository.
type MongoQuotaRequestRepository struct {
	coll *mongo.Collection
}

// NewQuotaRequestRepository vraća repozitorijum i obezbeđuje indekse: parcijalni
// jedinstveni indeks (najviše jedan zahtev na čekanju po korisniku) i indeks za
// nalaženje najnovijeg zahteva korisnika.
func NewQuotaRequestRepository(ctx context.Context, db *mongo.Database) (*MongoQuotaRequestRepository, error) {
	coll := db.Collection("quota_requests")
	_, err := coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "userId", Value: 1}},
			Options: options.Index().
				SetUnique(true).
				SetPartialFilterExpression(bson.D{{Key: "status", Value: string(model.QuotaRequestPending)}}).
				SetName("uniq_pending_per_user"),
		},
		{
			Keys:    bson.D{{Key: "userId", Value: 1}, {Key: "createdAt", Value: -1}},
			Options: options.Index().SetName("user_createdAt"),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("repository: kreiranje indeksa za zahteve za kvotu: %w", err)
	}
	return &MongoQuotaRequestRepository{coll: coll}, nil
}

func (r *MongoQuotaRequestRepository) Create(ctx context.Context, q model.QuotaRequest) error {
	if _, err := r.coll.InsertOne(ctx, q); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrQuotaRequestPending
		}
		return fmt.Errorf("repository: upis zahteva za kvotu: %w", err)
	}
	return nil
}

func (r *MongoQuotaRequestRepository) Latest(ctx context.Context, userID string) (model.QuotaRequest, error) {
	var q model.QuotaRequest
	err := r.coll.FindOne(ctx,
		bson.D{{Key: "userId", Value: userID}},
		options.FindOne().SetSort(bson.D{{Key: "createdAt", Value: -1}}),
	).Decode(&q)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.QuotaRequest{}, ErrQuotaRequestNotFound
	}
	if err != nil {
		return model.QuotaRequest{}, fmt.Errorf("repository: citanje zahteva za kvotu: %w", err)
	}
	return q, nil
}
