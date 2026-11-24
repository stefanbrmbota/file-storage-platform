package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/service"
)

type fakeReserver struct {
	remaining int64
	err       error
	calls     int
	id        string
	bytes     int64
}

func (f *fakeReserver) ReserveQuota(_ context.Context, id string, bytes int64) (int64, error) {
	f.calls++
	f.id, f.bytes = id, bytes
	return f.remaining, f.err
}

func doReserve(q QuotaReserver, id, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/users/{id}/quota/reserve", ReserveQuotaHandler(q))
	req := httptest.NewRequest(http.MethodPost, "/internal/users/"+id+"/quota/reserve", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestReserveQuotaHandler_OK(t *testing.T) {
	q := &fakeReserver{remaining: 900}
	rec := doReserve(q, "u1", `{"bytes":100}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, ocekivano 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"reserved":100`) || !strings.Contains(rec.Body.String(), `"remainingBytes":900`) {
		t.Errorf("odgovor: %s", rec.Body.String())
	}
	if q.id != "u1" || q.bytes != 100 {
		t.Errorf("prosledjeno: %q, %d", q.id, q.bytes)
	}
}

func TestReserveQuotaHandler_BadBody(t *testing.T) {
	for _, body := range []string{``, `nije json`, `{"bytes":"x"}`, `{"bytes":1,"extra":2}`} {
		q := &fakeReserver{}
		rec := doReserve(q, "u1", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("telo %.20q: status = %d, ocekivano 400", body, rec.Code)
		}
		if q.calls != 0 {
			t.Errorf("telo %.20q: servis ne sme biti pozvan", body)
		}
	}
}

func TestReserveQuotaHandler_ErrorMapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"neispravan zahtev", service.ErrInvalidQuotaRequest, http.StatusBadRequest},
		{"nepoznat korisnik", service.ErrUserNotFound, http.StatusNotFound},
		{"nema mesta", service.ErrQuotaExceeded, http.StatusConflict},
		{"greska baze", errors.New("mongo: connection refused 10.0.0.5"), http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doReserve(&fakeReserver{err: tc.err}, "u1", `{"bytes":10}`)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, ocekivano %d", rec.Code, tc.want)
			}
			if !strings.Contains(rec.Body.String(), `"error"`) {
				t.Errorf("odgovor mora biti JSON sa error: %s", rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "mongo") || strings.Contains(rec.Body.String(), "10.0.0.5") {
				t.Errorf("odgovor otkriva interne detalje: %s", rec.Body.String())
			}
		})
	}
}

type fakeReleaser struct {
	used  int64
	err   error
	calls int
	id    string
	bytes int64
}

func (f *fakeReleaser) ReleaseQuota(_ context.Context, id string, bytes int64) (int64, error) {
	f.calls++
	f.id, f.bytes = id, bytes
	return f.used, f.err
}

func doRelease(q QuotaReleaser, id, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/users/{id}/quota/release", ReleaseQuotaHandler(q))
	req := httptest.NewRequest(http.MethodPost, "/internal/users/"+id+"/quota/release", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestReleaseQuotaHandler_OK(t *testing.T) {
	q := &fakeReleaser{used: 400}
	rec := doRelease(q, "u1", `{"bytes":100}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, ocekivano 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"released":100`) || !strings.Contains(rec.Body.String(), `"usedBytes":400`) {
		t.Errorf("odgovor: %s", rec.Body.String())
	}
	if q.id != "u1" || q.bytes != 100 {
		t.Errorf("prosledjeno: %q, %d", q.id, q.bytes)
	}
}

func TestReleaseQuotaHandler_BadBody(t *testing.T) {
	for _, body := range []string{``, `nije json`, `{"bytes":"x"}`, `{"bytes":1,"extra":2}`} {
		q := &fakeReleaser{}
		rec := doRelease(q, "u1", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("telo %.20q: status = %d, ocekivano 400", body, rec.Code)
		}
		if q.calls != 0 {
			t.Errorf("telo %.20q: servis ne sme biti pozvan", body)
		}
	}
}

func TestReleaseQuotaHandler_ErrorMapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"neispravan zahtev", service.ErrInvalidQuotaRequest, http.StatusBadRequest},
		{"nepoznat korisnik", service.ErrUserNotFound, http.StatusNotFound},
		{"greska baze", errors.New("mongo: connection refused 10.0.0.5"), http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRelease(&fakeReleaser{err: tc.err}, "u1", `{"bytes":10}`)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, ocekivano %d", rec.Code, tc.want)
			}
			if !strings.Contains(rec.Body.String(), `"error"`) {
				t.Errorf("odgovor mora biti JSON sa error: %s", rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "mongo") || strings.Contains(rec.Body.String(), "10.0.0.5") {
				t.Errorf("odgovor otkriva interne detalje: %s", rec.Body.String())
			}
		})
	}
}
