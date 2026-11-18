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

// ErrOTPNotFound se vraća kada korisnik nema aktivan OTP.
var ErrOTPNotFound = errors.New("repository: otp nije pronadjen")

// OTPRepository je pristup jednokratnim kodovima.
type OTPRepository interface {
	// Save upisuje OTP korisnika; postojeći se zamenjuje (i brojač pokušaja se resetuje).
	Save(ctx context.Context, o model.OTP) error
	Get(ctx context.Context, userID string) (model.OTP, error)
	// IncrementAttempts atomično povećava brojač pokušaja i vraća zapis posle
	// povećanja. ErrOTPNotFound ako korisnik nema OTP.
	IncrementAttempts(ctx context.Context, userID string) (model.OTP, error)
	// Delete briše OTP. ErrOTPNotFound ako ga nema; tako samo jedan od dva
	// istovremena zahteva može da potroši isti OTP.
	Delete(ctx context.Context, userID string) error
}

// MongoOTPRepository je MongoDB implementacija OTPRepository.
type MongoOTPRepository struct {
	coll *mongo.Collection
}

// NewOTPRepository vraća repozitorijum i obezbeđuje TTL indeks koji briše
// istekle OTP-ove. Mongo TTL monitor radi otprilike jednom u minutu, pa se
// provera isteka pri verifikaciji ne sme oslanjati samo na brisanje.
func NewOTPRepository(ctx context.Context, db *mongo.Database) (*MongoOTPRepository, error) {
	coll := db.Collection("otps")
	_, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "expiresAt", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(0).SetName("ttl_expiresAt"),
	})
	if err != nil {
		return nil, fmt.Errorf("repository: kreiranje TTL indeksa za otp: %w", err)
	}
	return &MongoOTPRepository{coll: coll}, nil
}

func (r *MongoOTPRepository) Save(ctx context.Context, o model.OTP) error {
	_, err := r.coll.ReplaceOne(ctx, bson.D{{Key: "_id", Value: o.UserID}}, o, options.Replace().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("repository: upis otp-a: %w", err)
	}
	return nil
}

func (r *MongoOTPRepository) Get(ctx context.Context, userID string) (model.OTP, error) {
	var o model.OTP
	err := r.coll.FindOne(ctx, bson.D{{Key: "_id", Value: userID}}).Decode(&o)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.OTP{}, ErrOTPNotFound
	}
	if err != nil {
		return model.OTP{}, fmt.Errorf("repository: citanje otp-a: %w", err)
	}
	return o, nil
}

func (r *MongoOTPRepository) IncrementAttempts(ctx context.Context, userID string) (model.OTP, error) {
	var o model.OTP
	err := r.coll.FindOneAndUpdate(ctx,
		bson.D{{Key: "_id", Value: userID}},
		bson.D{{Key: "$inc", Value: bson.D{{Key: "attempts", Value: 1}}}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&o)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.OTP{}, ErrOTPNotFound
	}
	if err != nil {
		return model.OTP{}, fmt.Errorf("repository: povecanje pokusaja otp-a: %w", err)
	}
	return o, nil
}

func (r *MongoOTPRepository) Delete(ctx context.Context, userID string) error {
	res, err := r.coll.DeleteOne(ctx, bson.D{{Key: "_id", Value: userID}})
	if err != nil {
		return fmt.Errorf("repository: brisanje otp-a: %w", err)
	}
	if res.DeletedCount == 0 {
		return ErrOTPNotFound
	}
	return nil
}
