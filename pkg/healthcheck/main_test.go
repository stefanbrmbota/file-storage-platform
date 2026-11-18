package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func portOf(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	_, p, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if code := run([]string{portOf(t, srv)}); code != 0 {
		t.Fatalf("ocekivan 0, dobijen %d", code)
	}
}

func TestRunUnhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if code := run([]string{portOf(t, srv)}); code != 1 {
		t.Fatalf("ocekivan 1, dobijen %d", code)
	}
}

func TestRunNoServer(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	if code := run([]string{strconv.Itoa(port)}); code != 1 {
		t.Fatalf("ocekivan 1, dobijen %d", code)
	}
}

func TestRunBadArgs(t *testing.T) {
	for _, args := range [][]string{nil, {"abc"}, {"0"}, {"70000"}, {"1", "2"}} {
		if code := run(args); code != 1 {
			t.Fatalf("args %v: ocekivan 1, dobijen %d", args, code)
		}
	}
}
