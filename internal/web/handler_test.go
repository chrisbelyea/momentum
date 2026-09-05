package web

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/chrisbelyea/momentum/internal/db"
	"github.com/chrisbelyea/momentum/internal/models"
	_ "github.com/mattn/go-sqlite3"
)

// setupTestDB creates a test database with the tasks table
func setupTestDB(t *testing.T) *sql.DB {
	// Create in-memory SQLite database
	database, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	if err := db.InitializeSchema(database); err != nil {
		t.Fatalf("Failed to initialize test schema: %v", err)
	}

	// Create test user and backend
	database.Exec("INSERT INTO users (id, email) VALUES (1, 'test@example.com')")
	database.Exec("INSERT INTO backends (id, user_id, backend_type, name) VALUES (1, 1, 'internal', 'Test Backend')")

	return database
}

func TestUpdateStatus(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	taskRepo := db.NewTaskRepository(database)

	// Create a test task
	task := &models.Task{
		BackendID: 1,
		Title:     "Test Task",
		Status:    models.StatusNeedsAction,
	}
	if err := taskRepo.Create(task); err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	// Create handler without templates (we're only testing the API)
	handler := &Handler{
		taskRepo:  taskRepo,
		templates: nil,
	}

	// Update status to IN-PROCESS
	updateReq := struct {
		Status string `json:"status"`
	}{
		Status: models.StatusInProcess,
	}
	body, _ := json.Marshal(updateReq)

	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/tasks/%d/status", task.ID), bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	handler.HandleUpdateStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var updatedTask models.Task
	json.NewDecoder(rec.Body).Decode(&updatedTask)

	if updatedTask.Status != models.StatusInProcess {
		t.Errorf("Expected status '%s', got '%s'", models.StatusInProcess, updatedTask.Status)
	}

	// Verify the change persisted in the database
	taskFromDB, err := taskRepo.Get(task.ID)
	if err != nil {
		t.Fatalf("Failed to get task from DB: %v", err)
	}

	if taskFromDB.Status != models.StatusInProcess {
		t.Errorf("Status not persisted in DB. Expected '%s', got '%s'", models.StatusInProcess, taskFromDB.Status)
	}
}

func TestUpdateStatusNotFound(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	taskRepo := db.NewTaskRepository(database)
	handler := &Handler{
		taskRepo:  taskRepo,
		templates: nil,
	}

	// Try to update a non-existent task
	updateReq := struct {
		Status string `json:"status"`
	}{
		Status: models.StatusInProcess,
	}
	body, _ := json.Marshal(updateReq)

	req := httptest.NewRequest(http.MethodPatch, "/api/tasks/999/status", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	handler.HandleUpdateStatus(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", rec.Code)
	}
}

func TestUpdateStatusMethodNotAllowed(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	taskRepo := db.NewTaskRepository(database)
	handler := &Handler{
		taskRepo:  taskRepo,
		templates: nil,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/tasks/1/status", nil)
	rec := httptest.NewRecorder()

	handler.HandleUpdateStatus(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", rec.Code)
	}
}

func TestUpdateStatusInvalidStatus(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	taskRepo := db.NewTaskRepository(database)

	// Create a test task
	task := &models.Task{
		BackendID: 1,
		Title:     "Test Task",
		Status:    models.StatusNeedsAction,
	}
	if err := taskRepo.Create(task); err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	handler := &Handler{
		taskRepo:  taskRepo,
		templates: nil,
	}

	// Try to set an invalid status
	updateReq := struct {
		Status string `json:"status"`
	}{
		Status: "INVALID-STATUS",
	}
	body, _ := json.Marshal(updateReq)

	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/tasks/%d/status", task.ID), bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	handler.HandleUpdateStatus(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", rec.Code)
	}

	// Verify the task status didn't change
	taskFromDB, err := taskRepo.Get(task.ID)
	if err != nil {
		t.Fatalf("Failed to get task from DB: %v", err)
	}

	if taskFromDB.Status != models.StatusNeedsAction {
		t.Errorf("Status should not have changed. Expected '%s', got '%s'", models.StatusNeedsAction, taskFromDB.Status)
	}
}

func TestUpdateStatusPersistence(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	taskRepo := db.NewTaskRepository(database)
	handler := &Handler{
		taskRepo:  taskRepo,
		templates: nil,
	}

	// Create tasks in different statuses
	tasks := []*models.Task{
		{BackendID: 1, Title: "Task 1", Status: models.StatusNeedsAction},
		{BackendID: 1, Title: "Task 2", Status: models.StatusInProcess},
		{BackendID: 1, Title: "Task 3", Status: models.StatusCompleted},
	}

	for _, task := range tasks {
		if err := taskRepo.Create(task); err != nil {
			t.Fatalf("Failed to create task: %v", err)
		}
	}

	// Move Task 1 from NEEDS-ACTION to IN-PROCESS
	updateReq := struct {
		Status string `json:"status"`
	}{Status: models.StatusInProcess}
	body, _ := json.Marshal(updateReq)

	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/tasks/%d/status", tasks[0].ID), bytes.NewBuffer(body))
	rec := httptest.NewRecorder()
	handler.HandleUpdateStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	// Move Task 2 from IN-PROCESS to COMPLETED
	updateReq = struct {
		Status string `json:"status"`
	}{Status: models.StatusCompleted}
	body, _ = json.Marshal(updateReq)

	req = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/tasks/%d/status", tasks[1].ID), bytes.NewBuffer(body))
	rec = httptest.NewRecorder()
	handler.HandleUpdateStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	// Verify all tasks persisted correctly
	allTasks, err := taskRepo.List(1)
	if err != nil {
		t.Fatalf("Failed to list tasks: %v", err)
	}

	if len(allTasks) != 3 {
		t.Fatalf("Expected 3 tasks, got %d", len(allTasks))
	}

	// Check statuses
	expectedStatuses := map[int]string{
		tasks[0].ID: models.StatusInProcess,
		tasks[1].ID: models.StatusCompleted,
		tasks[2].ID: models.StatusCompleted,
	}

	for _, task := range allTasks {
		expected := expectedStatuses[task.ID]
		if task.Status != expected {
			t.Errorf("Task %d: expected status '%s', got '%s'", task.ID, expected, task.Status)
		}
	}
}

func TestMain(m *testing.M) {
	// Run tests
	code := m.Run()
	os.Exit(code)
}
