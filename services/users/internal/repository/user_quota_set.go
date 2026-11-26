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

// QuotaSetter postavlja kvotu korisnika (US-23, odobren zahtev).
type QuotaSetter interface {
	// SetQuota postavlja quotaBytes korisnika na zadatu vrednost (pozitivnu) i vraća
	// prethodnu kvotu. ErrNotFound ako korisnik ne postoji. Zauzeće se ne menja.
	SetQuota(ctx context.Context, id string, quotaBytes int64) (previous int64, err error)
}

func (r *MongoUserRepository) SetQuota(ctx context.Context, id string, quotaBytes int64) (int64, error) {
	if quotaBytes <= 0 {
		return 0, fmt.Errorf("repository: kvota mora biti pozitivna, dobijeno %d", quotaBytes)
	}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "quotaBytes", Value: quotaBytes}}}}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.Before)

	var before model.User
	err := r.coll.FindOneAndUpdate(ctx, bson.D{{Key: "_id", Value: id}}, update, opts).Decode(&before)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("repository: postavljanje kvote: %w", err)
	}
	return before.QuotaBytes, nil
}
