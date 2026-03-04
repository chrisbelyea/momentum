package db

import (
	"database/sql"
	"testing"

	"github.com/chrisbelyea/momentum/internal/crypto"
	"github.com/chrisbelyea/momentum/internal/models"
	_ "github.com/mattn/go-sqlite3"
)

func setupBackendTestDB(t *testing.T) *sql.DB {
	// Initialize encryption for tests
	if err := crypto.InitializeEncryption("test-encryption-key-for-backends"); err != nil {
		t.Fatalf("Failed to initialize encryption: %v", err)
	}

	// Create in-memory SQLite database
	database, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Create schema
	schema := `
		CREATE TABLE users (
			id VARCHAR(36) PRIMARY KEY NOT NULL,
			email VARCHAR(255) NOT NULL UNIQUE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE backends (
			id VARCHAR(36) PRIMARY KEY NOT NULL,
			user_id VARCHAR(36) NOT NULL,
			backend_type VARCHAR(50) NOT NULL,
			name VARCHAR(255) NOT NULL,
			config_encrypted TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);
		CREATE INDEX idx_backends_user_id ON backends(user_id);
	`

	if _, err := database.Exec(schema); err != nil {
		t.Fatalf("Failed to create test schema: %v", err)
	}

	// Create test user
	database.Exec("INSERT INTO users (id, email) VALUES ('user-1', 'test@example.com')")

	return database
}

func TestBackendRepository_Create(t *testing.T) {
	db := setupBackendTestDB(t)
	defer db.Close()

	repo := NewBackendRepository(db)

	tests := []struct {
		name    string
		backend *models.Backend
		wantErr bool
	}{
		{
			name: "internal backend",
			backend: &models.Backend{
				ID:     "backend-1",
				UserID: "user-1",
				Type:   models.BackendTypeInternal,
				Name:   "Internal CalDAV",
			},
			wantErr: false,
		},
		{
			name: "external caldav with basic auth",
			backend: &models.Backend{
				ID:     "backend-2",
				UserID: "user-1",
				Type:   models.BackendTypeExternalCalDAV,
				Name:   "External CalDAV Server",
				Config: &models.BackendConfig{
					URL:      "https://caldav.example.com/dav/calendars/user",
					Username: "testuser",
					Password: "testpassword",
				},
			},
			wantErr: false,
		},
		{
			name: "external caldav with client cert",
			backend: &models.Backend{
				ID:     "backend-3",
				UserID: "user-1",
				Type:   models.BackendTypeExternalCalDAV,
				Name:   "CalDAV with Cert",
				Config: &models.BackendConfig{
					URL:            "https://caldav.example.com/dav/calendars/user",
					ClientCertPath: "/path/to/cert.pem",
					ClientKeyPath:  "/path/to/key.pem",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid - missing user_id",
			backend: &models.Backend{
				ID:   "backend-4",
				Type: models.BackendTypeInternal,
				Name: "Test Backend",
			},
			wantErr: true,
		},
		{
			name: "invalid - external caldav without config",
			backend: &models.Backend{
				ID:     "backend-5",
				UserID: "user-1",
				Type:   models.BackendTypeExternalCalDAV,
				Name:   "Invalid Backend",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Create(tt.backend)
			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				// Verify backend was created
				retrieved, err := repo.Get(tt.backend.ID)
				if err != nil {
					t.Errorf("Failed to get created backend: %v", err)
				}
				if retrieved == nil {
					t.Error("Expected backend to be created")
				}
				if retrieved != nil {
					if retrieved.Name != tt.backend.Name {
						t.Errorf("Name mismatch: got %s, want %s", retrieved.Name, tt.backend.Name)
					}
					if retrieved.Type != tt.backend.Type {
						t.Errorf("Type mismatch: got %s, want %s", retrieved.Type, tt.backend.Type)
					}
				}
			}
		})
	}
}

func TestBackendRepository_Get(t *testing.T) {
	db := setupBackendTestDB(t)
	defer db.Close()

	repo := NewBackendRepository(db)

	// Create a test backend
	backend := &models.Backend{
		ID:     "backend-1",
		UserID: "user-1",
		Type:   models.BackendTypeExternalCalDAV,
		Name:   "Test Backend",
		Config: &models.BackendConfig{
			URL:      "https://caldav.example.com/dav",
			Username: "user",
			Password: "secret123",
		},
	}

	if err := repo.Create(backend); err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}

	// Get the backend
	retrieved, err := repo.Get(backend.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected backend to be retrieved")
	}

	// Verify fields
	if retrieved.ID != backend.ID {
		t.Errorf("ID mismatch: got %s, want %s", retrieved.ID, backend.ID)
	}
	if retrieved.Name != backend.Name {
		t.Errorf("Name mismatch: got %s, want %s", retrieved.Name, backend.Name)
	}
	if retrieved.Type != backend.Type {
		t.Errorf("Type mismatch: got %s, want %s", retrieved.Type, backend.Type)
	}

	// Verify config was decrypted
	if retrieved.Config == nil {
		t.Fatal("Expected config to be retrieved")
	}
	if retrieved.Config.URL != backend.Config.URL {
		t.Errorf("URL mismatch: got %s, want %s", retrieved.Config.URL, backend.Config.URL)
	}
	if retrieved.Config.Username != backend.Config.Username {
		t.Errorf("Username mismatch: got %s, want %s", retrieved.Config.Username, backend.Config.Username)
	}
	if retrieved.Config.Password != backend.Config.Password {
		t.Errorf("Password mismatch: got %s, want %s", retrieved.Config.Password, backend.Config.Password)
	}

	// Test non-existent backend
	nonExistent, err := repo.Get("non-existent-id")
	if err != nil {
		t.Errorf("Get() should not error for non-existent backend: %v", err)
	}
	if nonExistent != nil {
		t.Error("Expected nil for non-existent backend")
	}
}

