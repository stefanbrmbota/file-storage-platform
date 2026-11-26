package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

const maxQuotaRequestIDLen = 128

var (
	// ErrQuotaRequestNotFound se vraća kada zahtev sa zadatim ID-jem ne postoji.
	ErrQuotaRequestNotFound = errors.New("service: zahtev za kvotu nije pronadjen")
	// ErrQuotaRequestDecided se vraća kada je zahtev već odobren ili odbijen.
	ErrQuotaRequestDecided = errors.New("service: zahtev za kvotu je vec odlucen")
)

// QuotaDecisionUsers je pristup nalozima potreban za odluku o zahtevu: provera
// uloge administratora i postavljanje kvote korisnika.
type QuotaDecisionUsers interface {
	AccountFinder
	repository.QuotaSetter
}

// QuotaDecisionService omogućava administratoru da odobri ili odbije zahtev za
// povećanje kvote (US-23).
type QuotaDecisionService struct {
	users    QuotaDecisionUsers
	requests repository.QuotaRequestDecider
	now      func() time.Time
}

// NewQuotaDecisionService vraća servis za odluke o zahtevima za kvotu.
func NewQuotaDecisionService(users QuotaDecisionUsers, requests repository.QuotaRequestDecider) *QuotaDecisionService {
	return &QuotaDecisionService{users: users, requests: requests, now: func() time.Time { return time.Now().UTC() }}
}

// Approve odobrava zahtev: kvota korisnika se postavlja na traženu vrednost, a
// zahtev dobija status APPROVED. Lokalna baza nema transakcije preko dve
// kolekcije, pa je redosled: (1) atomično se "zauzima" zahtev na čekanju (samo
// jedan od istovremenih poziva uspeva, pa nema dvostrukog odobravanja), (2) upisuje
// se kvota, (3) ako upis kvote ne uspe, zahtev se vraća na čekanje i administrator
// može da pokuša ponovo (ili da ga odbije). Ne-administrator dobija ErrForbidden,
// nepoznat zahtev ErrQuotaRequestNotFound, već odlučen ErrQuotaRequestDecided, a
// zahtev obrisanog korisnika ErrUserNotFound (ostaje na čekanju, može se odbiti).
// Odluka se loguje kao administratorska aktivnost.
func (s *QuotaDecisionService) Approve(ctx context.Context, adminID, requestID string) (model.QuotaRequest, error) {
	if err := s.authorize(ctx, adminID, requestID); err != nil {
		return model.QuotaRequest{}, err
	}
	req, err := s.decide(ctx, requestID, model.QuotaRequestApproved)
	if err != nil {
		return model.QuotaRequest{}, err
	}

	previous, err := s.users.SetQuota(ctx, req.UserID, req.RequestedBytes)
	if err != nil {
		s.reopen(ctx, req)
		if errors.Is(err, repository.ErrNotFound) {
			return model.QuotaRequest{}, ErrUserNotFound
		}
		return model.QuotaRequest{}, fmt.Errorf("odluka o zahtevu: postavljanje kvote: %w", err)
	}

	slog.Info("administratorska aktivnost",
		"action", "quota_request_approved",
		"adminId", adminID,
		"requestId", req.ID,
		"userId", req.UserID,
		"previousQuotaBytes", previous,
		"newQuotaBytes", req.RequestedBytes,
	)
	return req, nil
}

// Reject odbija zahtev: status postaje REJECTED, a kvota korisnika ostaje ista.
// Greške su iste kao kod Approve (osim ErrUserNotFound: kvota se ne dira, pa se
// zahtev može odbiti i kad je nalog obrisan). Odluka se loguje.
func (s *QuotaDecisionService) Reject(ctx context.Context, adminID, requestID string) (model.QuotaRequest, error) {
	if err := s.authorize(ctx, adminID, requestID); err != nil {
		return model.QuotaRequest{}, err
	}
	req, err := s.decide(ctx, requestID, model.QuotaRequestRejected)
	if err != nil {
		return model.QuotaRequest{}, err
	}

	slog.Info("administratorska aktivnost",
		"action", "quota_request_rejected",
		"adminId", adminID,
		"requestId", req.ID,
		"userId", req.UserID,
		"requestedBytes", req.RequestedBytes,
	)
	return req, nil
}

// authorize proverava administratora (u bazi) i oblik ID-ja zahteva.
func (s *QuotaDecisionService) authorize(ctx context.Context, adminID, requestID string) error {
	if err := requireActiveAdmin(ctx, s.users, adminID); err != nil {
		return err
	}
	if strings.TrimSpace(requestID) == "" || len(requestID) > maxQuotaRequestIDLen {
		return ErrQuotaRequestNotFound
	}
	return nil
}

func (s *QuotaDecisionService) decide(ctx context.Context, requestID string, status model.QuotaRequestStatus) (model.QuotaRequest, error) {
	req, err := s.requests.Decide(ctx, requestID, status, s.now())
	switch {
	case err == nil:
		return req, nil
	case errors.Is(err, repository.ErrQuotaRequestNotFound):
		return model.QuotaRequest{}, ErrQuotaRequestNotFound
	case errors.Is(err, repository.ErrQuotaRequestDecided):
		return model.QuotaRequest{}, ErrQuotaRequestDecided
	default:
		return model.QuotaRequest{}, fmt.Errorf("odluka o zahtevu: %w", err)
	}
}

// reopen vraća odobren zahtev na čekanje kad kvota nije mogla da se upiše. Radi i
// kad je zahtev otkazan (poseban kontekst). Ako ni to ne uspe, zahtev ostaje
// odobren bez promenjene kvote, što se mora videti u logu radi ručne popravke.
func (s *QuotaDecisionService) reopen(ctx context.Context, req model.QuotaRequest) {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.requests.Reopen(rctx, req.ID); err != nil {
		slog.Error("odluka o zahtevu: vracanje zahteva na cekanje nije uspelo, kvota nije postavljena",
			"requestId", req.ID, "userId", req.UserID, "err", err)
	}
}
