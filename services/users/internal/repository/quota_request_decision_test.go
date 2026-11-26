package repository

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
)

func TestQuotaRequestDecide_ApproveAndReject(t *testing.T) {
	_, reqs := newAdminListRepos(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	_ = reqs.Create(ctx, quotaReq("r1", "u1", 5<<30, model.QuotaRequestPending, now))
	_ = reqs.Create(ctx, quotaReq("r2", "u2", 6<<30, model.QuotaRequestPending, now))
	at := now.Add(time.Minute)

	got, err := reqs.Decide(ctx, "r1", model.QuotaRequestApproved, at)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "r1" || got.Status != model.QuotaRequestApproved || got.RequestedBytes != 5<<30 || got.DecidedAt == nil || !got.DecidedAt.Equal(at) {
		t.Errorf("odobren: %+v", got)
	}
	got, err = reqs.Decide(ctx, "r2", model.QuotaRequestRejected, at)
	if err != nil || got.Status != model.QuotaRequestRejected {
		t.Fatalf("odbijen: %+v, err = %v", got, err)
	}

	latest, _ := reqs.Latest(ctx, "u1")
	if latest.Status != model.QuotaRequestApproved || latest.DecidedAt == nil {
		t.Errorf("odluka mora biti sacuvana: %+v", latest)
	}
}

func TestQuotaRequestDecide_OnlyOnce(t *testing.T) {
	_, reqs := newAdminListRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()
	_ = reqs.Create(ctx, quotaReq("r1", "u1", 5<<30, model.QuotaRequestPending, now))

	if _, err := reqs.Decide(ctx, "r1", model.QuotaRequestApproved, now); err != nil {
		t.Fatal(err)
	}
	for _, s := range []model.QuotaRequestStatus{model.QuotaRequestApproved, model.QuotaRequestRejected} {
		if _, err := reqs.Decide(ctx, "r1", s, now); !errors.Is(err, ErrQuotaRequestDecided) {
			t.Errorf("%s: err = %v, ocekivano ErrQuotaRequestDecided", s, err)
		}
	}
	latest, _ := reqs.Latest(ctx, "u1")
	if latest.Status != model.QuotaRequestApproved {
		t.Errorf("prva odluka mora ostati: %+v", latest)
	}
}

func TestQuotaRequestDecide_NotFoundAndInvalidStatus(t *testing.T) {
	_, reqs := newAdminListRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if _, err := reqs.Decide(ctx, "nema", model.QuotaRequestApproved, now); !errors.Is(err, ErrQuotaRequestNotFound) {
		t.Errorf("err = %v, ocekivano ErrQuotaRequestNotFound", err)
	}
	_ = reqs.Create(ctx, quotaReq("r1", "u1", 5<<30, model.QuotaRequestPending, now))
	for _, s := range []model.QuotaRequestStatus{model.QuotaRequestPending, "BILO_STA", ""} {
		if _, err := reqs.Decide(ctx, "r1", s, now); err == nil || errors.Is(err, ErrQuotaRequestDecided) {
			t.Errorf("status %q: ocekivana greska o neispravnom statusu, dobijeno %v", s, err)
		}
	}
	if latest, _ := reqs.Latest(ctx, "u1"); latest.Status != model.QuotaRequestPending {
		t.Errorf("neispravan status ne sme menjati zahtev: %+v", latest)
	}
}

func TestQuotaRequestDecide_ConcurrentOnlyOneWins(t *testing.T) {
	_, reqs := newAdminListRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()
	_ = reqs.Create(ctx, quotaReq("r1", "u1", 5<<30, model.QuotaRequestPending, now))

	var wg sync.WaitGroup
	var mu sync.Mutex
	wins, decided := 0, 0
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			status := model.QuotaRequestApproved
			if i%2 == 1 {
				status = model.QuotaRequestRejected
			}
			_, err := reqs.Decide(ctx, "r1", status, now)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				wins++
			case errors.Is(err, ErrQuotaRequestDecided):
				decided++
			}
		}(i)
	}
	wg.Wait()
	if wins != 1 || decided != 9 {
		t.Fatalf("uspesnih: %d, vec odlucenih: %d; ocekivano 1 i 9", wins, decided)
	}
}

