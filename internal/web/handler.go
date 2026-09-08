package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chrisbelyea/momentum/internal/auth"
	"github.com/chrisbelyea/momentum/internal/db"
	"github.com/chrisbelyea/momentum/internal/models"
	webassets "github.com/chrisbelyea/momentum/web"
)

// Handler handles web UI requests
type Handler struct {
	taskRepo    *db.TaskRepository
	backendRepo *db.BackendRepository
	syncRepo    *db.SyncRepository
	templates   *template.Template
}

// NewHandler creates a new web Handler
func NewHandler(taskRepo *db.TaskRepository, backendRepos ...*db.BackendRepository) *Handler {
	var backendRepo *db.BackendRepository
	if len(backendRepos) > 0 {
		backendRepo = backendRepos[0]
	}
	return &Handler{
		taskRepo:    taskRepo,
		backendRepo: backendRepo,
		templates:   template.Must(template.ParseFS(webassets.Files, "templates/*.html")),
	}
}

// SetSyncRepository enables the authenticated sync status and conflict
// recovery API. Keeping this optional preserves the lightweight handler setup
// used by task-only tests and embedders.
func (h *Handler) SetSyncRepository(syncRepo *db.SyncRepository) {
	h.syncRepo = syncRepo
}

// HandleIndex serves the main kanban board page
func (h *Handler) HandleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := auth.UserIDFromRequest(r)
	if !ok {
		userID = 1
	}
	backendID, err := h.backendForRequest(r, userID)
	if err != nil {
		http.Error(w, "no task backend configured", 500)
		return
	}

	tasks, err := h.taskRepo.ListForUser(backendID, userID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load tasks: %v", err), http.StatusInternalServerError)
		return
	}

	// Apply filter (tag filter only; status filter handled by column grouping)
	filter := ParseTaskFilter(r)
	// For the board, only apply the tag filter so columns remain meaningful
	boardFilter := TaskFilter{Tag: filter.Tag, Sort: "created_at", Order: "desc"}
	tasks = ApplyFilter(tasks, boardFilter)

	// Group tasks by status
	data := struct {
		TodoTasks       []*models.Task
		InProgressTasks []*models.Task
		DoneTasks       []*models.Task
		Filter          TaskFilter
		Backends        []*models.Backend
		BackendID       int
	}{Filter: filter, BackendID: backendID, Backends: h.backendsForUser(userID)}

	for _, task := range tasks {
		switch task.Status {
		case models.StatusNeedsAction:
			data.TodoTasks = append(data.TodoTasks, task)
		case models.StatusInProcess:
			data.InProgressTasks = append(data.InProgressTasks, task)
		case models.StatusCompleted:
			data.DoneTasks = append(data.DoneTasks, task)
		}
	}

	if err := h.templates.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %v", err), http.StatusInternalServerError)
		return
	}
}

// HandleList serves the list view with filter/sort support
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := auth.UserIDFromRequest(r)
	if !ok {
		userID = 1
	}
	backendID, err := h.backendForRequest(r, userID)
	if err != nil {
		http.Error(w, "no task backend configured", 500)
		return
	}

	tasks, err := h.taskRepo.ListForUser(backendID, userID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load tasks: %v", err), http.StatusInternalServerError)
		return
	}

	filter := ParseTaskFilter(r)
	tasks = ApplyFilter(tasks, filter)

	data := struct {
		Tasks     []*models.Task
		Filter    TaskFilter
		Backends  []*models.Backend
		BackendID int
	}{
		Tasks:     tasks,
		Filter:    filter,
		BackendID: backendID,
		Backends:  h.backendsForUser(userID),
	}

	if err := h.templates.ExecuteTemplate(w, "list.html", data); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %v", err), http.StatusInternalServerError)
		return
	}
}

