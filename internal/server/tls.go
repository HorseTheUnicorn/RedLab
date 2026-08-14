package server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func EnsureCertificate(directory, hostname string) (certFile, keyFile, fingerprint string, err error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", "", "", err
	}
	certFile, keyFile = filepath.Join(directory, "server.crt"), filepath.Join(directory, "server.key")
	// #nosec G304 -- both basenames are fixed and directory is the organizer-selected event data directory.
	if certPEM, readErr := os.ReadFile(certFile); readErr == nil {
		if _, keyErr := tls.LoadX509KeyPair(certFile, keyFile); keyErr == nil {
			if block, _ := pem.Decode(certPEM); block != nil {
				if certificate, parseErr := x509.ParseCertificate(block.Bytes); parseErr == nil && time.Now().After(certificate.NotBefore) && time.Now().Before(certificate.NotAfter) {
					if fingerprint := certificateFingerprint(certPEM); fingerprint != "" {
						_ = os.Chmod(keyFile, 0600)
						return certFile, keyFile, fingerprint, nil
					}
				}
			}
		}
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", "", err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		return "", "", "", err
	}
	now := time.Now().UTC()
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: hostname}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(365 * 24 * time.Hour), KeyUsage: x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true, DNSNames: []string{hostname, "localhost"}}
	if ip := net.ParseIP(hostname); ip != nil {
		template.IPAddresses = []net.IP{ip}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return "", "", "", err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := writeFileAtomic(certFile, certPEM, 0600); err != nil {
		return "", "", "", err
	}
	if err := writeFileAtomic(keyFile, keyPEM, 0600); err != nil {
		return "", "", "", err
	}
	if _, err := tls.LoadX509KeyPair(certFile, keyFile); err != nil {
		return "", "", "", err
	}
	return certFile, keyFile, certificateFingerprint(certPEM), nil
}

func certificateFingerprint(pemBytes []byte) string {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return ""
	}
	sum := sha256.Sum256(block.Bytes)
	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = hex.EncodeToString([]byte{b})
	}
	return strings.ToUpper(strings.Join(parts, ":"))
}
func (s *Server) ListenAndServeTLS(address, certFile, keyFile string) error {
	s.HTTP.Addr = address
	return s.HTTP.ListenAndServeTLS(certFile, keyFile)
}
func CertificateMessage(address, fingerprint string) string {
	return fmt.Sprintf("https://%s\nCA/server certificate SHA-256 fingerprint: %s", address, fingerprint)
}
