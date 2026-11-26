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

	"github.com/stefanbrmbota/file-storage-platform/services/notifications/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/notifications/internal/service"
)

type fakeLister struct {
	page      service.Page
	count     int
	err       error
	gotUser   string
	gotCursor string
	gotLimit  int
}

func (f *fakeLister) List(_ context.Context, userID, cursor string, limit int) (service.Page, error) {
	f.gotUser, f.gotCursor, f.gotLimit = userID, cursor, limit
	return f.page, f.err
}

func (f *fakeLister) UnreadCount(_ context.Context, userID string) (int, error) {
	f.gotUser = userID
	return f.count, f.err
}

func do(h http.HandlerFunc, target, userID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if userID != "" {
		req.Header.Set(HeaderUserID, userID)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func errorOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var b map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&b); err != nil {
		t.Fatalf("telo nije JSON: %v", err)
	}
	return b["error"]
}

func TestListReturnsPage(t *testing.T) {
	created := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	f := &fakeLister{page: service.Page{
		Items:      []model.Notification{{ID: "id-2", Type: "SHARE_GRANTED", Message: "Marko je podelio folder", CreatedAt: created}},
		NextCursor: "id-2",
	}}
	rec := do(ListNotifications(f), "/notifications?limit=1&cursor=id-9", "u1")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, telo = %s", rec.Code, rec.Body)
	}
	if f.gotUser != "u1" || f.gotCursor != "id-9" || f.gotLimit != 1 {
		t.Errorf("servis dobio user=%q cursor=%q limit=%d", f.gotUser, f.gotCursor, f.gotLimit)
	}
	var got struct {
		Items []struct {
			ID, Type, Message string
			CreatedAt         time.Time
			IsRead            bool
		}
		NextCursor string
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Items[0].Message != "Marko je podelio folder" ||
		!got.Items[0].CreatedAt.Equal(created) || got.NextCursor != "id-2" {
		t.Errorf("odgovor = %+v", got)
	}
}

func TestListEmptyIsArray(t *testing.T) {
	rec := do(ListNotifications(&fakeLister{page: service.Page{Items: []model.Notification{}}}), "/notifications", "u1")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Errorf("status = %d, telo = %s", rec.Code, rec.Body)
	}
}

func TestListWithoutUserIs401(t *testing.T) {
	f := &fakeLister{}
	rec := do(ListNotifications(f), "/notifications", "")
	if rec.Code != http.StatusUnauthorized || errorOf(t, rec) != "unauthorized" {
		t.Fatalf("status = %d", rec.Code)
	}
	if f.gotUser != "" {
		t.Error("servis ne sme biti pozvan bez korisnika")
	}
}

func TestListIgnoresUserIDFromQuery(t *testing.T) {
	f := &fakeLister{}
	do(ListNotifications(f), "/notifications?userId=tudji&user_id=tudji", "u1")
	if f.gotUser != "u1" {
		t.Errorf("servis dobio %q; ID sme doci samo iz X-User-ID", f.gotUser)
	}
}

func TestListBadInput(t *testing.T) {
	for _, target := range []string{
		"/notifications?limit=abc",
		"/notifications?limit=0",
		"/notifications?limit=-3",
		"/notifications?limit=101",
		"/notifications?cursor=" + strings.Repeat("a", maxCursorLen+1),
	} {
		rec := do(ListNotifications(&fakeLister{}), target, "u1")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, ocekivano 400", target, rec.Code)
		}
	}
	rec := do(ListNotifications(&fakeLister{err: service.ErrInvalidCursor}), "/notifications?cursor=x", "u1")
	if rec.Code != http.StatusBadRequest || errorOf(t, rec) != "neispravan kursor" {
		t.Errorf("los kursor: status = %d", rec.Code)
	}
}

func TestListDatabaseErrorHidesDetails(t *testing.T) {
	rec := do(ListNotifications(&fakeLister{err: errors.New("gocql: no hosts available 10.0.0.5")}), "/notifications", "u1")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
	if msg := errorOf(t, rec); strings.Contains(msg, "gocql") || strings.Contains(msg, "10.0.0.5") {
		t.Errorf("interni detalji u odgovoru: %q", msg)
	}
}

func TestUnreadCount(t *testing.T) {
	f := &fakeLister{count: 4}
	rec := do(UnreadCount(f), "/notifications/unread-count", "u1")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"count":4}` || f.gotUser != "u1" {
		t.Errorf("status = %d, telo = %s, user = %q", rec.Code, rec.Body, f.gotUser)
	}
	if rec := do(UnreadCount(&fakeLister{}), "/notifications/unread-count", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("bez korisnika: status = %d", rec.Code)
	}
	if rec := do(UnreadCount(&fakeLister{err: errors.New("x")}), "/notifications/unread-count", "u1"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("greska baze: status = %d", rec.Code)
	}
}