// HandleBackendsPage renders the authenticated backend configuration screen.
// The JSON endpoints remain in internal/backend; this page only supplies the
// browser workflow and gets its data from the same ownership-scoped repository.
func (h *Handler) HandleBackendsPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.backendRepo == nil {
		http.Error(w, "backend configuration is unavailable", http.StatusNotImplemented)
		return
	}
	userID, ok := auth.UserIDFromRequest(r)
	if !ok {
		userID = 1
	}
	backends, err := h.backendRepo.List(userID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load backends: %v", err), http.StatusInternalServerError)
		return
	}
	// List sanitizes only at the response boundary too, but keep this explicit
	// because the values are passed into an HTML template.
	for _, backend := range backends {
		backend.SanitizeForResponse()
	}
	if err := h.templates.ExecuteTemplate(w, "backends.html", struct {
		Backends []*models.Backend
	}{Backends: backends}); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render backend settings: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handler) backendForRequest(r *http.Request, userID int) (int, error) {
	if raw := r.URL.Query().Get("backend_id"); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 || !h.taskRepo.BackendOwned(id, userID) {
			return 0, fmt.Errorf("backend not found")
		}
		return id, nil
	}
	return h.taskRepo.DefaultBackendForUser(userID)
}

func (h *Handler) backendsForUser(userID int) []*models.Backend {
	if h.backendRepo == nil {
		return nil
	}
	backends, err := h.backendRepo.List(userID)
	if err != nil {
		return nil
	}
	for _, b := range backends {
		b.SanitizeForResponse()
	}
	return backends
}

// HandleTasks serves the browser task API: create, edit, delete, and status updates.
func (h *Handler) HandleTasks(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/tasks")
	if path == "" || path == "/" {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.createTask(w, r)
		return
	}
	if strings.HasSuffix(path, "/status") {
		h.HandleUpdateStatus(w, r)
		return
	}
	id, err := strconv.Atoi(strings.Trim(path, "/"))
	if err != nil || id <= 0 {
		http.Error(w, "Invalid task ID", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodPut:
		h.updateTask(w, r, id)
	case http.MethodDelete:
		h.deleteTask(w, r, id)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) createTask(w http.ResponseWriter, r *http.Request) {
	uid, ok := auth.UserIDFromRequest(r)
	if !ok {
		uid = 1
	}
	var task models.Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, "Invalid request body", 400)
		return
	}
	if task.BackendID == 0 {
		var err error
		task.BackendID, err = h.backendForRequest(r, uid)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
	}
	if task.Title == "" {
		http.Error(w, "title is required", 400)
		return
	}
	if task.Status == "" {
		task.Status = models.StatusNeedsAction
	}
	if err := validateTaskFields(&task); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !h.taskRepo.BackendOwned(task.BackendID, uid) {
		http.Error(w, "backend not found", 404)
		return
	}
	if err := h.taskRepo.Create(&task); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeTaskResponse(w, http.StatusCreated, &task)
}

func (h *Handler) updateTask(w http.ResponseWriter, r *http.Request, id int) {
	uid, ok := auth.UserIDFromRequest(r)
	if !ok {
		uid = 1
	}
	old, err := h.taskRepo.GetForUser(id, uid)
	if err != nil || old == nil {
		http.Error(w, "Task not found", 404)
		return
	}
	conditional, err := taskIfMatch(r, old)
	if err != nil {
		writeTaskConflict(w)
		return
	}
	// Start from the stored task so this endpoint remains a safe partial update:
	// fields omitted by older clients are preserved, while an explicit JSON null
	// can still clear nullable fields such as description, due_at, priority, and
	// tags_json.
	task := *old
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, "Invalid request body", 400)
		return
	}
	task.ID, task.BackendID = id, old.BackendID
	if task.Title == "" {
		http.Error(w, "title is required", 400)
		return
	}
	if task.Status == "" {
		task.Status = old.Status
	}
	if err := validateTaskFields(&task); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if conditional {
		err = h.taskRepo.UpdateIfUnchanged(&task, *old.UpdatedAt)
	} else {
		err = h.taskRepo.UpdateForUser(&task, uid)
	}
	if err != nil {
		if errors.Is(err, db.ErrTaskVersionConflict) {
			writeTaskConflict(w)
			return
		}
		http.Error(w, "Task not found", 404)
		return
	}
	writeTaskResponse(w, http.StatusOK, &task)
}

