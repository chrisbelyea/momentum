package caldav

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/chrisbelyea/momentum/internal/db"
	"github.com/chrisbelyea/momentum/internal/models"
)

// Handler handles CalDAV HTTP requests
type Handler struct {
	taskRepo *db.TaskRepository
}

// NewHandler creates a new CalDAV Handler
func NewHandler(taskRepo *db.TaskRepository) *Handler {
	return &Handler{
		taskRepo: taskRepo,
	}
}

// HandleTasks handles requests to /caldav/tasks
// GET: List all tasks for a backend
// POST: Create a new task
func (h *Handler) HandleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listTasks(w, r)
	case http.MethodPost:
		h.createTask(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleTask handles requests to /caldav/tasks/{id}
// GET: Get a single task
// PUT: Update a task
// DELETE: Delete a task
func (h *Handler) HandleTask(w http.ResponseWriter, r *http.Request) {
	// Extract task ID from path
	taskIDStr := strings.TrimPrefix(r.URL.Path, "/caldav/tasks/")
	if taskIDStr == "" {
		http.Error(w, "Task ID is required", http.StatusBadRequest)
		return
	}

	taskID, err := strconv.Atoi(taskIDStr)
	if err != nil {
		http.Error(w, "Invalid task ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getTask(w, r, taskID)
	case http.MethodPut:
		h.updateTask(w, r, taskID)
	case http.MethodDelete:
		h.deleteTask(w, r, taskID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// listTasks lists all tasks for a backend
func (h *Handler) listTasks(w http.ResponseWriter, r *http.Request) {
	// Get backend_id from query parameter
	backendIDStr := r.URL.Query().Get("backend_id")
	if backendIDStr == "" {
		http.Error(w, "backend_id query parameter is required", http.StatusBadRequest)
		return
	}

	backendID, err := strconv.Atoi(backendIDStr)
	if err != nil {
		http.Error(w, "Invalid backend_id", http.StatusBadRequest)
		return
	}

	tasks, err := h.taskRepo.List(backendID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list tasks: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tasks)
}

// getTask gets a single task by ID
func (h *Handler) getTask(w http.ResponseWriter, r *http.Request, taskID int) {
	task, err := h.taskRepo.Get(taskID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get task: %v", err), http.StatusInternalServerError)
		return
	}

	if task == nil {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

// createTask creates a new task
func (h *Handler) createTask(w http.ResponseWriter, r *http.Request) {
	var task models.Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if task.BackendID == 0 {
		http.Error(w, "backend_id is required", http.StatusBadRequest)
		return
	}
	if task.Title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}
	if task.Status == "" {
		task.Status = models.StatusNeedsAction
	}

	if err := h.taskRepo.Create(&task); err != nil {
		http.Error(w, fmt.Sprintf("Failed to create task: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(task)
}

// updateTask updates an existing task
func (h *Handler) updateTask(w http.ResponseWriter, r *http.Request, taskID int) {
	var task models.Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Ensure ID matches the URL
	task.ID = taskID

	// Validate required fields
	if task.BackendID == 0 {
		http.Error(w, "backend_id is required", http.StatusBadRequest)
		return
	}
	if task.Title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}
	if task.Status == "" {
		http.Error(w, "status is required", http.StatusBadRequest)
		return
	}

	if err := h.taskRepo.Update(&task); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.Error(w, "Task not found", http.StatusNotFound)
			return
		}
		http.Error(w, fmt.Sprintf("Failed to update task: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

// deleteTask deletes a task
func (h *Handler) deleteTask(w http.ResponseWriter, r *http.Request, taskID int) {
	if err := h.taskRepo.Delete(taskID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.Error(w, "Task not found", http.StatusNotFound)
			return
		}
		http.Error(w, fmt.Sprintf("Failed to delete task: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
