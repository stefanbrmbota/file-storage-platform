// Program devcert pravi sertifikate za HTTPS u lokalnom razvoju (US-67, T-67.1).
//
// Pravi dva para kljuc/sertifikat u folderu certs/ (podrazumevano ../certs,
// tj. certs/ u root-u repoa kad se pokrene iz api-gateway):
//
//   - ca.crt / ca.key: nas lokalni sertifikacioni autoritet (CA). Samo njega
//     treba jednom dodati u poverene sertifikate na racunaru; tada pretrazivac
//     veruje svakom sertifikatu koji je on potpisao.
//   - localhost.crt / localhost.key: sertifikat servera za localhost,
//     127.0.0.1 i ::1, potpisan nasim CA. Koriste ga klijent (nginx) i gateway.
//
// Kljucevi su ECDSA P-256. Privatni kljucevi se nikad ne commituju (certs/ je
// u .gitignore). Postojeci fajlovi se ne prepisuju bez -force.
//
// Upotreba (iz foldera api-gateway):
//
//	go run ./cmd/devcert
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

func main() {
	out := flag.String("out", filepath.Join("..", "certs"), "folder za sertifikate")
	force := flag.Bool("force", false, "prepisi postojece sertifikate")
	flag.Parse()

	if err := run(*out, *force, time.Now()); err != nil {
		fmt.Fprintln(os.Stderr, "devcert:", err)
		os.Exit(1)
	}
	fmt.Println("Sertifikati su u", *out)
	fmt.Println("Jednom dodaj CA u poverene sertifikate (Windows, bez admin prava):")
	fmt.Println("  certutil -user -addstore Root", filepath.Join(*out, "ca.crt"))
}

func run(dir string, force bool, now time.Time) error {
	if !force {
		if _, err := os.Stat(filepath.Join(dir, "localhost.crt")); err == nil {
			return errors.New("sertifikati vec postoje (za nove pokreni sa -force)")
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// 1. Lokalni CA.
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: "File Storage Platform - lokalni razvojni CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(2, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true, // CA potpisuje samo serverske sertifikate, ne druge CA
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		return err
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return err
	}

	// 2. Sertifikat servera za localhost, potpisan CA-om.
	srvKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	srvTmpl := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.IPv6loopback},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.AddDate(0, 0, 397), // pretrazivaci ne prihvataju duze od 398 dana
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	srvDER, err := x509.CreateCertificate(rand.Reader, srvTmpl, caCert, &srvKey.PublicKey, caKey)
	if err != nil {
		return err
	}

	if err := writePEM(filepath.Join(dir, "ca.crt"), "CERTIFICATE", caDER, 0o644); err != nil {
		return err
	}
	if err := writeKey(filepath.Join(dir, "ca.key"), caKey); err != nil {
		return err
	}
	if err := writePEM(filepath.Join(dir, "localhost.crt"), "CERTIFICATE", srvDER, 0o644); err != nil {
		return err
	}
	return writeKey(filepath.Join(dir, "localhost.key"), srvKey)
}

func serial() *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		panic(err)
	}
	return n
}

func writeKey(path string, key *ecdsa.PrivateKey) error {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	// 0644 a ne 0600: kontejneri (nginx, gateway) rade kao drugi korisnik i
	// moraju da procitaju kljuc preko bind mount-a. Kljuc vazi samo za localhost.
	return writePEM(path, "PRIVATE KEY", der, 0o644)
}

func writePEM(path, typ string, der []byte, mode os.FileMode) error {
	return os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}), mode)
}
