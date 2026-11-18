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
