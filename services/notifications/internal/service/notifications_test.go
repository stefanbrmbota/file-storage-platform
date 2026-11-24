package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/notifications/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/notifications/internal/repository"
)

// fakeRepo cuva obavestenja po korisniku, vec sortirana od najnovijeg, i
// oponasa upit "WHERE user_id = ? AND notification_id < ? LIMIT ?".
type fakeRepo struct {
	byUser    map[string][]model.Notification
	unread    map[string]int
	err       error
	gotUser   string
	gotBefore string
	gotLimit  int
}

func (f *fakeRepo) ListByUser(_ context.Context, userID, before string, limit int) ([]model.Notification, error) {
	f.gotUser, f.gotBefore, f.gotLimit = userID, before, limit
	if f.err != nil {
		return nil, f.err
	}
	if before == "bad" {
		return nil, repository.ErrInvalidCursor
	}
	all := f.byUser[userID]
	start := 0
	if before != "" {
		for i, n := range all {
			if n.ID == before {
				start = i + 1
			}
		}
	}
	end := min(start+limit, len(all))
	return all[start:end], nil
}

func (f *fakeRepo) CountUnread(_ context.Context, userID string) (int, error) {
	f.gotUser = userID
	return f.unread[userID], f.err
}

// newest pravi n obavestenja od najnovijeg (id n-1) do najstarijeg (id 0).
func newest(prefix string, n int) []model.Notification {
	out := make([]model.Notification, 0, n)
	base := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	for i := n - 1; i >= 0; i-- {
		out = append(out, model.Notification{
			ID: fmt.Sprintf("%s-%02d", prefix, i), Message: fmt.Sprintf("poruka %d", i),
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		})
	}
	return out
}

func TestListFirstPageAndCursor(t *testing.T) {
	repo := &fakeRepo{byUser: map[string][]model.Notification{"u1": newest("a", 5)}}
	s := NewNotifications(repo)

	p1, err := s.List(context.Background(), "u1", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if repo.gotLimit != 3 {
		t.Errorf("repo trazen limit %d, ocekivano 3 (jedan vise)", repo.gotLimit)
	}
	if len(p1.Items) != 2 || p1.Items[0].ID != "a-04" || p1.Items[1].ID != "a-03" || p1.NextCursor != "a-03" {
		t.Fatalf("prva stranica = %+v", p1)
	}

	p2, _ := s.List(context.Background(), "u1", p1.NextCursor, 2)
	if len(p2.Items) != 2 || p2.Items[0].ID != "a-02" || p2.NextCursor != "a-01" {
		t.Fatalf("druga stranica = %+v", p2)
	}

	p3, _ := s.List(context.Background(), "u1", p2.NextCursor, 2)
	if len(p3.Items) != 1 || p3.Items[0].ID != "a-00" || p3.NextCursor != "" {
		t.Fatalf("poslednja stranica = %+v", p3)
	}
}

func TestListNewestFirst(t *testing.T) {
	repo := &fakeRepo{byUser: map[string][]model.Notification{"u1": newest("a", 4)}}
	p, _ := NewNotifications(repo).List(context.Background(), "u1", "", 0)
	for i := 1; i < len(p.Items); i++ {
		if !p.Items[i-1].CreatedAt.After(p.Items[i].CreatedAt) {
			t.Fatalf("nije od najnovijeg: %v pa %v", p.Items[i-1].CreatedAt, p.Items[i].CreatedAt)
		}
	}
}

func TestListOnlyOwnNotifications(t *testing.T) {
	repo := &fakeRepo{byUser: map[string][]model.Notification{
		"u1": newest("u1", 3), "u2": newest("u2", 3),
	}}
	p, _ := NewNotifications(repo).List(context.Background(), "u1", "", 10)
	if repo.gotUser != "u1" {
		t.Fatalf("repo pitan za %q", repo.gotUser)
	}
	for _, n := range p.Items {
		if n.ID[:2] != "u1" {
			t.Errorf("tudje obavestenje: %s", n.ID)
		}
	}
}

func TestListEmpty(t *testing.T) {
	p, err := NewNotifications(&fakeRepo{}).List(context.Background(), "u1", "", 10)
	if err != nil || len(p.Items) != 0 || p.NextCursor != "" {
		t.Fatalf("p = %+v, err = %v", p, err)
	}
}

func TestListLimits(t *testing.T) {
	cases := map[int]int{0: DefaultLimit + 1, -5: DefaultLimit + 1, 7: 8, MaxLimit + 50: MaxLimit + 1}
	for limit, want := range cases {
		repo := &fakeRepo{}
		_, _ = NewNotifications(repo).List(context.Background(), "u1", "", limit)
		if repo.gotLimit != want {
			t.Errorf("limit %d: repo dobio %d, ocekivano %d", limit, repo.gotLimit, want)
		}
	}
}

func TestListErrors(t *testing.T) {
	s := NewNotifications(&fakeRepo{})
	if _, err := s.List(context.Background(), "  ", "", 10); !errors.Is(err, ErrMissingUser) {
		t.Errorf("bez korisnika: %v", err)
	}
	if _, err := s.List(context.Background(), "u1", "bad", 10); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("los kursor: %v", err)
	}
	dbErr := errors.New("baza pala")
	if _, err := NewNotifications(&fakeRepo{err: dbErr}).List(context.Background(), "u1", "", 10); !errors.Is(err, dbErr) {
		t.Errorf("greska baze: %v", err)
	}
}

func TestUnreadCount(t *testing.T) {
	repo := &fakeRepo{unread: map[string]int{"u1": 3, "u2": 9}}
	s := NewNotifications(repo)
	n, err := s.UnreadCount(context.Background(), "u1")
	if err != nil || n != 3 || repo.gotUser != "u1" {
		t.Fatalf("n = %d, err = %v, user = %q", n, err, repo.gotUser)
	}
	if _, err := s.UnreadCount(context.Background(), ""); !errors.Is(err, ErrMissingUser) {
		t.Errorf("bez korisnika: %v", err)
	}
}
