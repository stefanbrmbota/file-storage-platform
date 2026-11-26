package repository

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
)

// MaxListLimit je najveći broj stavki koje jedna admin lista vraća.
const MaxListLimit = 200

// clampListLimit vraća ispravno ograničenje: nepozitivno i preveliko se svodi na MaxListLimit.
func clampListLimit(limit int) int64 {
	if limit <= 0 || limit > MaxListLimit {
		return MaxListLimit
	}
	return int64(limit)
}

// UserLister vraća listu korisnika za administratora (US-22).
type UserLister interface {
	// List vraća najviše limit korisnika, od najnovijeg. Heš lozinke se ne čita iz
	// baze (projekcija), pa ne može procureti ni greškom u višim slojevima.
	List(ctx context.Context, limit int) ([]model.User, error)
}

func (r *MongoUserRepository) List(ctx context.Context, limit int) ([]model.User, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: 1}}).
		SetLimit(clampListLimit(limit)).
		SetProjection(bson.D{{Key: "passwordHash", Value: 0}})

	cur, err := r.coll.Find(ctx, bson.D{}, opts)
	if err != nil {
		return nil, fmt.Errorf("repository: lista korisnika: %w", err)
	}
	users := []model.User{}
	if err := cur.All(ctx, &users); err != nil {
		return nil, fmt.Errorf("repository: citanje liste korisnika: %w", err)
	}
	return users, nil
}
