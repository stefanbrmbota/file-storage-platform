package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// fakeUsers pretends to be the Users service: it keeps one user's free space
// and answers reserve/release exactly like the contract in design 5.4 (S3).
type fakeUsers struct {
	mu        sync.Mutex
	free      int64
	lastPath  string
	lastBytes int64
}

const testToken = "test-internal-token-at-least-32-chars"

func (f *fakeUsers) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Like the real Users service: no valid internal token, no access.
	if r.Header.Get(InternalTokenHeader) != testToken {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var body struct{ Bytes int64 }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	f.lastPath, f.lastBytes = r.URL.Path, body.Bytes

	switch r.URL.Path {
	case "/internal/users/ana/quota/reserve":
		if body.Bytes > f.free {
			w.WriteHeader(http.StatusConflict)
			return
		}
		f.free -= body.Bytes
	case "/internal/users/ana/quota/release":
		f.free += body.Bytes
	default:
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func TestReserveAndRelease(t *testing.T) {
	users := &fakeUsers{free: 100}
	srv := httptest.NewServer(users)
	defer srv.Close()
	c := NewUsersClient(srv.URL, testToken, time.Second)
	ctx := context.Background()

	if err := c.Reserve(ctx, "ana", 60); err != nil {
		t.Fatalf("Reserve 60: %v", err)
	}
	if users.lastPath != "/internal/users/ana/quota/reserve" || users.lastBytes != 60 {
		t.Errorf("sent %s %d, want reserve 60", users.lastPath, users.lastBytes)
	}

	if err := c.Reserve(ctx, "ana", 50); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("Reserve 50 with 40 free: err = %v, want ErrQuotaExceeded", err)
	}

	if err := c.Release(ctx, "ana", 60); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if users.free != 100 {
		t.Errorf("free = %d after release, want 100", users.free)
	}
}

func TestWrongInternalTokenIsRefused(t *testing.T) {
	srv := httptest.NewServer(&fakeUsers{free: 100})
	defer srv.Close()

	err := NewUsersClient(srv.URL, "pogresan-token", time.Second).Reserve(context.Background(), "ana", 1)
	if !errors.Is(err, ErrUsersUnavailable) {
		t.Fatalf("wrong token: err = %v, want ErrUsersUnavailable", err)
	}
}

func TestUsersServiceDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := NewUsersClient(srv.URL, testToken, time.Second).Reserve(context.Background(), "ana", 1)
	if !errors.Is(err, ErrUsersUnavailable) {
		t.Fatalf("500 from Users: err = %v, want ErrUsersUnavailable", err)
	}

	srv.Close() // nothing listens on that address any more
	err = NewUsersClient(srv.URL, testToken, time.Second).Reserve(context.Background(), "ana", 1)
	if !errors.Is(err, ErrUsersUnavailable) {
		t.Fatalf("Users not running: err = %v, want ErrUsersUnavailable", err)
	}
}

func TestReserveTimesOut(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	}))
	defer slow.Close()

	start := time.Now()
	err := NewUsersClient(slow.URL, testToken, 100*time.Millisecond).Reserve(context.Background(), "ana", 1)
	if !errors.Is(err, ErrUsersUnavailable) {
		t.Fatalf("slow Users: err = %v, want ErrUsersUnavailable", err)
	}
	if time.Since(start) > time.Second {
		t.Errorf("call waited %v, the timeout should stop it after ~100ms", time.Since(start))
	}
}
