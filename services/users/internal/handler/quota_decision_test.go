package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/service"
)

type fakeDecider struct {
	req     model.QuotaRequest
	err     error
	admin   string
	id      string
	approve int
	reject  int
}

func (f *fakeDecider) Approve(_ context.Context, adminID, requestID string) (model.QuotaRequest, error) {
	f.approve++
	f.admin, f.id = adminID, requestID
	r := f.req
	r.Status = model.QuotaRequestApproved
	return r, f.err
}

func (f *fakeDecider) Reject(_ context.Context, adminID, requestID string) (model.QuotaRequest, error) {
	f.reject++
	f.admin, f.id = adminID, requestID
	r := f.req
	r.Status = model.QuotaRequestRejected
	return r, f.err
}

func decidedReq() model.QuotaRequest {
	at := time.Date(2026, 11, 26, 14, 0, 0, 0, time.UTC)
	return model.QuotaRequest{ID: "r1", UserID: "u1", RequestedBytes: 5 << 30, Status: model.QuotaRequestPending, CreatedAt: at.Add(-time.Hour), DecidedAt: &at}
}

// decideVia poziva handler preko mux-a, da {id} bude stvarna vrednost iz putanje.
func decideVia(h http.HandlerFunc, action, id, userID string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /admin/quota-requests/{id}/"+action, h)
	req := httptest.NewRequest(http.MethodPost, "/admin/quota-requests/"+id+"/"+action, nil)
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestQuotaDecisionHandlers_OK(t *testing.T) {
	cases := []struct {
		action string
		status string
		h      func(*fakeDecider) http.HandlerFunc
		calls  func(*fakeDecider) (int, int)
	}{
		{"approve", "APPROVED", func(f *fakeDecider) http.HandlerFunc { return ApproveQuotaRequestHandler(f) }, func(f *fakeDecider) (int, int) { return f.approve, f.reject }},
		{"reject", "REJECTED", func(f *fakeDecider) http.HandlerFunc { return RejectQuotaRequestHandler(f) }, func(f *fakeDecider) (int, int) { return f.reject, f.approve }},
	}
	for _, c := range cases {
		f := &fakeDecider{req: decidedReq()}
		rec := decideVia(c.h(f), c.action, "r1", "admin")

		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, telo %s", c.action, rec.Code, rec.Body.String())
		}
		var got map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got["id"] != "r1" || got["status"] != c.status || got["decidedAt"] == nil {
			t.Errorf("%s: telo %s", c.action, rec.Body.String())
		}
		own, other := c.calls(f)
		if own != 1 || other != 0 {
			t.Errorf("%s: pozivi: svoj=%d, tudj=%d", c.action, own, other)
		}
		if f.admin != "admin" || f.id != "r1" {
			t.Errorf("%s: servisu prosledjeno %q %q", c.action, f.admin, f.id)
		}
		if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Content-Type") != "application/json" {
			t.Errorf("%s: zaglavlja %v", c.action, rec.Header())
		}
	}
}

func TestQuotaDecisionHandlers_Unauthenticated(t *testing.T) {
	f := &fakeDecider{}
	for _, id := range []string{"", strings.Repeat("a", maxUserIDLen+1)} {
		if rec := decideVia(ApproveQuotaRequestHandler(f), "approve", "r1", id); rec.Code != http.StatusUnauthorized {
			t.Errorf("approve %q: status = %d", id, rec.Code)
		}
		if rec := decideVia(RejectQuotaRequestHandler(f), "reject", "r1", id); rec.Code != http.StatusUnauthorized {
			t.Errorf("reject %q: status = %d", id, rec.Code)
		}
	}
	if f.approve+f.reject != 0 {
		t.Error("bez prijave servis se ne zove")
	}
}

func TestQuotaDecisionHandlers_ErrorStatuses(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
		msg  string
	}{
		{"nije administrator", service.ErrForbidden, http.StatusForbidden, "zabranjen"},
		{"zahtev ne postoji", service.ErrQuotaRequestNotFound, http.StatusNotFound, "zahtev za kvotu"},
		{"korisnik obrisan", service.ErrUserNotFound, http.StatusNotFound, "korisnik"},
		{"vec odlucen", service.ErrQuotaRequestDecided, http.StatusConflict, "odlučen"},
	}
	for _, c := range cases {
		for action, h := range map[string]func(*fakeDecider) http.HandlerFunc{
			"approve": func(f *fakeDecider) http.HandlerFunc { return ApproveQuotaRequestHandler(f) },
			"reject":  func(f *fakeDecider) http.HandlerFunc { return RejectQuotaRequestHandler(f) },
		} {
			rec := decideVia(h(&fakeDecider{err: c.err}), action, "r1", "user")
			if rec.Code != c.code || !strings.Contains(rec.Body.String(), c.msg) || !strings.Contains(rec.Body.String(), `"error"`) {
				t.Errorf("%s/%s: status = %d, telo %s", c.name, action, rec.Code, rec.Body.String())
			}
		}
	}
}

func TestQuotaDecisionHandlers_ForbiddenLeaksNothing(t *testing.T) {
	f := &fakeDecider{err: service.ErrForbidden, req: decidedReq()}
	rec := decideVia(ApproveQuotaRequestHandler(f), "approve", "r1", "user")
	if strings.Contains(rec.Body.String(), "r1") || strings.Contains(rec.Body.String(), "requestedBytes") {
		t.Errorf("odgovor 403 ne sme sadrzati podatke o zahtevu: %s", rec.Body.String())
	}
}

func TestQuotaDecisionHandlers_InternalErrorHidesDetails(t *testing.T) {
	f := &fakeDecider{err: errors.New("mongo: connection refused 10.0.0.5")}
	for action, h := range map[string]http.HandlerFunc{"approve": ApproveQuotaRequestHandler(f), "reject": RejectQuotaRequestHandler(f)} {
		rec := decideVia(h, action, "r1", "admin")
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("%s: status = %d", action, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "mongo") || strings.Contains(rec.Body.String(), "10.0.0.5") {
			t.Errorf("%s: telo otkriva interne detalje: %s", action, rec.Body.String())
		}
	}
}
