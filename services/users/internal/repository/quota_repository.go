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

// ErrQuotaExceeded se vraca kada rezervacija ne staje u kvotu korisnika.
var ErrQuotaExceeded = errors.New("repository: kvota prekoracena")

// QuotaRepository je pristup kvoti korisnika (S3, US-20).
type QuotaRepository interface {
	// ReserveQuota atomicno povecava usedBytes za bytes, ali samo ako vazi
	// usedBytes + bytes <= quotaBytes. Vraca preostali prostor posle rezervacije.
	// ErrNotFound ako korisnik ne postoji, ErrQuotaExceeded ako nema mesta
	// (usedBytes se tada ne menja).
	ReserveQuota(ctx context.Context, id string, bytes int64) (remaining int64, err error)
	// ReleaseQuota atomicno smanjuje usedBytes za bytes, nikad ispod 0.
	// Vraca usedBytes posle oslobadjanja. ErrNotFound ako korisnik ne postoji.
	ReleaseQuota(ctx context.Context, id string, bytes int64) (usedBytes int64, err error)
}

// ReserveQuota je jedan FindOneAndUpdate: uslov i povecanje se izvrsavaju
// atomicno u bazi, pa dve istovremene rezervacije ne mogu zajedno preci kvotu.
func (r *MongoUserRepository) ReserveQuota(ctx context.Context, id string, bytes int64) (int64, error) {
	filter := bson.D{
		{Key: "_id", Value: id},
		{Key: "$expr", Value: bson.D{{Key: "$lte", Value: bson.A{
			bson.D{{Key: "$add", Value: bson.A{"$usedBytes", bytes}}},
			"$quotaBytes",
		}}}},
	}
	update := bson.D{{Key: "$inc", Value: bson.D{{Key: "usedBytes", Value: bytes}}}}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)

	var u model.User
	err := r.coll.FindOneAndUpdate(ctx, filter, update, opts).Decode(&u)
	if err == nil {
		return u.QuotaBytes - u.UsedBytes, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return 0, fmt.Errorf("repository: rezervacija kvote: %w", err)
	}
	// Nista nije izmenjeno: ili korisnik ne postoji, ili nema mesta.
	if _, ferr := r.FindByID(ctx, id); ferr != nil {
		return 0, ferr
	}
	return 0, ErrQuotaExceeded
}

// ReleaseQuota je jedan FindOneAndUpdate sa pipeline update-om, pa se
// usedBytes = max(0, usedBytes - bytes) racuna atomicno u bazi.
func (r *MongoUserRepository) ReleaseQuota(ctx context.Context, id string, bytes int64) (int64, error) {
	update := mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "usedBytes", Value: bson.D{{Key: "$max", Value: bson.A{
		int64(0),
		bson.D{{Key: "$subtract", Value: bson.A{"$usedBytes", bytes}}},
	}}}}}}}}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)

	var u model.User
	err := r.coll.FindOneAndUpdate(ctx, bson.D{{Key: "_id", Value: id}}, update, opts).Decode(&u)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("repository: oslobadjanje kvote: %w", err)
	}
	return u.UsedBytes, nil
}
