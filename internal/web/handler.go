package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/chrisbelyea/momentum/internal/auth"
	"github.com/chrisbelyea/momentum/internal/db"
	"github.com/chrisbelyea/momentum/internal/models"
	webassets "github.com/chrisbelyea/momentum/web"
)

// Handler handles web UI requests
type Handler struct {
	taskRepo  *db.TaskRepository
	templates *template.Template
}

// NewHandler creates a new web Handler
func NewHandler(taskRepo *db.TaskRepository) *Handler {
	return &Handler{
		taskRepo:  taskRepo,
		templates: template.Must(template.ParseFS(webassets.Files, "templates/*.html")),
	}
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
	backendID, err := h.taskRepo.DefaultBackendForUser(userID)
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
	}{Filter: filter}

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
	backendID, err := h.taskRepo.DefaultBackendForUser(userID)
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
		Tasks  []*models.Task
		Filter TaskFilter
	}{
		Tasks:  tasks,
		Filter: filter,
	}

	if err := h.templates.ExecuteTemplate(w, "list.html", data); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %v", err), http.StatusInternalServerError)
		return
	}
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

	// Update the status
	task.Status = req.Status

	if err := h.taskRepo.UpdateForUser(task, userID); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update task: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}
