package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ErrPasswordConflict se vraca kada heš lozinke u bazi nije onaj koji se
// ocekivao, tj. lozinka je u medjuvremenu vec promenjena (istovremena izmena).
var ErrPasswordConflict = errors.New("repository: lozinka je u medjuvremenu promenjena")

// PasswordRepository je pristup lozinki korisnika (US-14).
type PasswordRepository interface {
	// UpdatePassword postavlja novi heš lozinke i passwordChangedAt, ali samo
	// ako je heš u bazi jos uvek oldHash (compare-and-set u jednom UpdateOne),
	// pa dve istovremene izmene ne mogu pregaziti jedna drugu.
	// ErrNotFound ako korisnik ne postoji, ErrPasswordConflict ako heš nije oldHash.
	UpdatePassword(ctx context.Context, id, oldHash, newHash string, changedAt time.Time) error
}

func (r *MongoUserRepository) UpdatePassword(ctx context.Context, id, oldHash, newHash string, changedAt time.Time) error {
	res, err := r.coll.UpdateOne(ctx,
		bson.D{
			{Key: "_id", Value: id},
			{Key: "passwordHash", Value: oldHash},
		},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "passwordHash", Value: newHash},
			{Key: "passwordChangedAt", Value: changedAt},
		}}},
	)
	if err != nil {
		return fmt.Errorf("repository: izmena lozinke: %w", err)
	}
	if res.MatchedCount > 0 {
		return nil
	}
	// Nista nije izmenjeno: ili korisnik ne postoji, ili heš vise nije oldHash.
	if _, ferr := r.FindByID(ctx, id); ferr != nil {
		return ferr
	}
	return ErrPasswordConflict
}
