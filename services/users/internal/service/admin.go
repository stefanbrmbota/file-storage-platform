package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

// ErrForbidden se vraća kada pozivalac nije aktivan administrator. Isto važi za
// prazan ID, nepoznat i neaktivan nalog, pa odgovor ne otkriva ništa o nalozima.
var ErrForbidden = errors.New("service: zabranjen pristup")

// AdminUser je korisnik u administratorskom pregledu (US-22). Namerno nema
// heš lozinke ni druga interna polja.
type AdminUser struct {
	ID         string     `json:"id"`
	FirstName  string     `json:"firstName"`
	LastName   string     `json:"lastName"`
	Email      string     `json:"email"`
	Role       model.Role `json:"role"`
	Active     bool       `json:"active"`
	QuotaBytes int64      `json:"quotaBytes"`
	UsedBytes  int64      `json:"usedBytes"`
}

// AdminQuotaRequest je zahtev za kvotu u administratorskom pregledu (US-22):
// podnosilac, njegova trenutna kvota, tražena kvota i status. Ako je nalog
// podnosioca obrisan, ime, email i trenutna kvota su prazni.
type AdminQuotaRequest struct {
	ID                string                   `json:"id"`
	UserID            string                   `json:"userId"`
	FirstName         string                   `json:"firstName"`
	LastName          string                   `json:"lastName"`
	Email             string                   `json:"email"`
	CurrentQuotaBytes int64                    `json:"currentQuotaBytes"`
	RequestedBytes    int64                    `json:"requestedBytes"`
	Status            model.QuotaRequestStatus `json:"status"`
	CreatedAt         time.Time                `json:"createdAt"`
	DecidedAt         *time.Time               `json:"decidedAt,omitempty"`
}

// AdminService daje administratoru pregled korisnika i zahteva za kvotu (US-22).
type AdminService struct {
	accounts AccountFinder
	users    repository.UserLister
	requests repository.QuotaRequestLister
}

// NewAdminService vraća servis za administratorski pregled.
func NewAdminService(accounts AccountFinder, users repository.UserLister, requests repository.QuotaRequestLister) *AdminService {
	return &AdminService{accounts: accounts, users: users, requests: requests}
}

// requireAdmin proverava ulogu u bazi, a ne samo zaglavlje koje šalje gateway:
// uloga u tokenu može biti zastarela, a servis je poslednja linija odbrane.
func (s *AdminService) requireAdmin(ctx context.Context, adminID string) error {
	if strings.TrimSpace(adminID) == "" {
		return ErrForbidden
	}
	u, err := s.accounts.FindByID(ctx, adminID)
	switch {
	case err == nil:
	case errors.Is(err, repository.ErrNotFound):
		return ErrForbidden
	default:
		return fmt.Errorf("admin: provera uloge: %w", err)
	}
	if !u.Active || u.Role != model.RoleAdmin {
		return ErrForbidden
	}
	return nil
}

// ListUsers vraća korisnike (ime, email, kvota, zauzeće), od najnovijeg.
// Ne-administrator dobija ErrForbidden.
func (s *AdminService) ListUsers(ctx context.Context, adminID string) ([]AdminUser, error) {
	if err := s.requireAdmin(ctx, adminID); err != nil {
		return nil, err
	}
	users, err := s.users.List(ctx, repository.MaxListLimit)
	if err != nil {
		return nil, fmt.Errorf("admin: lista korisnika: %w", err)
	}
	out := make([]AdminUser, 0, len(users))
	for _, u := range users {
		out = append(out, AdminUser{
			ID: u.ID, FirstName: u.FirstName, LastName: u.LastName, Email: u.Email,
			Role: u.Role, Active: u.Active, QuotaBytes: u.QuotaBytes, UsedBytes: u.UsedBytes,
		})
	}
	return out, nil
}

// ListQuotaRequests vraća zahteve za kvotu sa podnosiocem i njegovom trenutnom
// kvotom: prvo oni na čekanju, pa ostali. Ne-administrator dobija ErrForbidden.
func (s *AdminService) ListQuotaRequests(ctx context.Context, adminID string) ([]AdminQuotaRequest, error) {
	if err := s.requireAdmin(ctx, adminID); err != nil {
		return nil, err
	}
	list, err := s.requests.ListWithRequester(ctx, repository.MaxListLimit)
	if err != nil {
		return nil, fmt.Errorf("admin: lista zahteva za kvotu: %w", err)
	}
	out := make([]AdminQuotaRequest, 0, len(list))
	for _, d := range list {
		r := AdminQuotaRequest{
			ID: d.ID, UserID: d.UserID, RequestedBytes: d.RequestedBytes,
			Status: d.Status, CreatedAt: d.CreatedAt, DecidedAt: d.DecidedAt,
		}
		if d.Requester != nil {
			r.FirstName, r.LastName, r.Email = d.Requester.FirstName, d.Requester.LastName, d.Requester.Email
			r.CurrentQuotaBytes = d.Requester.QuotaBytes
		}
		out = append(out, r)
	}
	return out, nil
}
