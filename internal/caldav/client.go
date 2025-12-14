package caldav

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/chrisbelyea/momentum/internal/models"
)

// Client represents a CalDAV client for external server connections
type Client struct {
	config     *models.BackendConfig
	httpClient *http.Client
}

// NewClient creates a new CalDAV client with the given configuration
func NewClient(config *models.BackendConfig) (*Client, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if config.URL == "" {
		return nil, fmt.Errorf("URL is required")
	}

	// Create TLS configuration
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: config.SkipTLSVerify,
	}

	// Load client certificates if provided
	if config.ClientCertPath != "" && config.ClientKeyPath != "" {
		cert, err := tls.LoadX509KeyPair(config.ClientCertPath, config.ClientKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// Create HTTP client with TLS and timeout configuration
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
			// Security best practices
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	return &Client{
		config:     config,
		httpClient: httpClient,
	}, nil
}

// ValidateConnection tests the connection to the CalDAV server
func (c *Client) ValidateConnection() error {
	// Create a PROPFIND request to test the connection
	req, err := http.NewRequest("PROPFIND", c.config.URL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set required headers
	req.Header.Set("Depth", "0")
	req.Header.Set("Content-Type", "application/xml")

	// Add authentication
	if c.config.Username != "" && c.config.Password != "" {
		req.SetBasicAuth(c.config.Username, c.config.Password)
	}

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	// Read and discard body to allow connection reuse
	_, _ = io.Copy(io.Discard, resp.Body)

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("connection validation failed with status %d", resp.StatusCode)
	}

	return nil
}

// Close closes the client and releases resources
func (c *Client) Close() {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
}

// LoadClientCertificate loads a client certificate from file
func LoadClientCertificate(certPath, keyPath string) (tls.Certificate, error) {
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to read certificate: %w", err)
	}

	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to read key: %w", err)
	}

	cert, err := tls.X509KeyPair(certData, keyData)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to parse certificate: %w", err)
	}

	return cert, nil
}

// ValidateCertificate validates a certificate against the system CA pool
func ValidateCertificate(certPath string) error {
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("failed to read certificate: %w", err)
	}

	block, _ := pem.Decode(certData)
	if block == nil {
		return fmt.Errorf("failed to parse certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Get system cert pool
	roots, err := x509.SystemCertPool()
	if err != nil {
		return fmt.Errorf("failed to load system CA pool: %w", err)
	}

	// Verify certificate
	opts := x509.VerifyOptions{
		Roots:       roots,
		CurrentTime: time.Now(),
	}

	if _, err := cert.Verify(opts); err != nil {
		return fmt.Errorf("certificate verification failed: %w", err)
	}

	return nil
}
