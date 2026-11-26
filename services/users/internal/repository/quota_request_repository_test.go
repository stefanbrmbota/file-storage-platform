package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
)

func newTestQuotaRequestRepo(t *testing.T) (*MongoQuotaRequestRepository, *mongo.Database) {
	t.Helper()
	uri := os.Getenv("USERS_MONGO_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri).SetServerSelectionTimeout(2 * time.Second))
	if err != nil {
		t.Skipf("MongoDB nije dostupan: %v", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		t.Skipf("MongoDB nije dostupan: %v", err)
	}
	db := client.Database(fmt.Sprintf("users_test_%d", time.Now().UnixNano()))
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	repo, err := NewQuotaRequestRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	return repo, db
}

func quotaReq(id, userID string, bytes int64, status model.QuotaRequestStatus, at time.Time) model.QuotaRequest {
	return model.QuotaRequest{ID: id, UserID: userID, RequestedBytes: bytes, Status: status, CreatedAt: at}
}

func TestQuotaRequestCreateAndLatest(t *testing.T) {
	repo, _ := newTestQuotaRequestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	if _, err := repo.Latest(ctx, "u1"); !errors.Is(err, ErrQuotaRequestNotFound) {
		t.Fatalf("pre upisa: err = %v, ocekivano ErrQuotaRequestNotFound", err)
	}
	want := quotaReq("r1", "u1", 5<<30, model.QuotaRequestPending, now)
	if err := repo.Create(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Latest(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "r1" || got.UserID != "u1" || got.RequestedBytes != 5<<30 || got.Status != model.QuotaRequestPending || !got.CreatedAt.Equal(now) {
		t.Errorf("got %+v, ocekivano %+v", got, want)
	}
	if got.DecidedAt != nil {
		t.Errorf("DecidedAt mora biti nil dok nema odluke: %v", got.DecidedAt)
	}
}

func TestQuotaRequestCreate_SecondPendingRejected(t *testing.T) {
	repo, _ := newTestQuotaRequestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := repo.Create(ctx, quotaReq("r1", "u1", 2<<30, model.QuotaRequestPending, now)); err != nil {
		t.Fatal(err)
	}
	err := repo.Create(ctx, quotaReq("r2", "u1", 3<<30, model.QuotaRequestPending, now.Add(time.Second)))
	if !errors.Is(err, ErrQuotaRequestPending) {
		t.Fatalf("err = %v, ocekivano ErrQuotaRequestPending", err)
	}
	got, _ := repo.Latest(ctx, "u1")
	if got.ID != "r1" {
		t.Errorf("prvi zahtev mora ostati: %+v", got)
	}
}

func TestQuotaRequestCreate_OtherUsersIndependent(t *testing.T) {
	repo, _ := newTestQuotaRequestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := repo.Create(ctx, quotaReq("r1", "u1", 2<<30, model.QuotaRequestPending, now)); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, quotaReq("r2", "u2", 2<<30, model.QuotaRequestPending, now)); err != nil {
		t.Fatalf("zahtev drugog korisnika mora proci: %v", err)
	}
}

func TestQuotaRequestCreate_AfterDecisionAllowed(t *testing.T) {
	repo, db := newTestQuotaRequestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	// Odlucen zahtev (odobren, odbijen) ne blokira novi; moze ih biti vise.
	_ = repo.Create(ctx, quotaReq("r1", "u1", 2<<30, model.QuotaRequestApproved, now.Add(-2*time.Hour)))
	_ = repo.Create(ctx, quotaReq("r2", "u1", 3<<30, model.QuotaRequestRejected, now.Add(-time.Hour)))
	if err := repo.Create(ctx, quotaReq("r3", "u1", 4<<30, model.QuotaRequestPending, now)); err != nil {
		t.Fatalf("novi zahtev posle odlucenih: %v", err)
	}

	// Kad se zahtev na cekanju odluci (rucno, odluku radi US-23), korisnik opet moze da trazi.
	if _, err := db.Collection("quota_requests").UpdateOne(ctx,
		bson.D{{Key: "_id", Value: "r3"}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "status", Value: string(model.QuotaRequestRejected)}}}}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, quotaReq("r4", "u1", 5<<30, model.QuotaRequestPending, now.Add(time.Minute))); err != nil {
		t.Fatalf("novi zahtev posle odluke: %v", err)
	}
}

func TestQuotaRequestLatest_ReturnsNewest(t *testing.T) {
	repo, _ := newTestQuotaRequestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_ = repo.Create(ctx, quotaReq("star", "u1", 2<<30, model.QuotaRequestApproved, now.Add(-time.Hour)))
	_ = repo.Create(ctx, quotaReq("nov", "u1", 3<<30, model.QuotaRequestPending, now))
	_ = repo.Create(ctx, quotaReq("tudj", "u2", 9<<30, model.QuotaRequestPending, now.Add(time.Hour)))

	got, err := repo.Latest(ctx, "u1")
	if err != nil || got.ID != "nov" {
		t.Fatalf("got %+v, err = %v, ocekivano najnoviji zahtev korisnika u1", got, err)
	}
}

func TestQuotaRequestCreate_ConcurrentOnlyOnePending(t *testing.T) {
	repo, _ := newTestQuotaRequestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	var wg sync.WaitGroup
	var mu sync.Mutex
	wins, pending := 0, 0
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := repo.Create(ctx, quotaReq(fmt.Sprintf("r%d", i), "u1", 2<<30, model.QuotaRequestPending, now))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				wins++
			case errors.Is(err, ErrQuotaRequestPending):
				pending++
			}
		}(i)
	}
	wg.Wait()
	if wins != 1 || pending != 9 {
		t.Fatalf("uspesnih: %d, odbijenih: %d; ocekivano 1 i 9", wins, pending)
	}
}

func TestQuotaRequests_Indexes(t *testing.T) {
	_, db := newTestQuotaRequestRepo(t)
	ctx := context.Background()

	cur, err := db.Collection("quota_requests").Indexes().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var idx []bson.M
	if err := cur.All(ctx, &idx); err != nil {
		t.Fatal(err)
	}
	var partialUnique, byUser bool
	for _, i := range idx {
		switch i["name"] {
		case "uniq_pending_per_user":
			partialUnique = i["unique"] == true && i["partialFilterExpression"] != nil
		case "user_createdAt":
			byUser = true
		}
	}
	if !partialUnique || !byUser {
		t.Fatalf("indeksi: partialUnique=%v byUser=%v; ocekivano oba", partialUnique, byUser)
	}
}
