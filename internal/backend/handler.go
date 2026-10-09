package backend

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/chrisbelyea/momentum/internal/auth"
	"github.com/chrisbelyea/momentum/internal/caldav"
	"github.com/chrisbelyea/momentum/internal/db"
	"github.com/chrisbelyea/momentum/internal/models"
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
	backendID, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/backends/"))
	if err != nil || backendID <= 0 {
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
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&backend); err != nil {
		writeValidationError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if userID, ok := auth.UserIDFromRequest(r); ok {
		backend.UserID = userID
	}

	// Validate backend configuration
	if err := backend.Validate(); err != nil {
		writeValidationError(w, http.StatusBadRequest, "invalid backend configuration")
		return
	}

	// Only external CalDAV backends can be validated
	if backend.Type != models.BackendTypeExternalCalDAV {
		writeValidationError(w, http.StatusBadRequest, "only external CalDAV backends can be validated")
		return
	}

	// Create CalDAV client and validate connection
	client, err := caldav.NewClient(backend.Config)
	if err != nil {
		writeValidationError(w, http.StatusBadRequest, "CalDAV configuration is not valid")
		return
	}
	defer client.Close()

	if err := client.ValidateConnection(); err != nil {
		writeValidationError(w, http.StatusBadRequest, validationErrorMessage(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"valid":   true,
		"message": "Connection validated successfully",
	})
}

// writeValidationError keeps validation failures machine-readable for the
// settings page without echoing request data (especially credentials) into a
// browser or a log. The underlying error is intentionally reduced to a stable,
// actionable category at this boundary.
func writeValidationError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"valid": false, "error": message})
}

func validationErrorMessage(err error) string {
	if err == nil {
		return "connection validation failed"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "401"), strings.Contains(message, "403"), strings.Contains(message, "authentication"):
		return "check the CalDAV username and app password"
	case strings.Contains(message, "https"), strings.Contains(message, "url"), strings.Contains(message, "certificate"):
		return "check the HTTPS URL, certificate, and CalDAV server policy"
	default:
		return "the CalDAV server could not be reached; check the URL and credentials"
	}
}

// listBackends lists all backends for a user
func (h *Handler) listBackends(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromRequest(r)
	if !ok {
		userID, _ = strconv.Atoi(r.URL.Query().Get("user_id"))
		if userID <= 0 {
			userID = 1
		}
	}
	var err error
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
func (h *Handler) getBackend(w http.ResponseWriter, r *http.Request, backendID int) {
	backend, err := h.backendRepo.Get(backendID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get backend: %v", err), http.StatusInternalServerError)
		return
	}

	if backend == nil {
		http.Error(w, "Backend not found", http.StatusNotFound)
		return
	}
	if uid, ok := auth.UserIDFromRequest(r); ok && backend.UserID != uid {
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
	if userID, ok := auth.UserIDFromRequest(r); ok {
		backend.UserID = userID
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
func (h *Handler) updateBackend(w http.ResponseWriter, r *http.Request, backendID int) {
	var backend models.Backend
	if err := json.NewDecoder(r.Body).Decode(&backend); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	existing, err := h.backendRepo.Get(backendID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get backend: %v", err), http.StatusInternalServerError)
		return
	}
	if existing == nil {
		http.Error(w, "Backend not found", http.StatusNotFound)
		return
	}
	if uid, ok := auth.UserIDFromRequest(r); ok && existing.UserID != uid {
		// Do not disclose whether another user's backend ID exists.
		http.Error(w, "Backend not found", http.StatusNotFound)
		return
	}

	// Ensure ID matches the URL
	backend.ID = backendID
	if uid, ok := auth.UserIDFromRequest(r); ok {
		backend.UserID = uid
	} else {
		backend.UserID = existing.UserID
	}

	// GET responses intentionally redact passwords. Preserve the encrypted
	// credential when the settings page submits an edit without a replacement;
	// otherwise an innocuous name change would invalidate an external backend.
	// The value is only used in memory for validation/encryption and is never
	// written to the response.
	if backend.Config == nil {
		backend.Config = existing.Config
	} else if existing.Config != nil {
		if backend.Config.Password == "" {
			backend.Config.Password = existing.Config.Password
		}
		if backend.Config.Username == "" {
			backend.Config.Username = existing.Config.Username
		}
		if backend.Config.ClientCertPath == "" {
			backend.Config.ClientCertPath = existing.Config.ClientCertPath
		}
		if backend.Config.ClientKeyPath == "" {
			backend.Config.ClientKeyPath = existing.Config.ClientKeyPath
		}
	}

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
func (h *Handler) deleteBackend(w http.ResponseWriter, r *http.Request, backendID int) {
	uid, hasUser := auth.UserIDFromRequest(r)
	owned, err := h.backendRepo.Get(backendID)
	if hasUser && (err != nil || owned == nil || owned.UserID != uid) {
		http.Error(w, "Backend not found", 404)
		return
	}
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
