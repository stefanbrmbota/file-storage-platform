package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

var quotaReqNow = time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)

const gib int64 = 1 << 30

type fakeQuotaRequests struct {
	byUser map[string][]model.QuotaRequest

	createErr, latestErr error
	creates              int
}

func newFakeQuotaRequests() *fakeQuotaRequests {
	return &fakeQuotaRequests{byUser: map[string][]model.QuotaRequest{}}
}

func (f *fakeQuotaRequests) Create(_ context.Context, r model.QuotaRequest) error {
	if f.createErr != nil {
		return f.createErr
	}
	for _, e := range f.byUser[r.UserID] {
		if e.Status == model.QuotaRequestPending {
			return repository.ErrQuotaRequestPending
		}
	}
	f.creates++
	f.byUser[r.UserID] = append(f.byUser[r.UserID], r)
	return nil
}

func (f *fakeQuotaRequests) Latest(_ context.Context, userID string) (model.QuotaRequest, error) {
	if f.latestErr != nil {
		return model.QuotaRequest{}, f.latestErr
	}
	list := f.byUser[userID]
	if len(list) == 0 {
		return model.QuotaRequest{}, repository.ErrQuotaRequestNotFound
	}
	return list[len(list)-1], nil
}

type fakeQuotaUsers struct {
	users   map[string]model.User // po ID-ju
	findErr error
}

func (f *fakeQuotaUsers) FindByID(_ context.Context, id string) (model.User, error) {
	if f.findErr != nil {
		return model.User{}, f.findErr
	}
	u, ok := f.users[id]
	if !ok {
		return model.User{}, repository.ErrNotFound
	}
	return u, nil
}

type quotaReqEnv struct {
	svc   *QuotaRequestService
	users *fakeQuotaUsers
	repo  *fakeQuotaRequests
}

func newQuotaReqEnv() *quotaReqEnv {
	users := &fakeQuotaUsers{users: map[string]model.User{
		"u1": {ID: "u1", Active: true, Role: model.RoleUser, QuotaBytes: gib},
	}}
	repo := newFakeQuotaRequests()
	svc := NewQuotaRequestService(users, repo)
	svc.now = func() time.Time { return quotaReqNow }
	return &quotaReqEnv{svc: svc, users: users, repo: repo}
}

func TestQuotaRequestSubmit_CreatesPending(t *testing.T) {
	e := newQuotaReqEnv()

	req, err := e.svc.Submit(context.Background(), "u1", 5*gib)
	if err != nil {
		t.Fatal(err)
	}
	if req.ID == "" || req.UserID != "u1" || req.RequestedBytes != 5*gib || req.Status != model.QuotaRequestPending || !req.CreatedAt.Equal(quotaReqNow) {
		t.Errorf("zahtev: %+v", req)
	}
	if req.DecidedAt != nil {
		t.Error("DecidedAt mora biti nil")
	}
	if e.repo.creates != 1 {
		t.Errorf("upisa: %d", e.repo.creates)
	}
}

func TestQuotaRequestSubmit_ValueMustExceedCurrentQuota(t *testing.T) {
	e := newQuotaReqEnv()
	ctx := context.Background()

	// Tacno trenutna kvota i manja vrednost nisu povecanje.
	for _, v := range []int64{gib, gib - 1, 1} {
		if _, err := e.svc.Submit(ctx, "u1", v); !errors.Is(err, ErrQuotaNotIncreased) {
			t.Errorf("%d: err = %v, ocekivano ErrQuotaNotIncreased", v, err)
		}
	}
	// Za jedan bajt vise je dovoljno.
	if _, err := e.svc.Submit(ctx, "u1", gib+1); err != nil {
		t.Fatalf("gib+1: %v", err)
	}
	if e.repo.creates != 1 {
		t.Errorf("odbijeni zahtevi se ne upisuju: upisa=%d", e.repo.creates)
	}
}

func TestQuotaRequestSubmit_InvalidValue(t *testing.T) {
	e := newQuotaReqEnv()
	for _, v := range []int64{0, -1, -gib, MaxQuotaRequestBytes + 1} {
		if _, err := e.svc.Submit(context.Background(), "u1", v); !errors.Is(err, ErrInvalidQuotaRequest) {
			t.Errorf("%d: err = %v, ocekivano ErrInvalidQuotaRequest", v, err)
		}
	}
	if _, err := e.svc.Submit(context.Background(), "u1", MaxQuotaRequestBytes); err != nil {
		t.Errorf("gornja granica je dozvoljena: %v", err)
	}
}

