package crypto

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetOrCreateDevCert_NewCert(t *testing.T) {
	certDir := t.TempDir()

	certFile, keyFile, err := GetOrCreateDevCert(certDir)
	if err != nil {
		t.Fatalf("GetOrCreateDevCert() error = %v", err)
	}

	// Verify the returned paths are inside certDir.
	if filepath.Dir(certFile) != certDir {
		t.Errorf("certFile %q is not inside certDir %q", certFile, certDir)
	}
	if filepath.Dir(keyFile) != certDir {
		t.Errorf("keyFile %q is not inside certDir %q", keyFile, certDir)
	}

	// Both files must exist.
	if !fileExists(certFile) {
		t.Errorf("certificate file %q does not exist", certFile)
	}
	if !fileExists(keyFile) {
		t.Errorf("key file %q does not exist", keyFile)
	}
}

func TestGetOrCreateDevCert_ReuseExisting(t *testing.T) {
	certDir := t.TempDir()

	// First call creates the cert.
	certFile1, keyFile1, err := GetOrCreateDevCert(certDir)
	if err != nil {
		t.Fatalf("first GetOrCreateDevCert() error = %v", err)
	}

	// Read the original cert bytes.
	certBytes1, err := os.ReadFile(certFile1)
	if err != nil {
		t.Fatalf("failed to read cert file: %v", err)
	}

	// Second call should reuse the same files.
	certFile2, keyFile2, err := GetOrCreateDevCert(certDir)
	if err != nil {
		t.Fatalf("second GetOrCreateDevCert() error = %v", err)
	}

	if certFile1 != certFile2 {
		t.Errorf("cert file path changed: %q -> %q", certFile1, certFile2)
	}
	if keyFile1 != keyFile2 {
		t.Errorf("key file path changed: %q -> %q", keyFile1, keyFile2)
	}

	// The cert file should not have been regenerated.
	certBytes2, err := os.ReadFile(certFile2)
	if err != nil {
		t.Fatalf("failed to read cert file on second call: %v", err)
	}
	if string(certBytes1) != string(certBytes2) {
		t.Error("certificate was regenerated on second call; it should have been reused")
	}
}

func TestGetOrCreateDevCert_CertIsValid(t *testing.T) {
	certDir := t.TempDir()

	certFile, keyFile, err := GetOrCreateDevCert(certDir)
	if err != nil {
		t.Fatalf("GetOrCreateDevCert() error = %v", err)
	}

	// Load the certificate as a TLS keypair — this validates PEM encoding and key match.
	_, err = tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatalf("tls.LoadX509KeyPair() failed: %v", err)
	}

	// Parse the certificate for detailed checks.
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatalf("failed to read cert file: %v", err)
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("failed to decode PEM block from cert file")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("x509.ParseCertificate() failed: %v", err)
	}

	// Check Subject fields.
	if cert.Subject.CommonName != "localhost" {
		t.Errorf("CommonName = %q, want %q", cert.Subject.CommonName, "localhost")
	}
	if len(cert.Subject.Organization) == 0 || cert.Subject.Organization[0] != "Momentum Development" {
		t.Errorf("Organization = %v, want [\"Momentum Development\"]", cert.Subject.Organization)
	}

	// Check SANs.
	hasDNS := false
	for _, d := range cert.DNSNames {
		if d == "localhost" {
			hasDNS = true
		}
	}
	if !hasDNS {
		t.Error("certificate does not have localhost in DNSNames")
	}

	hasIPv4, hasIPv6 := false, false
	for _, ip := range cert.IPAddresses {
		if ip.Equal(parseIP("127.0.0.1")) {
			hasIPv4 = true
		}
		if ip.Equal(parseIP("::1")) {
			hasIPv6 = true
		}
	}
	if !hasIPv4 {
		t.Error("certificate does not include 127.0.0.1 in IPAddresses")
	}
	if !hasIPv6 {
		t.Error("certificate does not include ::1 in IPAddresses")
	}

	// Validity period should be approximately one year.
	duration := cert.NotAfter.Sub(cert.NotBefore)
	if duration < 364*24*time.Hour || duration > 366*24*time.Hour {
		t.Errorf("certificate validity duration = %v, want ~365 days", duration)
	}

	// Extended key usage must include server authentication.
	hasServerAuth := false
	for _, eku := range cert.ExtKeyUsage {
		if eku == x509.ExtKeyUsageServerAuth {
			hasServerAuth = true
		}
	}
	if !hasServerAuth {
		t.Error("certificate does not have ExtKeyUsageServerAuth")
	}
}

func TestGetOrCreateDevCert_KeyPermissions(t *testing.T) {
	certDir := t.TempDir()

	_, keyFile, err := GetOrCreateDevCert(certDir)
	if err != nil {
		t.Fatalf("GetOrCreateDevCert() error = %v", err)
	}

	info, err := os.Stat(keyFile)
	if err != nil {
		t.Fatalf("os.Stat() failed: %v", err)
	}

	// Private key must not be world- or group-readable.
	if info.Mode().Perm()&0077 != 0 {
		t.Errorf("key file permissions = %o, want 0600 (no group/other access)", info.Mode().Perm())
	}
}

func TestDevCertDir_HasFallback(t *testing.T) {
	// DevCertDir() should always return a non-empty path.
	dir := DevCertDir()
	if dir == "" {
		t.Error("DevCertDir() returned empty string")
	}
}

// parseIP is a helper that panics on invalid IP strings (test-only).
func parseIP(s string) net.IP {
	ip := net.ParseIP(s)
	if ip == nil {
		panic("invalid IP: " + s)
	}
	return ip
}
