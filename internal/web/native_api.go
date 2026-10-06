package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	"github.com/chrisbelyea/momentum/internal/auth"
	"github.com/chrisbelyea/momentum/internal/models"
)

const nativeAPIMediaType = "application/vnd.momentum.v1+json"

// HandleNativeAPI exposes the versioned native surface. Mutations use the
// existing task handlers, preserving their ownership and validation rules.
func (h *Handler) HandleNativeAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", nativeAPIMediaType)
	path := strings.TrimPrefix(r.URL.Path, "/api/v1")
	switch {
	case path == "/capabilities" && r.Method == http.MethodGet:
		h.writeCapabilities(w)
	case path == "/tasks" && r.Method == http.MethodGet:
		h.listNativeTasks(w, r)
	case path == "/tasks" || strings.HasPrefix(path, "/tasks/"):
		if r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete {
			if strings.TrimSpace(r.Header.Get("If-Match")) == "" {
				writeNativeError(w, http.StatusPreconditionRequired, "version_required", "If-Match is required", false)
				return
			}
		}
		clone := r.Clone(r.Context())
		clone.URL.Path = "/api" + path
		rec := httptest.NewRecorder()
		h.HandleTasks(rec, clone)
		h.forwardNativeResponse(w, r, path, rec)
	case path == "/sync/conflicts" || strings.HasPrefix(path, "/sync/conflicts/"):
		clone := r.Clone(r.Context())
		clone.URL.Path = "/api" + path
		rec := httptest.NewRecorder()
		h.HandleSyncConflicts(rec, clone)
		h.forwardNativeResponse(w, r, path, rec)
	default:
		writeNativeError(w, http.StatusNotFound, "not_found", "native API route not found", false)
	}
}

func (h *Handler) forwardNativeResponse(w http.ResponseWriter, r *http.Request, path string, rec *httptest.ResponseRecorder) {
	if rec.Code == http.StatusConflict && strings.HasPrefix(path, "/tasks/") {
		var authoritative *models.Task
		idPart := strings.TrimSuffix(strings.TrimPrefix(path, "/tasks/"), "/status")
		if id, err := strconv.Atoi(idPart); err == nil {
			if userID, ok := auth.UserIDFromRequest(r); ok {
				authoritative, _ = h.taskRepo.GetForUser(id, userID)
			}
		}
		w.Header().Set("Content-Type", nativeAPIMediaType)
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":              map[string]any{"code": "task_conflict", "message": "task changed; review server version", "retryable": false},
			"authoritative_task": authoritative,
		})
		return
	}
	for key, values := range rec.Header() {
		if key == "Content-Type" {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	if rec.Code >= 400 {
		writeNativeError(w, rec.Code, "request_failed", "native request failed", rec.Code >= 500)
		return
	}
	w.Header().Set("Content-Type", nativeAPIMediaType)
	w.WriteHeader(rec.Code)
	_, _ = w.Write(rec.Body.Bytes())
}

func (h *Handler) writeCapabilities(w http.ResponseWriter) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"api":         map[string]any{"minimum": 1, "maximum": 1},
		"task_fields": []string{"title", "description", "status", "priority", "due_at", "due_date_only", "tags_json"},
		"features": map[string]bool{
			"board": true, "list": true, "sync_status": true,
			"conflict_recovery": true, "logout_revocation": true,
		},
		"limits": map[string]int{"request_body_bytes": 1048576},
	})
}

func (h *Handler) listNativeTasks(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromRequest(r)
	if !ok {
		writeNativeError(w, http.StatusUnauthorized, "authentication_required", "authentication required", false)
		return
	}
	backendID, err := h.backendForRequest(r, userID)
	if err != nil {
		writeNativeError(w, http.StatusBadRequest, "backend_not_found", "no accessible task backend", false)
		return
	}
	tasks, err := h.taskRepo.ListForUser(backendID, userID)
	if err != nil {
		writeNativeError(w, http.StatusInternalServerError, "task_list_failed", "failed to list tasks", true)
		return
	}
	if tasks == nil {
		tasks = make([]*models.Task, 0)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"backend_id": backendID,
		"tasks":      tasks,
	})
}

func writeNativeError(w http.ResponseWriter, status int, code, message string, retryable bool) {
	w.Header().Set("Content-Type", nativeAPIMediaType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{"code": code, "message": message, "retryable": retryable},
	})
}
