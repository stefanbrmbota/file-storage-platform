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

// ErrQuotaRequestDecided se vraća kada je zahtev već odlučen (nije na čekanju).
var ErrQuotaRequestDecided = errors.New("repository: zahtev za kvotu je vec odlucen")

// QuotaRequestDecider upravlja odlukom administratora o zahtevu (US-23).
type QuotaRequestDecider interface {
	// Decide atomično menja status zahteva na čekanju u APPROVED ili REJECTED i
	// upisuje vreme odluke; vraća zahtev posle izmene. Samo jedan od istovremenih
	// poziva uspeva. ErrQuotaRequestNotFound ako zahtev ne postoji,
	// ErrQuotaRequestDecided ako nije na čekanju.
	Decide(ctx context.Context, id string, status model.QuotaRequestStatus, at time.Time) (model.QuotaRequest, error)
	// Reopen vraća odobren zahtev na čekanje (kompenzacija kad kvota nije mogla da
	// se upiše). ErrQuotaRequestPending ako je korisnik u međuvremenu podneo novi
	// zahtev, ErrQuotaRequestNotFound ako zahteva nema ili nije odobren.
	Reopen(ctx context.Context, id string) error
}

func (r *MongoQuotaRequestRepository) Decide(ctx context.Context, id string, status model.QuotaRequestStatus, at time.Time) (model.QuotaRequest, error) {
	if status != model.QuotaRequestApproved && status != model.QuotaRequestRejected {
		return model.QuotaRequest{}, fmt.Errorf("repository: neispravan status odluke %q", status)
	}
	filter := bson.D{
		{Key: "_id", Value: id},
		{Key: "status", Value: string(model.QuotaRequestPending)},
	}
	update := bson.D{{Key: "$set", Value: bson.D{
		{Key: "status", Value: string(status)},
		{Key: "decidedAt", Value: at},
	}}}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)

	var q model.QuotaRequest
	err := r.coll.FindOneAndUpdate(ctx, filter, update, opts).Decode(&q)
	if err == nil {
		return q, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return model.QuotaRequest{}, fmt.Errorf("repository: odluka o zahtevu za kvotu: %w", err)
	}
	// Nista nije izmenjeno: ili zahtev ne postoji, ili je vec odlucen.
	n, cerr := r.coll.CountDocuments(ctx, bson.D{{Key: "_id", Value: id}})
	if cerr != nil {
		return model.QuotaRequest{}, fmt.Errorf("repository: provera zahteva za kvotu: %w", cerr)
	}
	if n == 0 {
		return model.QuotaRequest{}, ErrQuotaRequestNotFound
	}
	return model.QuotaRequest{}, ErrQuotaRequestDecided
}

func (r *MongoQuotaRequestRepository) Reopen(ctx context.Context, id string) error {
	filter := bson.D{
		{Key: "_id", Value: id},
		{Key: "status", Value: string(model.QuotaRequestApproved)},
	}
	update := bson.D{
		{Key: "$set", Value: bson.D{{Key: "status", Value: string(model.QuotaRequestPending)}}},
		{Key: "$unset", Value: bson.D{{Key: "decidedAt", Value: ""}}},
	}
	res, err := r.coll.UpdateOne(ctx, filter, update)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrQuotaRequestPending
		}
		return fmt.Errorf("repository: vracanje zahteva za kvotu na cekanje: %w", err)
	}
	if res.MatchedCount == 0 {
		return ErrQuotaRequestNotFound
	}
	return nil
}
