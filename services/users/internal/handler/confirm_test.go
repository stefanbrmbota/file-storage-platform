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

type fakeConfirmer struct {
	err   error
	token string
	calls int
}

func (f *fakeConfirmer) Confirm(_ context.Context, token string) error {
	f.calls++
	f.token = token
	return f.err
}

func doConfirm(c Confirmer, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/confirm", strings.NewReader(body))
	rec := httptest.NewRecorder()
	ConfirmHandler(c).ServeHTTP(rec, req)
	return rec
}

func TestConfirmHandler_Success(t *testing.T) {
	c := &fakeConfirmer{}
	rec := doConfirm(c, `{"token":"abc"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, očekivano 200", rec.Code)
	}
	if c.token != "abc" {
		t.Errorf("prosleđen token %q", c.token)
	}
}

func TestConfirmHandler_Errors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		err        error
		wantStatus int
		wantMsg    string
		wantCalled bool
	}{
		{"prazno telo", ``, nil, http.StatusBadRequest, "neispravan zahtev", false},
		{"loš JSON", `{nije`, nil, http.StatusBadRequest, "neispravan zahtev", false},
		{"nepoznato polje", `{"token":"a","x":1}`, nil, http.StatusBadRequest, "neispravan zahtev", false},
		{"nevažeći token", `{"token":"a"}`, service.ErrInvalidToken, http.StatusBadRequest, "nevažeći, istekao ili je već iskorišćen", true},
		{"Files nedostupan", `{"token":"a"}`, service.ErrUpstream, http.StatusBadGateway, "nedostupan", true},
		{"interna greška", `{"token":"a"}`, errors.New("mongo: 10.0.0.5 refused"), http.StatusInternalServerError, "interna greška", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &fakeConfirmer{err: tt.err}
			rec := doConfirm(c, tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, očekivano %d", rec.Code, tt.wantStatus)
			}
			if msg := errorOf(t, rec); !strings.Contains(msg, tt.wantMsg) {
				t.Errorf("poruka %q ne sadrži %q", msg, tt.wantMsg)
			}
			if (c.calls > 0) != tt.wantCalled {
				t.Errorf("Confirm pozvan %d puta", c.calls)
			}
			if strings.Contains(rec.Body.String(), "mongo") || strings.Contains(rec.Body.String(), "10.0.0.5") {
				t.Errorf("interni detalji procureli: %s", rec.Body.String())
			}
		})
	}
}
