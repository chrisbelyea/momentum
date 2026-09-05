package backend

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chrisbelyea/momentum/internal/crypto"
	"github.com/chrisbelyea/momentum/internal/db"
	"github.com/chrisbelyea/momentum/internal/models"
	_ "github.com/mattn/go-sqlite3"
)

func setupBackendHandlerTestDB(t *testing.T) *sql.DB {
	// Initialize encryption for tests
	if err := crypto.InitializeEncryption("test-encryption-key-for-handlers"); err != nil {
		t.Fatalf("Failed to initialize encryption: %v", err)
	}

	// Create in-memory SQLite database
	database, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	if err := db.InitializeSchema(database); err != nil {
		t.Fatalf("Failed to initialize test schema: %v", err)
	}

	// Create test user
	database.Exec("INSERT INTO users (id, email) VALUES (1, 'test@example.com')")

	return database
}

func TestHandler_CreateBackend(t *testing.T) {
	database := setupBackendHandlerTestDB(t)
	defer database.Close()

	backendRepo := db.NewBackendRepository(database)
	handler := NewHandler(backendRepo)

	tests := []struct {
		name           string
		backend        models.Backend
		expectedStatus int
	}{
		{
			name: "create internal backend",
			backend: models.Backend{
				UserID: 1,
				Type:   models.BackendTypeInternal,
				Name:   "Internal Backend",
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "create external caldav backend",
			backend: models.Backend{
				UserID: 1,
				Type:   models.BackendTypeExternalCalDAV,
				Name:   "External CalDAV",
				Config: &models.BackendConfig{
					URL:      "https://caldav.example.com",
					Username: "user",
					Password: "password123",
				},
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "invalid backend - missing user_id",
			backend: models.Backend{
				Type: models.BackendTypeInternal,
				Name: "Test",
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.backend)
			req := httptest.NewRequest(http.MethodPost, "/backends", bytes.NewBuffer(body))
			rec := httptest.NewRecorder()

			handler.HandleBackends(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d: %s", tt.expectedStatus, rec.Code, rec.Body.String())
			}

			if rec.Code == http.StatusCreated {
				var created models.Backend
				json.NewDecoder(rec.Body).Decode(&created)

				if created.ID == 0 {
					t.Error("Expected backend ID to be generated")
				}
				if created.Name != tt.backend.Name {
					t.Errorf("Expected name %s, got %s", tt.backend.Name, created.Name)
				}
				// Verify password is sanitized
				if created.Config != nil && created.Config.Password != "" {
					t.Error("Password should be sanitized in response")
				}
			}
		})
	}
}

func TestHandler_ListBackends(t *testing.T) {
	database := setupBackendHandlerTestDB(t)
	defer database.Close()

	backendRepo := db.NewBackendRepository(database)
	handler := NewHandler(backendRepo)

	// Create test backends
	backends := []*models.Backend{
		{
			ID:     1,
			UserID: 1,
			Type:   models.BackendTypeInternal,
			Name:   "Backend 1",
		},
		{
			ID:     2,
			UserID: 1,
			Type:   models.BackendTypeExternalCalDAV,
			Name:   "Backend 2",
			Config: &models.BackendConfig{
				URL:      "https://caldav.example.com",
				Username: "user",
				Password: "secret",
			},
		},
	}

	for _, backend := range backends {
		backendRepo.Create(backend)
	}

	// List backends
	req := httptest.NewRequest(http.MethodGet, "/backends?user_id=1", nil)
	rec := httptest.NewRecorder()

	handler.HandleBackends(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var retrieved []*models.Backend
	json.NewDecoder(rec.Body).Decode(&retrieved)

	if len(retrieved) != len(backends)+1 {
		t.Errorf("Expected %d backends, got %d", len(backends)+1, len(retrieved))
	}

	// Verify passwords are sanitized
	for _, backend := range retrieved {
		if backend.Config != nil && backend.Config.Password != "" {
			t.Error("Password should be sanitized in list response")
		}
	}
}

func TestHandler_GetBackend(t *testing.T) {
	database := setupBackendHandlerTestDB(t)
	defer database.Close()

	backendRepo := db.NewBackendRepository(database)
	handler := NewHandler(backendRepo)

	// Create a test backend
	backend := &models.Backend{
		ID:     1,
		UserID: 1,
		Type:   models.BackendTypeInternal,
		Name:   "Test Backend",
	}
	backendRepo.Create(backend)

	// Get backend
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/backends/%d", backend.ID), nil)
	rec := httptest.NewRecorder()

	handler.HandleBackend(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var retrieved models.Backend
	json.NewDecoder(rec.Body).Decode(&retrieved)

	if retrieved.ID != backend.ID {
		t.Errorf("Expected ID %d, got %d", backend.ID, retrieved.ID)
	}
}

func TestHandler_UpdateBackend(t *testing.T) {
	database := setupBackendHandlerTestDB(t)
	defer database.Close()

	backendRepo := db.NewBackendRepository(database)
	handler := NewHandler(backendRepo)

	// Create a backend
	backend := &models.Backend{
		ID:     1,
		UserID: 1,
		Type:   models.BackendTypeInternal,
		Name:   "Original Name",
	}
	backendRepo.Create(backend)

	// Update backend
	backend.Name = "Updated Name"
	body, _ := json.Marshal(backend)
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/backends/%d", backend.ID), bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	handler.HandleBackend(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var updated models.Backend
	json.NewDecoder(rec.Body).Decode(&updated)

	if updated.Name != "Updated Name" {
		t.Errorf("Expected name to be updated to 'Updated Name', got %s", updated.Name)
	}
}

func TestHandler_DeleteBackend(t *testing.T) {
	database := setupBackendHandlerTestDB(t)
	defer database.Close()

	backendRepo := db.NewBackendRepository(database)
	handler := NewHandler(backendRepo)

	// Create a backend
	backend := &models.Backend{
		ID:     1,
		UserID: 1,
		Type:   models.BackendTypeInternal,
		Name:   "Test Backend",
	}
	backendRepo.Create(backend)

	// Delete backend
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/backends/%d", backend.ID), nil)
	rec := httptest.NewRecorder()

	handler.HandleBackend(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", rec.Code)
	}

	// Verify deletion
	retrieved, _ := backendRepo.Get(backend.ID)
	if retrieved != nil {
		t.Error("Expected backend to be deleted")
	}
}

func TestHandler_ValidateConnection(t *testing.T) {
	database := setupBackendHandlerTestDB(t)
	defer database.Close()

	backendRepo := db.NewBackendRepository(database)
	handler := NewHandler(backendRepo)

	// Create a test CalDAV server
	caldavServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer caldavServer.Close()

	tests := []struct {
		name           string
		backend        models.Backend
		expectedStatus int
		expectValid    bool
	}{
		{
			name: "valid caldav connection",
			backend: models.Backend{
				UserID: 1,
				Type:   models.BackendTypeExternalCalDAV,
				Name:   "Test CalDAV",
				Config: &models.BackendConfig{
					URL:           caldavServer.URL,
					Username:      "user",
					Password:      "pass",
					SkipTLSVerify: true,
				},
			},
			expectedStatus: http.StatusOK,
			expectValid:    true,
		},
		{
			name: "invalid url",
			backend: models.Backend{
				Type: models.BackendTypeExternalCalDAV,
				Config: &models.BackendConfig{
					URL:      "http://invalid-domain.example.invalid",
					Username: "user",
					Password: "pass",
				},
			},
			expectedStatus: http.StatusBadRequest,
			expectValid:    false,
		},
		{
			name: "internal backend - not supported",
			backend: models.Backend{
				Type: models.BackendTypeInternal,
			},
			expectedStatus: http.StatusBadRequest,
			expectValid:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.backend)
			req := httptest.NewRequest(http.MethodPost, "/backends/validate", bytes.NewBuffer(body))
			rec := httptest.NewRecorder()

			handler.HandleValidateConnection(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d: %s", tt.expectedStatus, rec.Code, rec.Body.String())
			}

			if tt.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				json.NewDecoder(rec.Body).Decode(&response)

				if valid, ok := response["valid"].(bool); ok {
					if valid != tt.expectValid {
						t.Errorf("Expected valid=%v, got %v", tt.expectValid, valid)
					}
				}
			}
		})
	}
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	database := setupBackendHandlerTestDB(t)
	defer database.Close()

	backendRepo := db.NewBackendRepository(database)
	handler := NewHandler(backendRepo)

	tests := []struct {
		name   string
		path   string
		method string
	}{
		{"PATCH on /backends", "/backends", http.MethodPatch},
		{"OPTIONS on /backends", "/backends", http.MethodOptions},
		{"PATCH on /backends/{id}", "/backends/1", http.MethodPatch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()

			if tt.path == "/backends" {
				handler.HandleBackends(rec, req)
			} else {
				handler.HandleBackend(rec, req)
			}

			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("Expected status 405, got %d", rec.Code)
			}
		})
	}
}
