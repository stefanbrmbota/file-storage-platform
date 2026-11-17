package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

// --- lažne zavisnosti ---

type fakeIssuer struct{ err error }

func (f *fakeIssuer) Issue(context.Context, model.User) error { return f.err }

type fakeTokens struct {
	mu   sync.Mutex
	data map[string]model.ConfirmationToken
}

func newFakeTokens() *fakeTokens { return &fakeTokens{data: map[string]model.ConfirmationToken{}} }

func (f *fakeTokens) Create(_ context.Context, t model.ConfirmationToken) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[t.ID] = t
	return nil
}

func (f *fakeTokens) Consume(_ context.Context, hash string, now time.Time) (model.ConfirmationToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.data[hash]
	if !ok || !t.ExpiresAt.After(now) {
		return model.ConfirmationToken{}, repository.ErrTokenNotFound
	}
	delete(f.data, hash)
	return t, nil
}

type fakeMailer struct {
	err              error
	to, subject, msg string
	calls            int
}

func (f *fakeMailer) Send(_ context.Context, to, subject, body string) error {
	f.calls++
	f.to, f.subject, f.msg = to, subject, body
	return f.err
}

type fakeFiles struct {
	err   error
	calls []string
}

func (f *fakeFiles) CreateRootFolder(_ context.Context, userID string) error {
	f.calls = append(f.calls, userID)
	return f.err
}

type confirmEnv struct {
	repo   *fakeRepo
	tokens *fakeTokens
	mailer *fakeMailer
	files  *fakeFiles
	svc    *ConfirmationService
	user   model.User
	now    time.Time
}

func newConfirmEnv(t *testing.T) *confirmEnv {
	t.Helper()
	e := &confirmEnv{
		repo:   newFakeRepo(),
		tokens: newFakeTokens(),
		mailer: &fakeMailer{},
		files:  &fakeFiles{},
		now:    time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC),
	}
	e.svc = NewConfirmationService(e.repo, e.tokens, e.mailer, e.files, 24*time.Hour, "http://klijent.local/confirm")
	e.svc.now = func() time.Time { return e.now }
	e.user = model.NewUser("u1", "Pera", "Perić", "pera@example.com", "hash", 1<<30, e.now)
	e.repo.users[e.user.Email] = e.user
	return e
}

// issueAndGetToken izdaje token i vadi ga iz linka u poslatom mejlu.
func (e *confirmEnv) issueAndGetToken(t *testing.T) string {
	t.Helper()
	if err := e.svc.Issue(context.Background(), e.user); err != nil {
		t.Fatal(err)
	}
	_, tok, ok := strings.Cut(e.mailer.msg, "#token=")
	if !ok {
		t.Fatalf("mejl ne sadrži link sa tokenom: %q", e.mailer.msg)
	}
	return strings.Fields(tok)[0]
}

// --- Issue ---

func TestIssue_SendsLinkAndStoresOnlyHash(t *testing.T) {
	e := newConfirmEnv(t)
	raw := e.issueAndGetToken(t)

	if e.mailer.to != "pera@example.com" {
		t.Errorf("mejl poslat na %q", e.mailer.to)
	}
	if !strings.Contains(e.mailer.msg, "http://klijent.local/confirm#token="+raw) {
		t.Error("link treba da vodi na klijentsku stranicu sa tokenom u fragmentu")
	}
	if strings.Contains(e.mailer.msg, "?token=") {
		t.Error("token ne sme biti u query delu URL-a")
	}
	if len(raw) < 40 {
		t.Errorf("token je prekratak: %d", len(raw))
	}
	if len(e.tokens.data) != 1 {
		t.Fatalf("upisano %d tokena", len(e.tokens.data))
	}
	for id, tok := range e.tokens.data {
		if id == raw || strings.Contains(id, raw) {
			t.Fatal("u bazi je sirov token umesto heša")
		}
		if id != hashToken(raw) || tok.UserID != "u1" {
			t.Errorf("neočekivan zapis: %+v", tok)
		}
		if !tok.ExpiresAt.Equal(e.now.Add(24 * time.Hour)) {
			t.Errorf("rok = %v", tok.ExpiresAt)
		}
	}
}

