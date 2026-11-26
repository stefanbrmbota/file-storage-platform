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

type fakeQuotaSubmitter struct {
	req   model.QuotaRequest
	err   error
	user  string
	bytes int64
	calls int
}

func (f *fakeQuotaSubmitter) Submit(_ context.Context, userID string, requestedBytes int64) (model.QuotaRequest, error) {
	f.calls++
	f.user, f.bytes = userID, requestedBytes
	return f.req, f.err
}

type fakeQuotaReader struct {
	req   model.QuotaRequest
	err   error
	user  string
	calls int
}

func (f *fakeQuotaReader) Latest(_ context.Context, userID string) (model.QuotaRequest, error) {
	f.calls++
	f.user = userID
	return f.req, f.err
}

func pendingReq() model.QuotaRequest {
	return model.QuotaRequest{
		ID: "r1", UserID: "u1", RequestedBytes: 5 << 30, Status: model.QuotaRequestPending,
		CreatedAt: time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC),
	}
}

// postAs šalje POST sa zaglavljem X-User-ID (prazno = bez zaglavlja).
func postAs(h http.HandlerFunc, userID, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/users/me/quota-requests", strings.NewReader(body))
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func getAs(h http.HandlerFunc, userID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/users/me/quota-requests/latest", nil)
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// ---- POST /users/me/quota-requests ----

func TestSubmitQuotaRequestHandler_Created(t *testing.T) {
	s := &fakeQuotaSubmitter{req: pendingReq()}
	rec := postAs(SubmitQuotaRequestHandler(s), "u1", `{"requestedBytes":5368709120}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, telo %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["status"] != "PENDING" || got["requestedBytes"] != float64(5<<30) || got["id"] != "r1" {
		t.Errorf("telo: %s", rec.Body.String())
	}
	if _, has := got["decidedAt"]; has {
		t.Errorf("decidedAt se ne salje dok nema odluke: %s", rec.Body.String())
	}
	if s.user != "u1" || s.bytes != 5<<30 || s.calls != 1 {
		t.Errorf("servisu prosledjeno %q %d (%d poziva)", s.user, s.bytes, s.calls)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", rec.Header().Get("Content-Type"))
	}
}

func TestSubmitQuotaRequestHandler_IdentityOnlyFromHeader(t *testing.T) {
	s := &fakeQuotaSubmitter{req: pendingReq()}
	// Korisnik u telu ne moze da podnese zahtev u tudje ime: nepoznato polje se odbija.
	rec := postAs(SubmitQuotaRequestHandler(s), "u1", `{"requestedBytes":5368709120,"userId":"u2"}`)
	if rec.Code != http.StatusBadRequest || s.calls != 0 {
		t.Fatalf("status = %d, poziva = %d; ocekivano 400 bez poziva servisa", rec.Code, s.calls)
	}
}

func TestSubmitQuotaRequestHandler_Unauthenticated(t *testing.T) {
	s := &fakeQuotaSubmitter{}
	for name, id := range map[string]string{"bez zaglavlja": "", "predug id": strings.Repeat("a", maxUserIDLen+1)} {
		rec := postAs(SubmitQuotaRequestHandler(s), id, `{"requestedBytes":5368709120}`)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, ocekivano 401", name, rec.Code)
		}
	}
	if s.calls != 0 {
		t.Error("bez prijave servis se ne zove")
	}
}

func TestSubmitQuotaRequestHandler_MalformedBody(t *testing.T) {
	for name, body := range map[string]string{
		"prazno":       ``,
		"nije json":    `requestedBytes=5`,
		"pogresan tip": `{"requestedBytes":"puno"}`,
		"decimalan":    `{"requestedBytes":1.5}`,
		"prevelik":     `{"requestedBytes":5,"x":"` + strings.Repeat("a", 2000) + `"}`,
	} {
		s := &fakeQuotaSubmitter{}
		rec := postAs(SubmitQuotaRequestHandler(s), "u1", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, ocekivano 400", name, rec.Code)
		}
		if s.calls != 0 {
			t.Errorf("%s: servis se ne sme zvati", name)
		}
	}
}

func TestSubmitQuotaRequestHandler_ServiceErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
		msg  string
	}{
		{"neispravna vrednost", service.ErrInvalidQuotaRequest, http.StatusBadRequest, "nije ispravna"},
		{"nije vece od kvote", service.ErrQuotaNotIncreased, http.StatusBadRequest, "veća od trenutne"},
		{"vec na cekanju", service.ErrQuotaRequestPending, http.StatusConflict, "na čekanju"},
		{"nalog ne postoji", service.ErrUserNotFound, http.StatusNotFound, "nalog"},
	}
	for _, c := range cases {
		rec := postAs(SubmitQuotaRequestHandler(&fakeQuotaSubmitter{err: c.err}), "u1", `{"requestedBytes":5368709120}`)
		if rec.Code != c.code || !strings.Contains(rec.Body.String(), c.msg) || !strings.Contains(rec.Body.String(), `"error"`) {
			t.Errorf("%s: status = %d, telo %s", c.name, rec.Code, rec.Body.String())
		}
	}
}

func TestSubmitQuotaRequestHandler_InternalErrorHidesDetails(t *testing.T) {
	rec := postAs(SubmitQuotaRequestHandler(&fakeQuotaSubmitter{err: errors.New("mongo: connection refused 10.0.0.5")}), "u1", `{"requestedBytes":5368709120}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "mongo") || strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Errorf("telo otkriva interne detalje: %s", rec.Body.String())
	}
}

// ---- GET /users/me/quota-requests/latest ----

func TestLatestQuotaRequestHandler_OK(t *testing.T) {
	s := &fakeQuotaReader{req: pendingReq()}
	rec := getAs(LatestQuotaRequestHandler(s), "u1")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, telo %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"PENDING"`) {
		t.Errorf("telo: %s", rec.Body.String())
	}
	if s.user != "u1" {
		t.Errorf("servisu prosledjeno %q; identitet je samo iz zaglavlja", s.user)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
}

func TestLatestQuotaRequestHandler_Unauthenticated(t *testing.T) {
	s := &fakeQuotaReader{}
	if rec := getAs(LatestQuotaRequestHandler(s), ""); rec.Code != http.StatusUnauthorized || s.calls != 0 {
		t.Fatalf("status = %d, poziva = %d", rec.Code, s.calls)
	}
}

func TestLatestQuotaRequestHandler_NoRequest(t *testing.T) {
	for _, err := range []error{service.ErrNoQuotaRequest, service.ErrUserNotFound} {
		rec := getAs(LatestQuotaRequestHandler(&fakeQuotaReader{err: err}), "u1")
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"error"`) {
			t.Errorf("%v: status = %d, telo %s", err, rec.Code, rec.Body.String())
		}
	}
}

func TestLatestQuotaRequestHandler_InternalErrorHidesDetails(t *testing.T) {
	rec := getAs(LatestQuotaRequestHandler(&fakeQuotaReader{err: errors.New("mongo: connection refused 10.0.0.5")}), "u1")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "mongo") || strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Errorf("telo otkriva interne detalje: %s", rec.Body.String())
	}
}
