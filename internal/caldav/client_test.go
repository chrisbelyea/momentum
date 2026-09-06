package caldav

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chrisbelyea/momentum/internal/models"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		config  *models.BackendConfig
		wantErr bool
	}{
		{
			name: "valid config with basic auth",
			config: &models.BackendConfig{
				URL:      "https://caldav.example.com",
				Username: "user",
				Password: "pass",
			},
			wantErr: false,
		},
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "empty URL",
			config: &models.BackendConfig{
				Username: "user",
				Password: "pass",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && client == nil {
				t.Error("Expected client to be created")
			}
			if client != nil {
				client.Close()
			}
		})
	}
}

func TestClient_ValidateConnection(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse int
		wantErr        bool
	}{
		{
			name:           "successful connection",
			serverResponse: http.StatusOK,
			wantErr:        false,
		},
		{
			name:           "multi-status response",
			serverResponse: http.StatusMultiStatus,
			wantErr:        false,
		},
		{
			name:           "unauthorized",
			serverResponse: http.StatusUnauthorized,
			wantErr:        true,
		},
		{
			name:           "not found",
			serverResponse: http.StatusNotFound,
			wantErr:        true,
		},
		{
			name:           "server error",
			serverResponse: http.StatusInternalServerError,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and headers
				if r.Method != "PROPFIND" {
					t.Errorf("Expected PROPFIND request, got %s", r.Method)
				}
				if r.Header.Get("Depth") != "0" {
					t.Errorf("Expected Depth header to be 0, got %s", r.Header.Get("Depth"))
				}

				// Check authentication
				username, password, ok := r.BasicAuth()
				if ok {
					if username != "testuser" || password != "testpass" {
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
				}

				w.WriteHeader(tt.serverResponse)
			}))
			defer server.Close()

			// Create client with test server URL
			config := &models.BackendConfig{
				URL:           server.URL,
				Username:      "testuser",
				Password:      "testpass",
				SkipTLSVerify: true, // Skip verification for test server
			}

			client, err := NewClient(config)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			defer client.Close()

			// Validate connection
			err = client.ValidateConnection()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConnection() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClient_ValidateConnectionWithoutAuth(t *testing.T) {
	// Create a test server that doesn't require auth
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := &models.BackendConfig{
		URL:           server.URL,
		SkipTLSVerify: true,
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// This should succeed without authentication
	err = client.ValidateConnection()
	if err != nil {
		t.Errorf("ValidateConnection() should succeed without auth: %v", err)
	}
}

func TestClient_ValidateConnectionInvalidURL(t *testing.T) {
	config := &models.BackendConfig{
		URL:      "http://invalid-domain-that-does-not-exist.example.invalid",
		Username: "user",
		Password: "pass",
	}

	if _, err := NewClient(config); err == nil {
		t.Error("Expected invalid HTTP URL to be rejected before dialing")
	}
}

func TestClient_Close(t *testing.T) {
	config := &models.BackendConfig{
		URL:      "https://caldav.example.com",
		Username: "user",
		Password: "pass",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Close should not panic
	client.Close()
	client.Close() // Should be safe to call multiple times
}

func TestClient_TLSConfiguration(t *testing.T) {
	tests := []struct {
		name          string
		skipTLSVerify bool
	}{
		{
			name:          "with TLS verification",
			skipTLSVerify: false,
		},
		{
			name:          "without TLS verification",
			skipTLSVerify: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "https://caldav.example.com"
			if tt.skipTLSVerify {
				url = "https://localhost/tasks"
			}
			config := &models.BackendConfig{
				URL:           url,
				Username:      "user",
				Password:      "pass",
				SkipTLSVerify: tt.skipTLSVerify,
			}

			client, err := NewClient(config)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			defer client.Close()

			if client.httpClient.Transport == nil {
				t.Error("Expected HTTP transport to be configured")
			}
		})
	}
}

func TestValidateTargetURLRejectsSSRFAndDowngrade(t *testing.T) {
	tests := []string{
		"http://caldav.example.com/tasks",
		"https://127.0.0.1/tasks",
		"https://[::1]/tasks",
		"https://10.0.0.5/tasks",
		"https://169.254.169.254/latest/meta-data",
		"https://caldav.example.com:8443/tasks",
		"https://user:password@caldav.example.com/tasks",
	}
	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			if _, err := validateTargetURL(raw, false); err == nil {
				t.Fatalf("validateTargetURL(%q) accepted an unsafe destination", raw)
			}
		})
	}
}

func TestValidateTargetURLAcceptsHTTPS443(t *testing.T) {
	for _, raw := range []string{"https://caldav.example.com/tasks", "https://caldav.example.com:443/tasks"} {
		if _, err := validateTargetURL(raw, false); err != nil {
			t.Errorf("validateTargetURL(%q) rejected a valid destination: %v", raw, err)
		}
	}
}
