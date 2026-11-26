package repository

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
)

// QuotaRequestLister vraća zahteve za kvotu sa podacima podnosilaca (US-22).
type QuotaRequestLister interface {
	// ListWithRequester vraća najviše limit zahteva: prvo oni na čekanju, pa ostali,
	// svaka grupa od najnovijeg. Uz zahtev idu ime, email i trenutna kvota podnosioca.
	ListWithRequester(ctx context.Context, limit int) ([]model.QuotaRequestDetails, error)
}

func (r *MongoQuotaRequestRepository) ListWithRequester(ctx context.Context, limit int) ([]model.QuotaRequestDetails, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$addFields", Value: bson.D{{Key: "pendingFirst", Value: bson.D{{Key: "$cond", Value: bson.A{
			bson.D{{Key: "$eq", Value: bson.A{"$status", string(model.QuotaRequestPending)}}}, 0, 1,
		}}}}}}},
		{{Key: "$sort", Value: bson.D{{Key: "pendingFirst", Value: 1}, {Key: "createdAt", Value: -1}, {Key: "_id", Value: 1}}}},
		{{Key: "$limit", Value: clampListLimit(limit)}},
		{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "users"},
			{Key: "localField", Value: "userId"},
			{Key: "foreignField", Value: "_id"},
			{Key: "as", Value: "user"},
			// Iz naloga se uzimaju samo polja koja admin sme da vidi (nikad heš lozinke).
			{Key: "pipeline", Value: bson.A{
				bson.D{{Key: "$project", Value: bson.D{
					{Key: "firstName", Value: 1}, {Key: "lastName", Value: 1},
					{Key: "email", Value: 1}, {Key: "quotaBytes", Value: 1},
				}}},
			}},
		}}},
		{{Key: "$unwind", Value: bson.D{{Key: "path", Value: "$user"}, {Key: "preserveNullAndEmptyArrays", Value: true}}}},
	}

	cur, err := r.coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("repository: lista zahteva za kvotu: %w", err)
	}
	list := []model.QuotaRequestDetails{}
	if err := cur.All(ctx, &list); err != nil {
		return nil, fmt.Errorf("repository: citanje liste zahteva za kvotu: %w", err)
	}
	return list, nil
}