func TestIssue_TokensAreUnique(t *testing.T) {
	e := newConfirmEnv(t)
	a, b := e.issueAndGetToken(t), e.issueAndGetToken(t)
	if a == b {
		t.Fatal("dva izdata tokena su ista")
	}
}

func TestIssue_MailerFailureIsError(t *testing.T) {
	e := newConfirmEnv(t)
	e.mailer.err = errors.New("smtp pao")
	if err := e.svc.Issue(context.Background(), e.user); err == nil {
		t.Fatal("očekivana greška kad slanje mejla ne uspe")
	}
}

// --- Confirm ---

func TestConfirm_ActivatesAndCreatesRoot(t *testing.T) {
	e := newConfirmEnv(t)
	raw := e.issueAndGetToken(t)

	if err := e.svc.Confirm(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	if !e.repo.users["pera@example.com"].Active {
		t.Error("nalog nije aktiviran")
	}
	if len(e.files.calls) != 1 || e.files.calls[0] != "u1" {
		t.Errorf("Files pozivi: %v", e.files.calls)
	}
}

func TestConfirm_SecondUseFails(t *testing.T) {
	e := newConfirmEnv(t)
	raw := e.issueAndGetToken(t)
	if err := e.svc.Confirm(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Confirm(context.Background(), raw); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, očekivano ErrInvalidToken", err)
	}
	if len(e.files.calls) != 1 {
		t.Errorf("Files pozvan %d puta, očekivano 1", len(e.files.calls))
	}
}

func TestConfirm_ExpiredTokenFails(t *testing.T) {
	e := newConfirmEnv(t)
	raw := e.issueAndGetToken(t)
	e.now = e.now.Add(24*time.Hour + time.Second)

	if err := e.svc.Confirm(context.Background(), raw); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, očekivano ErrInvalidToken", err)
	}
	if e.repo.users["pera@example.com"].Active {
		t.Error("istekao token ne sme aktivirati nalog")
	}
	if len(e.files.calls) != 0 {
		t.Error("Files ne sme biti pozvan za istekao token")
	}
}

func TestConfirm_UnknownOrMalformedToken(t *testing.T) {
	e := newConfirmEnv(t)
	for _, tok := range []string{"", "   ", "nepostojeci", strings.Repeat("a", 500)} {
		if err := e.svc.Confirm(context.Background(), tok); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("token %.10q: err = %v, očekivano ErrInvalidToken", tok, err)
		}
	}
}

func TestConfirm_FilesFailureRestoresToken(t *testing.T) {
	e := newConfirmEnv(t)
	raw := e.issueAndGetToken(t)
	e.files.err = errors.New("files je pao")

	err := e.svc.Confirm(context.Background(), raw)
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, očekivano ErrUpstream", err)
	}
	if e.repo.users["pera@example.com"].Active {
		t.Error("nalog ne sme biti aktivan ako Files nije uspeo")
	}

	// Ponovni pokušaj sa istim linkom radi kad se Files oporavi.
	e.files.err = nil
	if err := e.svc.Confirm(context.Background(), raw); err != nil {
		t.Fatalf("ponovni pokušaj: %v", err)
	}
	if !e.repo.users["pera@example.com"].Active {
		t.Error("nalog nije aktiviran posle ponovnog pokušaja")
	}
}

func TestConfirm_DeletedUserIsInvalid(t *testing.T) {
	e := newConfirmEnv(t)
	raw := e.issueAndGetToken(t)
	delete(e.repo.users, "pera@example.com")

	if err := e.svc.Confirm(context.Background(), raw); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, očekivano ErrInvalidToken", err)
	}
}

func TestConfirm_ConcurrentClicksOnlyOneWins(t *testing.T) {
	e := newConfirmEnv(t)
	raw := e.issueAndGetToken(t)

	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e.svc.Confirm(context.Background(), raw) == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("uspešnih potvrda: %d, očekivano 1", ok)
	}
}
