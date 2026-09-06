package caldav

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
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
	target, err := validateTargetURL(config.URL, config.SkipTLSVerify)
	if err != nil {
		return nil, err
	}
	// Skipping certificate verification is deliberately limited to loopback
	// development targets. It must never be a way to disable TLS verification
	// for a remotely reachable backend.
	if config.SkipTLSVerify && !isLoopbackHost(target.Hostname()) {
		return nil, fmt.Errorf("skip_tls_verify is only permitted for loopback development targets")
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
	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
		DialContext:     safeDialContext(config.SkipTLSVerify),
		// Security best practices
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	httpClient := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
		// CalDAV validation must not follow a redirect to a different host (or
		// downgrade to HTTP), where credentials could be disclosed.
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return fmt.Errorf("redirects are not permitted for CalDAV validation")
		},
	}

	return &Client{
		config:     config,
		httpClient: httpClient,
	}, nil
}

// ValidateConnection tests the connection to the CalDAV server
func (c *Client) ValidateConnection() error {
	if _, err := validateTargetURL(c.config.URL, c.config.SkipTLSVerify); err != nil {
		return err
	}
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
	// A validation response is not expected to contain a large body. Bound the
	// drain so a malicious endpoint cannot consume unbounded memory/bandwidth.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("connection validation failed with status %d", resp.StatusCode)
	}

	return nil
}

// validateTargetURL applies the outbound CalDAV SSRF policy before any
// credentials are attached to a request. DNS is checked again by
// safeDialContext immediately before dialing to reduce DNS-rebinding risk.
func validateTargetURL(raw string, allowLocal bool) (*url.URL, error) {
	u, err := url.ParseRequestURI(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("CalDAV URL must be an HTTPS URL without embedded credentials")
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	p, err := strconv.Atoi(port)
	if err != nil || (p != 443 && !(allowLocal && isLoopbackHost(u.Hostname()))) {
		return nil, fmt.Errorf("CalDAV URL must use port 443")
	}
	if isBlockedHost(u.Hostname()) && !(allowLocal && isLoopbackHost(u.Hostname())) {
		return nil, fmt.Errorf("CalDAV URL targets a loopback, private, link-local, or otherwise blocked address")
	}
	return u, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isBlockedHost(host string) bool {
	if isLoopbackHost(host) {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast()
}

func safeDialContext(allowLocal bool) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid CalDAV address: %w", err)
		}
		if p, err := strconv.Atoi(port); err != nil || (p != 443 && !(allowLocal && isLoopbackHost(host))) {
			return nil, fmt.Errorf("CalDAV destination port is not allowed")
		}
		ips, err := net.LookupIP(host)
		if err != nil {
			return nil, fmt.Errorf("CalDAV destination DNS lookup failed: %w", err)
		}
		for _, ip := range ips {
			if isBlockedHost(ip.String()) && !(allowLocal && ip.IsLoopback()) {
				return nil, fmt.Errorf("CalDAV destination resolved to a blocked address")
			}
		}
		// Dial only one of the already-validated answers. The hostname remains in
		// the URL for TLS SNI/hostname verification, while the socket uses the
		// validated address to close the DNS-rebinding window.
		for _, ip := range ips {
			conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
		}
		return nil, fmt.Errorf("CalDAV destination resolved only to blocked or unreachable addresses")
	}
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
