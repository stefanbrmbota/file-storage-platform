package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

var decisionNow = time.Date(2026, 11, 26, 14, 0, 0, 0, time.UTC)

// fakeDecider je memorijski QuotaRequestDecider.
type fakeDecider struct {
	reqs map[string]model.QuotaRequest

	decideErr, reopenErr error
	reopens              int
}

func (f *fakeDecider) Decide(_ context.Context, id string, status model.QuotaRequestStatus, at time.Time) (model.QuotaRequest, error) {
	if f.decideErr != nil {
		return model.QuotaRequest{}, f.decideErr
	}
	r, ok := f.reqs[id]
	if !ok {
		return model.QuotaRequest{}, repository.ErrQuotaRequestNotFound
	}
	if r.Status != model.QuotaRequestPending {
		return model.QuotaRequest{}, repository.ErrQuotaRequestDecided
	}
	r.Status, r.DecidedAt = status, &at
	f.reqs[id] = r
	return r, nil
}

func (f *fakeDecider) Reopen(_ context.Context, id string) error {
	f.reopens++
	if f.reopenErr != nil {
		return f.reopenErr
	}
	r := f.reqs[id]
	r.Status, r.DecidedAt = model.QuotaRequestPending, nil
	f.reqs[id] = r
	return nil
}

// fakeDecisionUsers je memorijski QuotaDecisionUsers.
type fakeDecisionUsers struct {
	users map[string]model.User

	findErr, setErr error
	setCalls        int
}

func (f *fakeDecisionUsers) FindByID(_ context.Context, id string) (model.User, error) {
	if f.findErr != nil {
		return model.User{}, f.findErr
	}
	u, ok := f.users[id]
	if !ok {
		return model.User{}, repository.ErrNotFound
	}
	return u, nil
}

func (f *fakeDecisionUsers) SetQuota(_ context.Context, id string, quotaBytes int64) (int64, error) {
	f.setCalls++
	if f.setErr != nil {
		return 0, f.setErr
	}
	u, ok := f.users[id]
	if !ok {
		return 0, repository.ErrNotFound
	}
	prev := u.QuotaBytes
	u.QuotaBytes = quotaBytes
	f.users[id] = u
	return prev, nil
}

type decisionEnv struct {
	svc   *QuotaDecisionService
	users *fakeDecisionUsers
	reqs  *fakeDecider
	log   *bytes.Buffer
}

func newDecisionEnv(t *testing.T) *decisionEnv {
	t.Helper()
	users := &fakeDecisionUsers{users: map[string]model.User{
		"admin":    {ID: "admin", Active: true, Role: model.RoleAdmin},
		"user":     {ID: "user", Active: true, Role: model.RoleUser, QuotaBytes: gib, UsedBytes: 100},
		"inactive": {ID: "inactive", Active: false, Role: model.RoleAdmin},
	}}
	reqs := &fakeDecider{reqs: map[string]model.QuotaRequest{
		"r1": {ID: "r1", UserID: "user", RequestedBytes: 5 * gib, Status: model.QuotaRequestPending, CreatedAt: decisionNow.Add(-time.Hour)},
	}}
	svc := NewQuotaDecisionService(users, reqs)
	svc.now = func() time.Time { return decisionNow }

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &decisionEnv{svc: svc, users: users, reqs: reqs, log: &buf}
}

// adminLog vraca zapise "administratorska aktivnost" iz loga.
func (e *decisionEnv) adminLog(t *testing.T) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(e.log.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log nije JSON: %q", line)
		}
		if m["msg"] == "administratorska aktivnost" {
			out = append(out, m)
		}
	}
	return out
}

func TestQuotaDecisionApprove_SetsQuotaAndStatus(t *testing.T) {
	e := newDecisionEnv(t)

	req, err := e.svc.Approve(context.Background(), "admin", "r1")
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != model.QuotaRequestApproved || req.DecidedAt == nil || !req.DecidedAt.Equal(decisionNow) {
		t.Errorf("zahtev: %+v", req)
	}
	u := e.users.users["user"]
	if u.QuotaBytes != 5*gib {
		t.Errorf("kvota = %d, ocekivano %d", u.QuotaBytes, 5*gib)
	}
	if u.UsedBytes != 100 {
		t.Errorf("zauzece se ne menja: %d", u.UsedBytes)
	}
}

