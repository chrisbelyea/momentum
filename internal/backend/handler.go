package backend

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/chrisbelyea/momentum/internal/caldav"
	"github.com/chrisbelyea/momentum/internal/db"
	"github.com/chrisbelyea/momentum/internal/models"
	"github.com/google/uuid"
)

// Handler handles backend configuration HTTP requests
type Handler struct {
	backendRepo *db.BackendRepository
}

// NewHandler creates a new backend Handler
func NewHandler(backendRepo *db.BackendRepository) *Handler {
	return &Handler{
		backendRepo: backendRepo,
	}
}

// HandleBackends handles requests to /backends
// GET: List all backends for a user
// POST: Create a new backend
func (h *Handler) HandleBackends(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listBackends(w, r)
	case http.MethodPost:
		h.createBackend(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleBackend handles requests to /backends/{id}
// GET: Get a single backend
// PUT: Update a backend
// DELETE: Delete a backend
func (h *Handler) HandleBackend(w http.ResponseWriter, r *http.Request) {
	// Extract backend ID from path
	backendID := strings.TrimPrefix(r.URL.Path, "/backends/")
	if backendID == "" {
		http.Error(w, "Backend ID is required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getBackend(w, r, backendID)
	case http.MethodPut:
		h.updateBackend(w, r, backendID)
	case http.MethodDelete:
		h.deleteBackend(w, r, backendID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleValidateConnection handles requests to /backends/validate
// POST: Validate a backend connection without saving
func (h *Handler) HandleValidateConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var backend models.Backend
	if err := json.NewDecoder(r.Body).Decode(&backend); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Validate backend configuration
	if err := backend.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("Invalid backend configuration: %v", err), http.StatusBadRequest)
		return
	}

	// Only external CalDAV backends can be validated
	if backend.Type != models.BackendTypeExternalCalDAV {
		http.Error(w, "Only external CalDAV backends can be validated", http.StatusBadRequest)
		return
	}

	// Create CalDAV client and validate connection
	client, err := caldav.NewClient(backend.Config)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create CalDAV client: %v", err), http.StatusBadRequest)
		return
	}
	defer client.Close()

	if err := client.ValidateConnection(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"valid": false,
			"error": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"valid":   true,
		"message": "Connection validated successfully",
	})
}

// listBackends lists all backends for a user
func (h *Handler) listBackends(w http.ResponseWriter, r *http.Request) {
	// Get user_id from query parameter
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		http.Error(w, "user_id query parameter is required", http.StatusBadRequest)
		return
	}

	backends, err := h.backendRepo.List(userID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list backends: %v", err), http.StatusInternalServerError)
		return
	}

	// Sanitize sensitive data
	for _, backend := range backends {
		backend.SanitizeForResponse()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(backends)
}

// getBackend gets a single backend by ID
func (h *Handler) getBackend(w http.ResponseWriter, r *http.Request, backendID string) {
	backend, err := h.backendRepo.Get(backendID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get backend: %v", err), http.StatusInternalServerError)
		return
	}

	if backend == nil {
		http.Error(w, "Backend not found", http.StatusNotFound)
		return
	}

	// Sanitize sensitive data
	backend.SanitizeForResponse()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(backend)
}

// createBackend creates a new backend
func (h *Handler) createBackend(w http.ResponseWriter, r *http.Request) {
	var backend models.Backend
	if err := json.NewDecoder(r.Body).Decode(&backend); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Generate ID if not provided
	if backend.ID == "" {
		backend.ID = uuid.New().String()
	}

	// Validate backend
	if err := backend.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("Invalid backend: %v", err), http.StatusBadRequest)
		return
	}

	if err := h.backendRepo.Create(&backend); err != nil {
		http.Error(w, fmt.Sprintf("Failed to create backend: %v", err), http.StatusInternalServerError)
		return
	}

	// Sanitize sensitive data
	backend.SanitizeForResponse()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(backend)
}

// updateBackend updates an existing backend
func (h *Handler) updateBackend(w http.ResponseWriter, r *http.Request, backendID string) {
	var backend models.Backend
	if err := json.NewDecoder(r.Body).Decode(&backend); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Ensure ID matches the URL
	backend.ID = backendID

	// Validate backend
	if err := backend.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("Invalid backend: %v", err), http.StatusBadRequest)
		return
	}

	if err := h.backendRepo.Update(&backend); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.Error(w, "Backend not found", http.StatusNotFound)
			return
		}
		http.Error(w, fmt.Sprintf("Failed to update backend: %v", err), http.StatusInternalServerError)
		return
	}

	// Sanitize sensitive data
	backend.SanitizeForResponse()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(backend)
}

// deleteBackend deletes a backend
func (h *Handler) deleteBackend(w http.ResponseWriter, r *http.Request, backendID string) {
	if err := h.backendRepo.Delete(backendID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.Error(w, "Backend not found", http.StatusNotFound)
			return
		}
		http.Error(w, fmt.Sprintf("Failed to delete backend: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