func TestQuotaRequestReopen(t *testing.T) {
	_, reqs := newAdminListRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()
	_ = reqs.Create(ctx, quotaReq("r1", "u1", 5<<30, model.QuotaRequestPending, now))
	_, _ = reqs.Decide(ctx, "r1", model.QuotaRequestApproved, now)

	if err := reqs.Reopen(ctx, "r1"); err != nil {
		t.Fatal(err)
	}
	got, _ := reqs.Latest(ctx, "u1")
	if got.Status != model.QuotaRequestPending || got.DecidedAt != nil {
		t.Fatalf("zahtev mora biti ponovo na cekanju bez vremena odluke: %+v", got)
	}
	// Moze se ponovo odluciti.
	if _, err := reqs.Decide(ctx, "r1", model.QuotaRequestRejected, now); err != nil {
		t.Errorf("ponovna odluka: %v", err)
	}
}

func TestQuotaRequestReopen_OnlyApproved(t *testing.T) {
	_, reqs := newAdminListRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()
	_ = reqs.Create(ctx, quotaReq("r1", "u1", 5<<30, model.QuotaRequestPending, now))

	for name, prepare := range map[string]func(){
		"na cekanju": func() {},
		"odbijen":    func() { _, _ = reqs.Decide(ctx, "r1", model.QuotaRequestRejected, now) },
	} {
		prepare()
		if err := reqs.Reopen(ctx, "r1"); !errors.Is(err, ErrQuotaRequestNotFound) {
			t.Errorf("%s: err = %v, ocekivano ErrQuotaRequestNotFound", name, err)
		}
	}
	if err := reqs.Reopen(ctx, "nema"); !errors.Is(err, ErrQuotaRequestNotFound) {
		t.Errorf("nepostojeci: err = %v", err)
	}
}

func TestQuotaRequestReopen_NewPendingBlocks(t *testing.T) {
	_, reqs := newAdminListRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()
	_ = reqs.Create(ctx, quotaReq("r1", "u1", 5<<30, model.QuotaRequestPending, now))
	_, _ = reqs.Decide(ctx, "r1", model.QuotaRequestApproved, now)
	// Korisnik je u medjuvremenu podneo novi zahtev.
	if err := reqs.Create(ctx, quotaReq("r2", "u1", 6<<30, model.QuotaRequestPending, now.Add(time.Second))); err != nil {
		t.Fatal(err)
	}

	if err := reqs.Reopen(ctx, "r1"); !errors.Is(err, ErrQuotaRequestPending) {
		t.Fatalf("err = %v, ocekivano ErrQuotaRequestPending", err)
	}
	latest, _ := reqs.Latest(ctx, "u1")
	if latest.ID != "r2" || latest.Status != model.QuotaRequestPending {
		t.Errorf("novi zahtev mora ostati netaknut: %+v", latest)
	}
}

func TestUserSetQuota(t *testing.T) {
	users, _ := newAdminListRepos(t)
	ctx := context.Background()
	u := listUser("u1", time.Now().UTC()) // kvota 1 GiB, zauzece 123
	_ = users.Create(ctx, u)

	prev, err := users.SetQuota(ctx, "u1", 5<<30)
	if err != nil {
		t.Fatal(err)
	}
	if prev != 1<<30 {
		t.Errorf("prethodna kvota = %d", prev)
	}
	got, _ := users.FindByID(ctx, "u1")
	if got.QuotaBytes != 5<<30 || got.UsedBytes != 123 || got.Email != u.Email || got.PasswordHash != u.PasswordHash {
		t.Errorf("menja se samo kvota: %+v", got)
	}
}

func TestUserSetQuota_Errors(t *testing.T) {
	users, _ := newAdminListRepos(t)
	ctx := context.Background()
	_ = users.Create(ctx, listUser("u1", time.Now().UTC()))

	if _, err := users.SetQuota(ctx, "nema", 5<<30); !errors.Is(err, ErrNotFound) {
		t.Errorf("nepostojeci: err = %v, ocekivano ErrNotFound", err)
	}
	for _, v := range []int64{0, -1} {
		if _, err := users.SetQuota(ctx, "u1", v); err == nil {
			t.Errorf("%d: ocekivana greska", v)
		}
	}
	if got, _ := users.FindByID(ctx, "u1"); got.QuotaBytes != 1<<30 {
		t.Errorf("neispravna vrednost ne sme menjati kvotu: %d", got.QuotaBytes)
	}
}