func TestQuotaDecisionApprove_LogsAdminActivity(t *testing.T) {
	e := newDecisionEnv(t)
	if _, err := e.svc.Approve(context.Background(), "admin", "r1"); err != nil {
		t.Fatal(err)
	}

	logs := e.adminLog(t)
	if len(logs) != 1 {
		t.Fatalf("zapisa: %d, ocekivano 1: %s", len(logs), e.log.String())
	}
	l := logs[0]
	if l["action"] != "quota_request_approved" || l["adminId"] != "admin" || l["requestId"] != "r1" || l["userId"] != "user" ||
		l["previousQuotaBytes"] != float64(gib) || l["newQuotaBytes"] != float64(5*gib) {
		t.Errorf("zapis: %v", l)
	}
}

func TestQuotaDecisionReject_KeepsQuotaAndLogs(t *testing.T) {
	e := newDecisionEnv(t)

	req, err := e.svc.Reject(context.Background(), "admin", "r1")
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != model.QuotaRequestRejected || req.DecidedAt == nil {
		t.Errorf("zahtev: %+v", req)
	}
	if e.users.users["user"].QuotaBytes != gib || e.users.setCalls != 0 {
		t.Errorf("kvota mora ostati ista: %d (poziva SetQuota: %d)", e.users.users["user"].QuotaBytes, e.users.setCalls)
	}
	logs := e.adminLog(t)
	if len(logs) != 1 || logs[0]["action"] != "quota_request_rejected" || logs[0]["adminId"] != "admin" || logs[0]["requestId"] != "r1" {
		t.Errorf("zapis: %v", logs)
	}
}

func TestQuotaDecision_OnlyActiveAdmin(t *testing.T) {
	e := newDecisionEnv(t)
	for _, id := range []string{"user", "inactive", "nepoznat", "", " "} {
		if _, err := e.svc.Approve(context.Background(), id, "r1"); !errors.Is(err, ErrForbidden) {
			t.Errorf("Approve(%q): err = %v, ocekivano ErrForbidden", id, err)
		}
		if _, err := e.svc.Reject(context.Background(), id, "r1"); !errors.Is(err, ErrForbidden) {
			t.Errorf("Reject(%q): err = %v, ocekivano ErrForbidden", id, err)
		}
	}
	if e.reqs.reqs["r1"].Status != model.QuotaRequestPending || e.users.setCalls != 0 {
		t.Error("bez prava administratora se nista ne menja")
	}
	if len(e.adminLog(t)) != 0 {
		t.Error("odbijen pokusaj nije administratorska aktivnost")
	}
}

func TestQuotaDecision_UnknownAndMalformedRequestID(t *testing.T) {
	e := newDecisionEnv(t)
	for _, id := range []string{"nema", "", "  ", strings.Repeat("a", maxQuotaRequestIDLen+1)} {
		if _, err := e.svc.Approve(context.Background(), "admin", id); !errors.Is(err, ErrQuotaRequestNotFound) {
			t.Errorf("Approve(%q): err = %v, ocekivano ErrQuotaRequestNotFound", id, err)
		}
		if _, err := e.svc.Reject(context.Background(), "admin", id); !errors.Is(err, ErrQuotaRequestNotFound) {
			t.Errorf("Reject(%q): err = %v, ocekivano ErrQuotaRequestNotFound", id, err)
		}
	}
	if e.users.setCalls != 0 {
		t.Error("nepostojeci zahtev ne menja kvotu")
	}
}

