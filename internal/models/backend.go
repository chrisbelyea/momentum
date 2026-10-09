package models

import (
	"time"
)

// BackendType represents the type of task backend
type BackendType string

const (
	BackendTypeInternal       BackendType = "internal"
	BackendTypeExternalCalDAV BackendType = "external_caldav"
)

// Backend represents a task backend configuration
type Backend struct {
	ID        int            `json:"id"`
	UserID    int            `json:"user_id"`
	Type      BackendType    `json:"backend_type"`
	Name      string         `json:"name"`
	Config    *BackendConfig `json:"config,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// BackendConfig contains the configuration for a backend
type BackendConfig struct {
	// CalDAV specific fields
	URL            string `json:"url,omitempty"`
	Username       string `json:"username,omitempty"`
	Password       string `json:"password,omitempty"`
	ClientCertPath string `json:"client_cert_path,omitempty"`
	ClientKeyPath  string `json:"client_key_path,omitempty"`
	SkipTLSVerify  bool   `json:"skip_tls_verify,omitempty"`
	CalendarPath   string `json:"calendar_path,omitempty"`
}

// CalDAVAuthType represents the authentication method for CalDAV
type CalDAVAuthType string

const (
	CalDAVAuthBasic      CalDAVAuthType = "basic"
	CalDAVAuthClientCert CalDAVAuthType = "client_cert"
)

// Validate checks if the backend configuration is valid
func (b *Backend) Validate() error {
	if b.UserID <= 0 {
		return ErrInvalidBackend
	}
	if b.Name == "" {
		return ErrInvalidBackend
	}
	if b.Type == "" {
		return ErrInvalidBackend
	}
	if b.Type != BackendTypeInternal && b.Type != BackendTypeExternalCalDAV {
		return ErrInvalidBackend
	}

	// Validate CalDAV specific configuration
	if b.Type == BackendTypeExternalCalDAV {
		if b.Config == nil {
			return ErrInvalidBackend
		}
		if b.Config.URL == "" {
			return ErrInvalidBackend
		}
		// At least one auth method should be provided
		hasBasicAuth := b.Config.Username != "" && b.Config.Password != ""
		hasClientCert := b.Config.ClientCertPath != "" && b.Config.ClientKeyPath != ""
		if !hasBasicAuth && !hasClientCert {
			return ErrInvalidBackend
		}
	}

	return nil
}

// SanitizeForResponse removes sensitive data before sending to client
func (b *Backend) SanitizeForResponse() {
	if b.Config != nil {
		b.Config.Password = ""
	}
}
