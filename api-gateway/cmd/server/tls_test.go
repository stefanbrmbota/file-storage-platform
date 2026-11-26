package main

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Server sa nasim TLS podesavanjima odbija stare verzije protokola.
func TestTLSRejectsOldVersions(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	srv.TLS = tlsConfig()
	srv.StartTLS()
	defer srv.Close()

	old := srv.Client()
	old.Transport.(*http.Transport).TLSClientConfig.MaxVersion = tls.VersionTLS11
	if _, err := old.Get(srv.URL); err == nil {
		t.Fatal("TLS 1.1 je prihvacen")
	}

	ok := srv.Client()
	ok.Transport.(*http.Transport).TLSClientConfig.MaxVersion = 0
	ok.Transport.(*http.Transport).TLSClientConfig.MinVersion = tls.VersionTLS12
	resp, err := ok.Get(srv.URL)
	if err != nil {
		t.Fatalf("TLS 1.2+ odbijen: %v", err)
	}
	resp.Body.Close()
}

// Obican HTTP zahtev na TLS port ne prolazi.
func TestPlainHTTPToTLSPortFails(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	srv.TLS = tlsConfig()
	srv.StartTLS()
	defer srv.Close()

	resp, err := http.Get("http://" + srv.Listener.Addr().String())
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode < 400 {
			t.Fatalf("HTTP zahtev prosao sa statusom %d", resp.StatusCode)
		}
	}
}