func TestQuotaRequestSubmit_SecondPendingRejected(t *testing.T) {
	e := newQuotaReqEnv()
	ctx := context.Background()

	if _, err := e.svc.Submit(ctx, "u1", 2*gib); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Submit(ctx, "u1", 3*gib); !errors.Is(err, ErrQuotaRequestPending) {
		t.Fatalf("err = %v, ocekivano ErrQuotaRequestPending", err)
	}
	if e.repo.creates != 1 {
		t.Errorf("upisa: %d, ocekivano 1", e.repo.creates)
	}
}

func TestQuotaRequestSubmit_NewRequestAfterDecision(t *testing.T) {
	e := newQuotaReqEnv()
	ctx := context.Background()

	first, _ := e.svc.Submit(ctx, "u1", 2*gib)
	// Administrator je odbio prvi zahtev (US-23).
	l := e.repo.byUser["u1"]
	l[0].Status = model.QuotaRequestRejected
	e.repo.byUser["u1"] = l

	second, err := e.svc.Submit(ctx, "u1", 3*gib)
	if err != nil {
		t.Fatalf("novi zahtev posle odluke: %v", err)
	}
	if second.ID == first.ID {
		t.Error("svaki zahtev ima svoj ID")
	}
}

func TestQuotaRequestSubmit_UnknownOrInactiveUser(t *testing.T) {
	e := newQuotaReqEnv()
	e.users.users["u2"] = model.User{ID: "u2", Active: false, QuotaBytes: gib}

	for _, id := range []string{"", "  ", "nepoznat", "u2"} {
		if _, err := e.svc.Submit(context.Background(), id, 5*gib); !errors.Is(err, ErrUserNotFound) {
			t.Errorf("%q: err = %v, ocekivano ErrUserNotFound", id, err)
		}
	}
	if e.repo.creates != 0 {
		t.Error("za nepoznat ili neaktivan nalog nema upisa")
	}
}

func TestQuotaRequestSubmit_RepositoryErrors(t *testing.T) {
	boom := errors.New("baza pala")
	t.Run("citanje naloga", func(t *testing.T) {
		e := newQuotaReqEnv()
		e.users.findErr = boom
		if _, err := e.svc.Submit(context.Background(), "u1", 5*gib); !errors.Is(err, boom) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("upis zahteva", func(t *testing.T) {
		e := newQuotaReqEnv()
		e.repo.createErr = boom
		if _, err := e.svc.Submit(context.Background(), "u1", 5*gib); !errors.Is(err, boom) {
			t.Errorf("err = %v", err)
		}
	})
}

func TestQuotaRequestLatest(t *testing.T) {
	e := newQuotaReqEnv()
	ctx := context.Background()

	if _, err := e.svc.Latest(ctx, "u1"); !errors.Is(err, ErrNoQuotaRequest) {
		t.Fatalf("bez zahteva: err = %v, ocekivano ErrNoQuotaRequest", err)
	}
	sent, _ := e.svc.Submit(ctx, "u1", 5*gib)
	got, err := e.svc.Latest(ctx, "u1")
	if err != nil || got.ID != sent.ID || got.Status != model.QuotaRequestPending {
		t.Fatalf("got %+v, err = %v", got, err)
	}
}

func TestQuotaRequestLatest_OnlyOwnRequest(t *testing.T) {
	e := newQuotaReqEnv()
	e.users.users["u2"] = model.User{ID: "u2", Active: true, QuotaBytes: gib}
	_, _ = e.svc.Submit(context.Background(), "u2", 5*gib)

	if _, err := e.svc.Latest(context.Background(), "u1"); !errors.Is(err, ErrNoQuotaRequest) {
		t.Fatalf("korisnik ne vidi tudj zahtev: err = %v", err)
	}
}

func TestQuotaRequestLatest_ErrorsAndEmptyID(t *testing.T) {
	e := newQuotaReqEnv()
	if _, err := e.svc.Latest(context.Background(), " "); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("prazan ID: err = %v", err)
	}
	boom := errors.New("baza pala")
	e.repo.latestErr = boom
	if _, err := e.svc.Latest(context.Background(), "u1"); !errors.Is(err, boom) || errors.Is(err, ErrNoQuotaRequest) {
		t.Errorf("kvar baze nije isto sto i nema zahteva: err = %v", err)
	}
}
