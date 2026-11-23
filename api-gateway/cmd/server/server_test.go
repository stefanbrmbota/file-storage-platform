package main

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/config"
)

// Kratki timeout-i da test traje malo; prenos namerno traje mnogo duze od njih.
const (
	testHeaderTimeout = 100 * time.Millisecond
	transferDuration  = 8 * testHeaderTimeout
)

func startServer(t *testing.T, h http.Handler) string {
	t.Helper()
	cfg := config.Config{ReadHeaderTimeout: testHeaderTimeout, IdleTimeout: time.Second}
	srv := newServer(cfg, h)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return "http://" + ln.Addr().String()
}

// slowReader daje podatke u delovima sa pauzom, kao spor upload.
type slowReader struct {
	chunks int
	pause  time.Duration
}

func (r *slowReader) Read(p []byte) (int, error) {
	if r.chunks == 0 {
		return 0, io.EOF
	}
	time.Sleep(r.pause)
	r.chunks--
	return copy(p, "deo-datoteke;"), nil
}

func TestLongDownloadIsNotCut(t *testing.T) {
	const chunks = 8
	url := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rc := http.NewResponseController(w)
		for i := 0; i < chunks; i++ {
			_, _ = io.WriteString(w, "deo-datoteke;")
			_ = rc.Flush()
			time.Sleep(transferDuration / chunks)
		}
	}))

	start := time.Now()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("preuzimanje prekinuto posle %v: %v", time.Since(start), err)
	}
	if got := strings.Count(string(body), "deo-datoteke;"); got != chunks {
		t.Errorf("stiglo %d od %d delova", got, chunks)
	}
}

func TestLongUploadIsNotCut(t *testing.T) {
	url := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "prekinuto", http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, string(rune('0'+strings.Count(string(b), "deo-datoteke;"))))
	}))

	body := &slowReader{chunks: 8, pause: transferDuration / 8}
	resp, err := http.Post(url, "application/octet-stream", body)
	if err != nil {
		t.Fatalf("otpremanje prekinuto: %v", err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(got) != "8" {
		t.Errorf("status = %d, server primio %q delova", resp.StatusCode, got)
	}
}

func TestSlowHeadersAreRejected(t *testing.T) {
	url := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("zahtev bez celih zaglavlja ne sme stici do handlera")
	}))

	conn, err := net.Dial("tcp", strings.TrimPrefix(url, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// Pocetak zahteva, ali zaglavlja se nikad ne zavrse (slowloris).
	if _, err := io.WriteString(conn, "GET / HTTP/1.1\r\nHost: x\r\n"); err != nil {
		t.Fatal(err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(10 * testHeaderTimeout))
	start := time.Now()
	line, err := bufio.NewReader(conn).ReadString('\n')
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatal("server nije zatvorio vezu sporog klijenta")
	}
	if strings.HasPrefix(line, "HTTP/1.1 200") {
		t.Fatalf("spor klijent dobio odgovor: %q", line)
	}
	if elapsed := time.Since(start); elapsed > 5*testHeaderTimeout {
		t.Errorf("veza zatvorena tek posle %v", elapsed)
	}
}