// validateTaskFields enforces the values represented by the browser task
// editor at the API boundary as well. The repository deliberately remains a
// general VTODO persistence layer, so validation belongs here rather than in
// SQL or in a browser-only script.
func validateTaskFields(task *models.Task) error {
	validStatuses := map[string]bool{
		models.StatusNeedsAction: true,
		models.StatusInProcess:   true,
		models.StatusCompleted:   true,
		models.StatusCancelled:   true,
	}
	if !validStatuses[task.Status] {
		return fmt.Errorf("invalid status value")
	}
	if task.Priority != nil && (*task.Priority < 0 || *task.Priority > 9) {
		return fmt.Errorf("priority must be between 0 and 9")
	}
	if task.DueDateOnly && task.DueAt == nil {
		return fmt.Errorf("due_date_only requires due_at")
	}
	if task.TagsJSON != nil {
		var tags []string
		if err := json.Unmarshal([]byte(*task.TagsJSON), &tags); err != nil {
			return fmt.Errorf("tags_json must be a JSON array of strings")
		}
		for _, tag := range tags {
			if strings.TrimSpace(tag) == "" {
				return fmt.Errorf("tags_json cannot contain empty tags")
			}
		}
	}
	return nil
}

func (h *Handler) deleteTask(w http.ResponseWriter, r *http.Request, id int) {
	uid, ok := auth.UserIDFromRequest(r)
	if !ok {
		uid = 1
	}
	old, err := h.taskRepo.GetForUser(id, uid)
	if err != nil || old == nil {
		http.Error(w, "Task not found", 404)
		return
	}
	conditional, err := taskIfMatch(r, old)
	if err != nil {
		writeTaskConflict(w)
		return
	}
	if conditional {
		err = h.taskRepo.DeleteIfUnchanged(id, *old.UpdatedAt)
	} else {
		err = h.taskRepo.DeleteForUser(id, uid)
	}
	if err != nil {
		if errors.Is(err, db.ErrTaskVersionConflict) {
			writeTaskConflict(w)
			return
		}
		http.Error(w, "Task not found", 404)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleUpdateStatus updates a task's status via PATCH
func (h *Handler) HandleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract task ID from path
	taskIDStr := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	taskIDStr = strings.TrimSuffix(taskIDStr, "/status")
	if taskIDStr == "" {
		http.Error(w, "Task ID is required", http.StatusBadRequest)
		return
	}

	taskID, err := strconv.Atoi(taskIDStr)
	if err != nil {
		http.Error(w, "Invalid task ID", http.StatusBadRequest)
		return
	}

	// Parse request body
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Validate status value
	validStatuses := map[string]bool{
		models.StatusNeedsAction: true,
		models.StatusInProcess:   true,
		models.StatusCompleted:   true,
		models.StatusCancelled:   true,
	}
	if !validStatuses[req.Status] {
		http.Error(w, "Invalid status value", http.StatusBadRequest)
		return
	}

	// Get the existing task
	userID, ok := auth.UserIDFromRequest(r)
	if !ok {
		userID = 1
	}
	task, err := h.taskRepo.GetForUser(taskID, userID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get task: %v", err), http.StatusInternalServerError)
		return
	}
	if task == nil {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}
	conditional, err := taskIfMatch(r, task)
	if err != nil {
		writeTaskConflict(w)
		return
	}

	// Update the status
	task.Status = req.Status

	if conditional {
		err = h.taskRepo.UpdateIfUnchanged(task, *task.UpdatedAt)
	} else {
		err = h.taskRepo.UpdateForUser(task, userID)
	}
	if err != nil {
		if errors.Is(err, db.ErrTaskVersionConflict) {
			writeTaskConflict(w)
			return
		}
		http.Error(w, fmt.Sprintf("Failed to update task: %v", err), http.StatusInternalServerError)
		return
	}

	writeTaskResponse(w, http.StatusOK, task)
}

// taskVersion is deliberately an opaque, timestamp-derived ETag. The value
// is emitted in rendered task cards and compared by conditional mutations;
// clients that do not send If-Match retain the legacy last-write-wins API.
func taskVersion(task *models.Task) string {
	if task == nil || task.UpdatedAt == nil {
		return ""
	}
	// Preserve the persisted offset. Rendered cards use the same time value;
	// normalizing only one side would make an otherwise fresh ETag look stale
	// when the server is running outside UTC.
	return task.UpdatedAt.Format(time.RFC3339Nano)
}

func taskIfMatch(r *http.Request, current *models.Task) (bool, error) {
	raw := strings.TrimSpace(r.Header.Get("If-Match"))
	if raw == "" {
		return false, nil
	}
	if len(raw) >= 2 && strings.HasPrefix(raw, "\"") && strings.HasSuffix(raw, "\"") {
		raw = raw[1 : len(raw)-1]
	}
	if raw == "*" || raw != taskVersion(current) {
		return true, db.ErrTaskVersionConflict
	}
	return true, nil
}

func writeTaskConflict(w http.ResponseWriter) {
	http.Error(w, "task changed since it was read; reload and retry", http.StatusConflict)
}

func writeTaskResponse(w http.ResponseWriter, status int, task *models.Task) {
	if version := taskVersion(task); version != "" {
		w.Header().Set("ETag", `"`+version+`"`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(task)
}

// HandleSyncConflicts exposes retained local/provider snapshots and the Phase
// 1 manual recovery path. Authentication and backend ownership are enforced by
// the repository query, so a conflict ID cannot disclose another user's data.
// GET /api/sync/conflicts[?status=open&backend_id=N] lists conflicts.
// GET /api/sync/conflicts/{id} returns one conflict.
// PATCH /api/sync/conflicts/{id} resolves with {"resolution":"local"},
// {"resolution":"remote"}, or {"resolution":"dismissed"}.
func (h *Handler) HandleSyncConflicts(w http.ResponseWriter, r *http.Request) {
	if h.syncRepo == nil {
		http.Error(w, "sync recovery is unavailable", http.StatusNotImplemented)
		return
	}
	userID, ok := auth.UserIDFromRequest(r)
	if !ok {
		userID = 1
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/sync/conflicts")
	if path == "" || path == "/" {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		backendID := 0
		if raw := r.URL.Query().Get("backend_id"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed <= 0 {
				http.Error(w, "invalid backend_id", http.StatusBadRequest)
				return
			}
			backendID = parsed
		}
		status := r.URL.Query().Get("status")
		if status == "" {
			status = "open"
		} else if status == "all" {
			status = ""
		}
		conflicts, err := h.syncRepo.ListConflicts(r.Context(), userID, status, backendID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list sync conflicts: %v", err), http.StatusInternalServerError)
			return
		}
		writeSyncConflicts(w, conflicts)
		return
	}
	id, err := strconv.Atoi(strings.Trim(path, "/"))
	if err != nil || id <= 0 {
		http.Error(w, "invalid conflict ID", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		conflict, err := h.syncRepo.GetConflict(r.Context(), userID, id)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				http.Error(w, "sync conflict not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get sync conflict: %v", err), http.StatusInternalServerError)
			return
		}
		writeSyncConflict(w, *conflict)
	case http.MethodPatch:
		var request struct {
			Resolution string `json:"resolution"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		conflict, err := h.syncRepo.ResolveConflict(r.Context(), userID, id, request.Resolution)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				http.Error(w, "sync conflict not found", http.StatusNotFound)
				return
			}
			if errors.Is(err, db.ErrInvalidConflictResolution) || errors.Is(err, db.ErrConflictAlreadyResolved) || errors.Is(err, db.ErrConflictNoTask) || errors.Is(err, db.ErrInvalidConflictSnapshot) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, fmt.Sprintf("failed to resolve sync conflict: %v", err), http.StatusInternalServerError)
			return
		}
		writeSyncConflict(w, *conflict)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func writeSyncConflicts(w http.ResponseWriter, conflicts []db.SyncConflict) {
	w.Header().Set("Content-Type", "application/json")
	if conflicts == nil {
		conflicts = []db.SyncConflict{}
	}
	_ = json.NewEncoder(w).Encode(conflicts)
}

func writeSyncConflict(w http.ResponseWriter, conflict db.SyncConflict) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(conflict)
}
