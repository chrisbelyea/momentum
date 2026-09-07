package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

const (
	devCertFilename = "dev-cert.pem"
	devKeyFilename  = "dev-key.pem"
)

// DevCertDir returns the default directory for auto-generated development certificates.
// It uses ~/.momentum/dev-certs/ with a fallback to ./dev-certs/ if the home directory
// cannot be determined.
func DevCertDir() string {
	if dir := os.Getenv("MOMENTUM_DEV_CERT_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".momentum", "dev-certs")
	}
	return filepath.Join(home, ".momentum", "dev-certs")
}

// GetOrCreateDevCert returns paths to a self-signed development TLS certificate and
// private key, creating them if they do not already exist in certDir. The certificate
// is valid for localhost, 127.0.0.1, and ::1 for one year and uses an ECDSA P-256 key.
//
// certDir is typically the value returned by DevCertDir().
func GetOrCreateDevCert(certDir string) (certFile, keyFile string, err error) {
	certFile = filepath.Join(certDir, devCertFilename)
	keyFile = filepath.Join(certDir, devKeyFilename)

	// Reuse existing certificate if both files are present.
	if fileExists(certFile) && fileExists(keyFile) {
		return certFile, keyFile, nil
	}

	log.Println("Generating self-signed development certificate...")

	// Generate a new ECDSA P-256 private key.
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate private key: %w", err)
	}

	// Build the certificate template.
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", fmt.Errorf("failed to generate serial number: %w", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Momentum Development"},
			CommonName:   "localhost",
		},
		NotBefore:             now,
		NotAfter:              now.Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses: []net.IP{
			net.ParseIP("127.0.0.1"),
			net.ParseIP("::1"),
		},
	}

	// Self-sign the certificate.
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to create certificate: %w", err)
	}

	// Ensure the certificate directory exists with restricted permissions.
	if err := os.MkdirAll(certDir, 0700); err != nil {
		return "", "", fmt.Errorf("failed to create certificate directory %s: %w", certDir, err)
	}

	// Write the certificate PEM file.
	if err := writePEMFile(certFile, "CERTIFICATE", certDER, 0644); err != nil {
		return "", "", fmt.Errorf("failed to write certificate file: %w", err)
	}

	// Marshal and write the private key PEM file.
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal private key: %w", err)
	}
	if err := writePEMFile(keyFile, "EC PRIVATE KEY", keyDER, 0600); err != nil {
		return "", "", fmt.Errorf("failed to write key file: %w", err)
	}

	return certFile, keyFile, nil
}

// writePEMFile encodes derBytes as a PEM block with the given pemType and writes it
// to path with the specified permissions.
func writePEMFile(path, pemType string, derBytes []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: pemType, Bytes: derBytes})
}

// fileExists returns true if path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
