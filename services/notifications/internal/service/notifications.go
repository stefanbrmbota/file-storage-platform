// Package service je poslovna logika Notifications servisa.
package service

import (
	"context"
	"errors"
	"strings"

	"github.com/stefanbrmbota/file-storage-platform/services/notifications/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/notifications/internal/repository"
)

const (
	// DefaultLimit je velicina stranice kad klijent ne zada limit.
	DefaultLimit = 20
	// MaxLimit je najveca dozvoljena velicina stranice.
	MaxLimit = 100
)

var (
	// ErrMissingUser znaci da nije poznato ciji su zahtev (nema X-User-ID).
	ErrMissingUser = errors.New("service: nepoznat korisnik")
	// ErrInvalidCursor znaci da kursor za sledecu stranicu nije ispravan.
	ErrInvalidCursor = errors.New("service: neispravan kursor")
)

// Repository je citanje obavestenja iz baze (repository.Store).
type Repository interface {
	ListByUser(ctx context.Context, userID, before string, limit int) ([]model.Notification, error)
	CountUnread(ctx context.Context, userID string) (int, error)
}

// Page je jedna stranica obavestenja. NextCursor je prazan na poslednjoj stranici.
type Page struct {
	Items      []model.Notification `json:"items"`
	NextCursor string               `json:"nextCursor,omitempty"`
}

// Notifications cita obavestenja korisnika (US-07).
type Notifications struct {
	repo Repository
}

// NewNotifications pravi servis nad repozitorijumom.
func NewNotifications(repo Repository) *Notifications {
	return &Notifications{repo: repo}
}

// List vraca stranicu obavestenja korisnika userID, od najnovijeg. cursor je
// NextCursor sa prethodne stranice ("" za prvu), a limit velicina stranice
// (0 = DefaultLimit, najvise MaxLimit). Korisnik uvek dobija samo svoja
// obavestenja, jer se cita samo njegova particija.
func (s *Notifications) List(ctx context.Context, userID, cursor string, limit int) (Page, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return Page{}, ErrMissingUser
	}
	switch {
	case limit <= 0:
		limit = DefaultLimit
	case limit > MaxLimit:
		limit = MaxLimit
	}

	// Jedan vise od trazenog: ako stigne, postoji sledeca stranica.
	items, err := s.repo.ListByUser(ctx, userID, cursor, limit+1)
	if errors.Is(err, repository.ErrInvalidCursor) {
		return Page{}, ErrInvalidCursor
	}
	if err != nil {
		return Page{}, err
	}

	if items == nil {
		items = []model.Notification{} // klijent uvek dobija niz, ne null
	}
	page := Page{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = page.Items[limit-1].ID
	}
	return page, nil
}

// UnreadCount vraca broj neprocitanih obavestenja korisnika.
func (s *Notifications) UnreadCount(ctx context.Context, userID string) (int, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return 0, ErrMissingUser
	}
	return s.repo.CountUnread(ctx, userID)
}
