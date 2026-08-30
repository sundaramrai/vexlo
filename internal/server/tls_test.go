package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTestCertificate(t *testing.T, dir, name string) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: name},
		DNSNames:     []string{name},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
	}, &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: name},
		DNSNames:     []string{name},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
	}, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certFile := filepath.Join(dir, name+".crt")
	keyFile := filepath.Join(dir, name+".key")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}

func TestTunnelTLSConfigLoadsPrimaryAndExtraCertificatePairs(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeTestCertificate(t, dir, "vexlo.example.test")
	extraCertFile, extraKeyFile := writeTestCertificate(t, dir, "wildcard.example.test")
	s := &Server{cfg: Config{
		TLSCertFile:      certFile,
		TLSKeyFile:       keyFile,
		TLSExtraCertFile: extraCertFile,
		TLSExtraKeyFile:  extraKeyFile,
	}}

	config, err := s.tunnelTLSConfig()
	if err != nil {
		t.Fatalf("load certificate pairs: %v", err)
	}
	if len(config.Certificates) != 2 {
		t.Fatalf("loaded %d certificate pairs, want 2", len(config.Certificates))
	}
}

func TestTunnelTLSConfigRejectsIncompleteExtraCertificatePair(t *testing.T) {
	s := &Server{cfg: Config{EnableTLS: true, TLSExtraCertFile: "certificate.pem"}}
	if _, err := s.tunnelTLSConfig(); err == nil {
		t.Fatal("expected incomplete extra certificate pair to fail")
	}
}
