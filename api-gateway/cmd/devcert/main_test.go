package main

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCertificatesAreValidForLocalhost(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	if err := run(dir, false, now); err != nil {
		t.Fatal(err)
	}

	pair, err := tls.LoadX509KeyPair(filepath.Join(dir, "localhost.crt"), filepath.Join(dir, "localhost.key"))
	if err != nil {
		t.Fatalf("kljuc i sertifikat se ne slazu: %v", err)
	}
	leaf, _ := x509.ParseCertificate(pair.Certificate[0])

	caPEM, _ := os.ReadFile(filepath.Join(dir, "ca.crt"))
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		t.Fatal("ca.crt nije ispravan")
	}
	for _, host := range []string{"localhost", "127.0.0.1", "::1"} {
		if _, err := leaf.Verify(x509.VerifyOptions{DNSName: host, Roots: roots, CurrentTime: now}); err != nil {
			t.Errorf("%s: %v", host, err)
		}
	}
	if _, err := leaf.Verify(x509.VerifyOptions{DNSName: "example.com", Roots: roots, CurrentTime: now}); err == nil {
		t.Error("sertifikat ne sme da vazi za druge domene")
	}
	if leaf.NotAfter.Sub(now) > 398*24*time.Hour {
		t.Errorf("sertifikat vazi predugo: %v", leaf.NotAfter)
	}
}

func TestDoesNotOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	if err := run(dir, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "localhost.key"))
	if err := run(dir, false, time.Now()); err == nil {
		t.Fatal("ocekivana greska kad sertifikati vec postoje")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "localhost.key"))
	if string(before) != string(after) {
		t.Fatal("kljuc je prepisan bez -force")
	}
	if err := run(dir, true, time.Now()); err != nil {
		t.Fatalf("-force: %v", err)
	}
}
