package server

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureCertificate(t *testing.T) {
	cert, key, fingerprint, err := EnsureCertificate(t.TempDir(), "localhost")
	if err != nil {
		t.Fatal(err)
	}
	if fingerprint == "" {
		t.Fatal("empty certificate fingerprint")
	}
	certBytes, err := os.ReadFile(cert)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(certBytes)
	if block == nil {
		t.Fatal("certificate is not PEM")
	}
	if _, err := x509.ParseCertificate(block.Bytes); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(key); err != nil {
		t.Fatal(err)
	}
	cert2, key2, fingerprint2, err := EnsureCertificate(filepath.Dir(cert), "localhost")
	if err != nil {
		t.Fatal(err)
	}
	if cert2 != cert || key2 != key || fingerprint2 != fingerprint {
		t.Fatal("certificate was not reused")
	}
}