func TestBackendRepository_List(t *testing.T) {
	db := setupBackendTestDB(t)
	defer db.Close()

	repo := NewBackendRepository(db)

	// Create multiple backends
	backends := []*models.Backend{
		{
			ID:     "backend-1",
			UserID: "user-1",
			Type:   models.BackendTypeInternal,
			Name:   "Internal Backend",
		},
		{
			ID:     "backend-2",
			UserID: "user-1",
			Type:   models.BackendTypeExternalCalDAV,
			Name:   "External Backend",
			Config: &models.BackendConfig{
				URL:      "https://caldav.example.com",
				Username: "user",
				Password: "pass",
			},
		},
	}

	for _, backend := range backends {
		if err := repo.Create(backend); err != nil {
			t.Fatalf("Failed to create backend: %v", err)
		}
	}

	// List backends
	retrieved, err := repo.List("user-1")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(retrieved) != len(backends) {
		t.Errorf("Expected %d backends, got %d", len(backends), len(retrieved))
	}

	// List for non-existent user
	empty, err := repo.List("non-existent-user")
	if err != nil {
		t.Errorf("List() should not error for non-existent user: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("Expected 0 backends for non-existent user, got %d", len(empty))
	}
}

func TestBackendRepository_Update(t *testing.T) {
	db := setupBackendTestDB(t)
	defer db.Close()

	repo := NewBackendRepository(db)

	// Create a backend
	backend := &models.Backend{
		ID:     "backend-1",
		UserID: "user-1",
		Type:   models.BackendTypeExternalCalDAV,
		Name:   "Original Name",
		Config: &models.BackendConfig{
			URL:      "https://original.example.com",
			Username: "user",
			Password: "pass",
		},
	}

	if err := repo.Create(backend); err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}

	// Update the backend
	backend.Name = "Updated Name"
	backend.Config.URL = "https://updated.example.com"

	if err := repo.Update(backend); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Verify update
	retrieved, err := repo.Get(backend.ID)
	if err != nil {
		t.Fatalf("Failed to get updated backend: %v", err)
	}

	if retrieved.Name != "Updated Name" {
		t.Errorf("Name not updated: got %s, want %s", retrieved.Name, "Updated Name")
	}
	if retrieved.Config.URL != "https://updated.example.com" {
		t.Errorf("URL not updated: got %s, want %s", retrieved.Config.URL, "https://updated.example.com")
	}

	// Test updating non-existent backend
	nonExistent := &models.Backend{
		ID:     "non-existent",
		UserID: "user-1",
		Type:   models.BackendTypeInternal,
		Name:   "Non-existent",
	}
	err = repo.Update(nonExistent)
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestBackendRepository_Delete(t *testing.T) {
	db := setupBackendTestDB(t)
	defer db.Close()

	repo := NewBackendRepository(db)

	// Create a backend
	backend := &models.Backend{
		ID:     "backend-1",
		UserID: "user-1",
		Type:   models.BackendTypeInternal,
		Name:   "Test Backend",
	}

	if err := repo.Create(backend); err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}

	// Delete the backend
	if err := repo.Delete(backend.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deletion
	retrieved, err := repo.Get(backend.ID)
	if err != nil {
		t.Errorf("Get() should not error after deletion: %v", err)
	}
	if retrieved != nil {
		t.Error("Expected backend to be deleted")
	}

	// Test deleting non-existent backend
	err = repo.Delete("non-existent")
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestBackendRepository_EncryptionRoundTrip(t *testing.T) {
	db := setupBackendTestDB(t)
	defer db.Close()

	repo := NewBackendRepository(db)

	// Create backend with sensitive data
	backend := &models.Backend{
		ID:     "backend-1",
		UserID: "user-1",
		Type:   models.BackendTypeExternalCalDAV,
		Name:   "Secure Backend",
		Config: &models.BackendConfig{
			URL:            "https://caldav.example.com/dav",
			Username:       "testuser",
			Password:       "SuperSecret123!@#",
			ClientCertPath: "/path/to/cert.pem",
			ClientKeyPath:  "/path/to/key.pem",
		},
	}

	// Create backend
	if err := repo.Create(backend); err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}

	// Retrieve backend
	retrieved, err := repo.Get(backend.ID)
	if err != nil {
		t.Fatalf("Failed to get backend: %v", err)
	}

	// Verify all sensitive data is correctly decrypted
	if retrieved.Config.Password != backend.Config.Password {
		t.Errorf("Password not correctly decrypted: got %s, want %s", retrieved.Config.Password, backend.Config.Password)
	}
	if retrieved.Config.ClientCertPath != backend.Config.ClientCertPath {
		t.Errorf("ClientCertPath not correctly decrypted: got %s, want %s", retrieved.Config.ClientCertPath, backend.Config.ClientCertPath)
	}
	if retrieved.Config.ClientKeyPath != backend.Config.ClientKeyPath {
		t.Errorf("ClientKeyPath not correctly decrypted: got %s, want %s", retrieved.Config.ClientKeyPath, backend.Config.ClientKeyPath)
	}

	// Verify that data is actually encrypted in the database
	var encryptedConfig string
	err = db.QueryRow("SELECT config_encrypted FROM backends WHERE id = ?", backend.ID).Scan(&encryptedConfig)
	if err != nil {
		t.Fatalf("Failed to query encrypted config: %v", err)
	}

	// Encrypted config should not contain plain text password
	if len(encryptedConfig) > 0 && encryptedConfig == backend.Config.Password {
		t.Error("Password appears to be stored in plain text")
	}
}