func TestQuotaDecision_AlreadyDecided(t *testing.T) {
	e := newDecisionEnv(t)
	ctx := context.Background()
	if _, err := e.svc.Approve(ctx, "admin", "r1"); err != nil {
		t.Fatal(err)
	}
	e.users.setCalls = 0

	if _, err := e.svc.Approve(ctx, "admin", "r1"); !errors.Is(err, ErrQuotaRequestDecided) {
		t.Errorf("drugo odobravanje: err = %v", err)
	}
	if _, err := e.svc.Reject(ctx, "admin", "r1"); !errors.Is(err, ErrQuotaRequestDecided) {
		t.Errorf("odbijanje odobrenog: err = %v", err)
	}
	if e.users.setCalls != 0 || e.users.users["user"].QuotaBytes != 5*gib {
		t.Errorf("vec odlucen zahtev ne dira kvotu: poziva=%d, kvota=%d", e.users.setCalls, e.users.users["user"].QuotaBytes)
	}
}

func TestQuotaDecisionApprove_QuotaWriteFailureReopensRequest(t *testing.T) {
	e := newDecisionEnv(t)
	boom := errors.New("baza pala")
	e.users.setErr = boom

	if _, err := e.svc.Approve(context.Background(), "admin", "r1"); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	r := e.reqs.reqs["r1"]
	if r.Status != model.QuotaRequestPending || r.DecidedAt != nil || e.reqs.reopens != 1 {
		t.Errorf("zahtev mora biti vracen na cekanje: %+v (reopen: %d)", r, e.reqs.reopens)
	}
	if len(e.adminLog(t)) != 0 {
		t.Error("neuspela odluka se ne loguje kao izvrsena")
	}

	// Posle popravke isti zahtev moze da se odobri.
	e.users.setErr = nil
	if _, err := e.svc.Approve(context.Background(), "admin", "r1"); err != nil {
		t.Fatalf("ponovni pokusaj: %v", err)
	}
	if e.users.users["user"].QuotaBytes != 5*gib {
		t.Error("kvota nije postavljena")
	}
}

func TestQuotaDecisionApprove_DeletedUserLeavesRequestPendingForReject(t *testing.T) {
	e := newDecisionEnv(t)
	delete(e.users.users, "user")

	if _, err := e.svc.Approve(context.Background(), "admin", "r1"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("err = %v, ocekivano ErrUserNotFound", err)
	}
	if e.reqs.reqs["r1"].Status != model.QuotaRequestPending {
		t.Errorf("zahtev mora ostati na cekanju: %+v", e.reqs.reqs["r1"])
	}
	// Takav zahtev se moze odbiti.
	if _, err := e.svc.Reject(context.Background(), "admin", "r1"); err != nil {
		t.Errorf("odbijanje zahteva obrisanog korisnika: %v", err)
	}
}

func TestQuotaDecisionApprove_ReopenFailureIsLogged(t *testing.T) {
	e := newDecisionEnv(t)
	e.users.setErr = errors.New("baza pala")
	e.reqs.reopenErr = errors.New("i ovo pada")

	if _, err := e.svc.Approve(context.Background(), "admin", "r1"); err == nil {
		t.Fatal("ocekivana greska")
	}
	if !strings.Contains(e.log.String(), "vracanje zahteva na cekanje nije uspelo") {
		t.Errorf("neuspeh kompenzacije mora biti u logu: %s", e.log.String())
	}
}

func TestQuotaDecision_InfrastructureErrors(t *testing.T) {
	boom := errors.New("baza pala")
	t.Run("provera uloge", func(t *testing.T) {
		e := newDecisionEnv(t)
		e.users.findErr = boom
		if _, err := e.svc.Approve(context.Background(), "admin", "r1"); !errors.Is(err, boom) || errors.Is(err, ErrForbidden) {
			t.Errorf("kvar baze nije isto sto i zabrana: err = %v", err)
		}
	})
	t.Run("odluka", func(t *testing.T) {
		e := newDecisionEnv(t)
		e.reqs.decideErr = boom
		for _, f := range []func() error{
			func() error { _, err := e.svc.Approve(context.Background(), "admin", "r1"); return err },
			func() error { _, err := e.svc.Reject(context.Background(), "admin", "r1"); return err },
		} {
			if err := f(); !errors.Is(err, boom) {
				t.Errorf("err = %v", err)
			}
		}
		if e.users.setCalls != 0 {
			t.Error("bez odluke nema upisa kvote")
		}
	})
}
