package repository

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
)

// newAdminListRepos pravi repozitorijume korisnika i zahteva nad istom probnom bazom.
func newAdminListRepos(t *testing.T) (*MongoUserRepository, *MongoQuotaRequestRepository) {
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

	users, err := NewUserRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	reqs, err := NewQuotaRequestRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	return users, reqs
}

func listUser(id string, created time.Time) model.User {
	u := model.NewUser(id, "Ime-"+id, "Prezime-"+id, id+"@example.com", "tajni-hes-"+id, 1<<30, created)
	u.Active = true
	u.UsedBytes = 123
	return u
}

func TestUserList_NewestFirstWithoutPasswordHash(t *testing.T) {
	users, _ := newAdminListRepos(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	for i, id := range []string{"star", "srednji", "nov"} {
		if err := users.Create(ctx, listUser(id, now.Add(time.Duration(i)*time.Hour))); err != nil {
			t.Fatal(err)
		}
	}
	got, err := users.List(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ID != "nov" || got[1].ID != "srednji" || got[2].ID != "star" {
		t.Fatalf("redosled: %+v", got)
	}
	u := got[0]
	if u.FirstName != "Ime-nov" || u.LastName != "Prezime-nov" || u.Email != "nov@example.com" || u.QuotaBytes != 1<<30 || u.UsedBytes != 123 {
		t.Errorf("polja: %+v", u)
	}
	for _, x := range got {
		if x.PasswordHash != "" {
			t.Errorf("heš lozinke se ne sme citati: %q", x.PasswordHash)
		}
	}
}

func TestUserList_Empty(t *testing.T) {
	users, _ := newAdminListRepos(t)
	got, err := users.List(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("prazna lista mora biti [], ne nil: %#v", got)
	}
}

func TestUserList_Limit(t *testing.T) {
	users, _ := newAdminListRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		_ = users.Create(ctx, listUser(fmt.Sprintf("u%d", i), now.Add(time.Duration(i)*time.Minute)))
	}

	got, _ := users.List(ctx, 2)
	if len(got) != 2 || got[0].ID != "u4" || got[1].ID != "u3" {
		t.Fatalf("limit 2: %+v", got)
	}
	// Nepozitivno ograničenje se svodi na najveće dozvoljeno, ne na 0 ni na "sve".
	for _, l := range []int{0, -1} {
		if got, _ := users.List(ctx, l); len(got) != 5 {
			t.Errorf("limit %d: %d stavki, ocekivano 5", l, len(got))
		}
	}
	if clampListLimit(MaxListLimit+1) != MaxListLimit || clampListLimit(3) != 3 {
		t.Error("clampListLimit")
	}
}

func TestQuotaRequestListWithRequester_JoinsRequesterWithoutSecrets(t *testing.T) {
	users, reqs := newAdminListRepos(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	_ = users.Create(ctx, listUser("u1", now))
	if err := reqs.Create(ctx, quotaReq("r1", "u1", 5<<30, model.QuotaRequestPending, now)); err != nil {
		t.Fatal(err)
	}

	got, err := reqs.ListWithRequester(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("zahteva: %d", len(got))
	}
	d := got[0]
	if d.ID != "r1" || d.UserID != "u1" || d.RequestedBytes != 5<<30 || d.Status != model.QuotaRequestPending || !d.CreatedAt.Equal(now) {
		t.Errorf("zahtev: %+v", d.QuotaRequest)
	}
	if d.Requester == nil {
		t.Fatal("podnosilac nedostaje")
	}
	if d.Requester.FirstName != "Ime-u1" || d.Requester.LastName != "Prezime-u1" || d.Requester.Email != "u1@example.com" || d.Requester.QuotaBytes != 1<<30 {
		t.Errorf("podnosilac: %+v", *d.Requester)
	}
}

func TestQuotaRequestListWithRequester_PendingFirstThenNewest(t *testing.T) {
	users, reqs := newAdminListRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_ = users.Create(ctx, listUser("a", now))
	_ = users.Create(ctx, listUser("b", now))
	_ = users.Create(ctx, listUser("c", now))
	// "a" ima odlucen stari zahtev, pa novi na cekanju; "b" odbijen noviji; "c" na cekanju najstariji.
	_ = reqs.Create(ctx, quotaReq("a-old", "a", 2<<30, model.QuotaRequestApproved, now.Add(-5*time.Hour)))
	_ = reqs.Create(ctx, quotaReq("b-rej", "b", 2<<30, model.QuotaRequestRejected, now.Add(-time.Hour)))
	_ = reqs.Create(ctx, quotaReq("c-pen", "c", 2<<30, model.QuotaRequestPending, now.Add(-4*time.Hour)))
	_ = reqs.Create(ctx, quotaReq("a-pen", "a", 3<<30, model.QuotaRequestPending, now.Add(-2*time.Hour)))

	got, err := reqs.ListWithRequester(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, d := range got {
		ids = append(ids, d.ID)
	}
	want := []string{"a-pen", "c-pen", "b-rej", "a-old"} // na cekanju prvo (novije pre starijeg), pa ostali
	if fmt.Sprint(ids) != fmt.Sprint(want) {
		t.Fatalf("redosled %v, ocekivano %v", ids, want)
	}
}

func TestQuotaRequestListWithRequester_DeletedRequesterKeepsRequest(t *testing.T) {
	users, reqs := newAdminListRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_ = users.Create(ctx, listUser("u1", now))
	_ = reqs.Create(ctx, quotaReq("r1", "u1", 5<<30, model.QuotaRequestPending, now))
	if err := users.Delete(ctx, "u1"); err != nil {
		t.Fatal(err)
	}

	got, err := reqs.ListWithRequester(ctx, 10)
	if err != nil || len(got) != 1 {
		t.Fatalf("got %+v, err = %v", got, err)
	}
	if got[0].Requester != nil {
		t.Errorf("podnosilac je obrisan, ocekivano nil: %+v", got[0].Requester)
	}
}

func TestQuotaRequestListWithRequester_EmptyAndLimit(t *testing.T) {
	users, reqs := newAdminListRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()

	got, err := reqs.ListWithRequester(ctx, 10)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("prazna lista mora biti []: %#v, err = %v", got, err)
	}

	for i := 0; i < 4; i++ {
		id := fmt.Sprintf("u%d", i)
		_ = users.Create(ctx, listUser(id, now))
		_ = reqs.Create(ctx, quotaReq("r"+id, id, 2<<30, model.QuotaRequestPending, now.Add(time.Duration(i)*time.Minute)))
	}
	if got, _ := reqs.ListWithRequester(ctx, 2); len(got) != 2 || got[0].ID != "ru3" {
		t.Fatalf("limit 2: %+v", got)
	}
}
