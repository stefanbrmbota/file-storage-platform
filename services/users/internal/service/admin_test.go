package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

type fakeUserLister struct {
	users []model.User
	err   error
	limit int
	calls int
}

func (f *fakeUserLister) List(_ context.Context, limit int) ([]model.User, error) {
	f.calls++
	f.limit = limit
	return f.users, f.err
}

type fakeRequestLister struct {
	list  []model.QuotaRequestDetails
	err   error
	limit int
	calls int
}

func (f *fakeRequestLister) ListWithRequester(_ context.Context, limit int) ([]model.QuotaRequestDetails, error) {
	f.calls++
	f.limit = limit
	return f.list, f.err
}

type adminEnv struct {
	svc      *AdminService
	accounts *fakeQuotaUsers
	users    *fakeUserLister
	requests *fakeRequestLister
}

func newAdminEnv() *adminEnv {
	accounts := &fakeQuotaUsers{users: map[string]model.User{
		"admin":    {ID: "admin", Active: true, Role: model.RoleAdmin},
		"user":     {ID: "user", Active: true, Role: model.RoleUser},
		"inactive": {ID: "inactive", Active: false, Role: model.RoleAdmin},
	}}
	users := &fakeUserLister{}
	requests := &fakeRequestLister{}
	return &adminEnv{svc: NewAdminService(accounts, users, requests), accounts: accounts, users: users, requests: requests}
}

func TestAdminList_OnlyActiveAdmin(t *testing.T) {
	e := newAdminEnv()
	ctx := context.Background()

	for _, id := range []string{"user", "inactive", "nepoznat", "", "  "} {
		if _, err := e.svc.ListUsers(ctx, id); !errors.Is(err, ErrForbidden) {
			t.Errorf("ListUsers(%q): err = %v, ocekivano ErrForbidden", id, err)
		}
		if _, err := e.svc.ListQuotaRequests(ctx, id); !errors.Is(err, ErrForbidden) {
			t.Errorf("ListQuotaRequests(%q): err = %v, ocekivano ErrForbidden", id, err)
		}
	}
	if e.users.calls != 0 || e.requests.calls != 0 {
		t.Error("bez prava administratora se podaci ne citaju")
	}
}

func TestAdminList_RoleCheckedInDatabase(t *testing.T) {
	e := newAdminEnv()
	// Nalog je degradiran posle izdavanja tokena: stara uloga iz tokena ne vredi.
	e.accounts.users["admin"] = model.User{ID: "admin", Active: true, Role: model.RoleUser}
	if _, err := e.svc.ListUsers(context.Background(), "admin"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, ocekivano ErrForbidden", err)
	}
}

func TestAdminList_RoleCheckError(t *testing.T) {
	e := newAdminEnv()
	boom := errors.New("baza pala")
	e.accounts.findErr = boom
	if _, err := e.svc.ListUsers(context.Background(), "admin"); !errors.Is(err, boom) || errors.Is(err, ErrForbidden) {
		t.Fatalf("kvar baze nije isto sto i zabrana: err = %v", err)
	}
}

func TestAdminListUsers_MapsFieldsWithoutSecrets(t *testing.T) {
	e := newAdminEnv()
	e.users.users = []model.User{
		{ID: "u1", FirstName: "Ana", LastName: "Anić", Email: "ana@example.com", PasswordHash: "tajni-hes", Role: model.RoleUser, Active: true, QuotaBytes: 1 << 30, UsedBytes: 500},
		{ID: "u2", FirstName: "Admin", LastName: "Adminić", Email: "admin@example.com", Role: model.RoleAdmin, Active: false, QuotaBytes: 2 << 30},
	}

	got, err := e.svc.ListUsers(context.Background(), "admin")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("korisnika: %d", len(got))
	}
	a := got[0]
	if a.ID != "u1" || a.FirstName != "Ana" || a.LastName != "Anić" || a.Email != "ana@example.com" || a.Role != model.RoleUser || !a.Active || a.QuotaBytes != 1<<30 || a.UsedBytes != 500 {
		t.Errorf("korisnik: %+v", a)
	}
	if got[1].Active || got[1].Role != model.RoleAdmin {
		t.Errorf("drugi korisnik: %+v", got[1])
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "tajni-hes") || strings.Contains(strings.ToLower(string(raw)), "password") {
		t.Errorf("JSON sadrzi lozinku: %s", raw)
	}
	if e.users.limit != repository.MaxListLimit {
		t.Errorf("limit = %d", e.users.limit)
	}
}

func TestAdminListUsers_EmptyIsArrayNotNull(t *testing.T) {
	e := newAdminEnv()
	got, err := e.svc.ListUsers(context.Background(), "admin")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(got)
	if string(raw) != "[]" {
		t.Errorf("prazna lista = %s, ocekivano []", raw)
	}
}

func TestAdminListQuotaRequests_MapsRequesterAndQuota(t *testing.T) {
	e := newAdminEnv()
	at := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	decided := at.Add(time.Hour)
	e.requests.list = []model.QuotaRequestDetails{
		{
			QuotaRequest: model.QuotaRequest{ID: "r1", UserID: "u1", RequestedBytes: 5 << 30, Status: model.QuotaRequestPending, CreatedAt: at},
			Requester:    &model.QuotaRequester{FirstName: "Ana", LastName: "Anić", Email: "ana@example.com", QuotaBytes: 1 << 30},
		},
		{
			QuotaRequest: model.QuotaRequest{ID: "r2", UserID: "u2", RequestedBytes: 3 << 30, Status: model.QuotaRequestApproved, CreatedAt: at, DecidedAt: &decided},
			Requester:    nil, // nalog obrisan
		},
	}

	got, err := e.svc.ListQuotaRequests(context.Background(), "admin")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("zahteva: %d", len(got))
	}
	r := got[0]
	if r.ID != "r1" || r.UserID != "u1" || r.FirstName != "Ana" || r.LastName != "Anić" || r.Email != "ana@example.com" ||
		r.CurrentQuotaBytes != 1<<30 || r.RequestedBytes != 5<<30 || r.Status != model.QuotaRequestPending || !r.CreatedAt.Equal(at) || r.DecidedAt != nil {
		t.Errorf("zahtev: %+v", r)
	}
	d := got[1]
	if d.ID != "r2" || d.Email != "" || d.CurrentQuotaBytes != 0 || d.Status != model.QuotaRequestApproved || d.DecidedAt == nil || !d.DecidedAt.Equal(decided) {
		t.Errorf("zahtev obrisanog podnosioca: %+v", d)
	}
	if e.requests.limit != repository.MaxListLimit {
		t.Errorf("limit = %d", e.requests.limit)
	}
}

func TestAdminList_RepositoryErrors(t *testing.T) {
	boom := errors.New("baza pala")
	t.Run("korisnici", func(t *testing.T) {
		e := newAdminEnv()
		e.users.err = boom
		if _, err := e.svc.ListUsers(context.Background(), "admin"); !errors.Is(err, boom) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("zahtevi", func(t *testing.T) {
		e := newAdminEnv()
		e.requests.err = boom
		if _, err := e.svc.ListQuotaRequests(context.Background(), "admin"); !errors.Is(err, boom) {
			t.Errorf("err = %v", err)
		}
	})
}
