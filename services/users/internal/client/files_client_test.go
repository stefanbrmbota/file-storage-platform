package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// testToken je INTERNAL_TOKEN koji klijent salje Files servisu (US-62).
const testToken = "test-internal-token-at-least-32-chars!"

func TestCreateRootFolder_Success(t *testing.T) {
	for _, status := range []int{http.StatusCreated, http.StatusOK} {
		var gotPath, gotUser, gotToken string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotToken = r.Header.Get("X-Internal-Token")
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			gotUser = body["userId"]
			w.WriteHeader(status)
		}))

		c := NewFilesClient(srv.URL+"/", testToken, time.Second)
		if err := c.CreateRootFolder(context.Background(), "u1"); err != nil {
			t.Fatalf("status %d: %v", status, err)
		}
		srv.Close()
		if gotPath != "/internal/folders/root" || gotUser != "u1" || gotToken != testToken {
			t.Errorf("status %d: path=%q userId=%q token poslat=%v", status, gotPath, gotUser, gotToken == testToken)
		}
	}
}

func TestCreateRootFolder_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewFilesClient(srv.URL, testToken, time.Second)
	if err := c.CreateRootFolder(context.Background(), "u1"); err == nil {
		t.Fatal("očekivana greška za status 500")
	}
}

func TestCreateRootFolder_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()

	c := NewFilesClient(srv.URL, testToken, 100*time.Millisecond)
	start := time.Now()
	if err := c.CreateRootFolder(context.Background(), "u1"); err == nil {
		t.Fatal("očekivana greška zbog timeout-a")
	}
	if time.Since(start) > time.Second {
		t.Errorf("timeout nije primenjen: %v", time.Since(start))
	}
}

func TestCreateRootFolder_Unreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	c := NewFilesClient(url, testToken, time.Second)
	if err := c.CreateRootFolder(context.Background(), "u1"); err == nil {
		t.Fatal("očekivana greška za nedostupan servis")
	}
}
