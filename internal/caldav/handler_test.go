package caldav

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

	// Create tasks table (matching actual schema)
	schema := `
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
			email VARCHAR(255) NOT NULL UNIQUE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE backends (
			id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
			user_id INTEGER NOT NULL,
			type VARCHAR(64) NOT NULL,
			name VARCHAR(128) NOT NULL,
			config_json TEXT,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);
		CREATE TABLE tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
			backend_id INTEGER NOT NULL,
			external_id VARCHAR(255),
			title VARCHAR(512) NOT NULL,
			description TEXT,
			status VARCHAR(64) NOT NULL,
			priority INTEGER,
			due_at TIMESTAMP,
			tags_json TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP,
			FOREIGN KEY (backend_id) REFERENCES backends(id) ON DELETE CASCADE
		);
	`

	if _, err := database.Exec(schema); err != nil {
		t.Fatalf("Failed to create test schema: %v", err)
	}

	// Create test user and backend
	database.Exec("INSERT INTO users (id, email) VALUES (1, 'test@example.com')")
	database.Exec("INSERT INTO backends (id, user_id, type, name) VALUES (1, 1, 'internal', 'Test Backend')")

	return database
}

func TestCreateTask(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	taskRepo := db.NewTaskRepository(database)
	handler := NewHandler(taskRepo)

	// Create test task
	task := models.Task{
		BackendID: 1,
		Title:     "Test Task",
		Status:    models.StatusNeedsAction,
		Priority:  intPtr(5),
	}

	body, _ := json.Marshal(task)
	req := httptest.NewRequest(http.MethodPost, "/caldav/tasks", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	handler.HandleTasks(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var createdTask models.Task
	json.NewDecoder(rec.Body).Decode(&createdTask)

	if createdTask.ID == 0 {
		t.Error("Expected task ID to be generated")
	}
	if createdTask.Title != "Test Task" {
		t.Errorf("Expected title 'Test Task', got '%s'", createdTask.Title)
	}
}

func TestListTasks(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	taskRepo := db.NewTaskRepository(database)
	handler := NewHandler(taskRepo)

	// Create a test task
	task := &models.Task{
		BackendID: 1,
		Title:     "Test Task",
		Status:    models.StatusNeedsAction,
	}
	taskRepo.Create(task)

	// List tasks
	req := httptest.NewRequest(http.MethodGet, "/caldav/tasks?backend_id=1", nil)
	rec := httptest.NewRecorder()

	handler.HandleTasks(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var tasks []*models.Task
	json.NewDecoder(rec.Body).Decode(&tasks)

	if len(tasks) != 1 {
		t.Errorf("Expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Title != "Test Task" {
		t.Errorf("Expected title 'Test Task', got '%s'", tasks[0].Title)
	}
}

func TestGetTask(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	taskRepo := db.NewTaskRepository(database)
	handler := NewHandler(taskRepo)

	// Create a test task
	task := &models.Task{
		BackendID: 1,
		Title:     "Test Task",
		Status:    models.StatusNeedsAction,
	}
	taskRepo.Create(task)

	// Get task
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/caldav/tasks/%d", task.ID), nil)
	rec := httptest.NewRecorder()

	handler.HandleTask(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var retrievedTask models.Task
	json.NewDecoder(rec.Body).Decode(&retrievedTask)

	if retrievedTask.ID != task.ID {
		t.Errorf("Expected task ID %d, got %d", task.ID, retrievedTask.ID)
	}
}

func TestUpdateTask(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	taskRepo := db.NewTaskRepository(database)
	handler := NewHandler(taskRepo)

	// Create a test task
	task := &models.Task{
		BackendID: 1,
		Title:     "Original Title",
		Status:    models.StatusNeedsAction,
	}
	taskRepo.Create(task)

	// Update task
	task.Title = "Updated Title"
	task.Status = models.StatusInProcess

	body, _ := json.Marshal(task)
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/caldav/tasks/%d", task.ID), bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	handler.HandleTask(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var updatedTask models.Task
	json.NewDecoder(rec.Body).Decode(&updatedTask)

	if updatedTask.Title != "Updated Title" {
		t.Errorf("Expected title 'Updated Title', got '%s'", updatedTask.Title)
	}
	if updatedTask.Status != models.StatusInProcess {
		t.Errorf("Expected status '%s', got '%s'", models.StatusInProcess, updatedTask.Status)
	}
}

func TestDeleteTask(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	taskRepo := db.NewTaskRepository(database)
	handler := NewHandler(taskRepo)

	// Create a test task
	task := &models.Task{
		BackendID: 1,
		Title:     "Test Task",
		Status:    models.StatusNeedsAction,
	}
	taskRepo.Create(task)

	// Delete task
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/caldav/tasks/%d", task.ID), nil)
	rec := httptest.NewRecorder()

	handler.HandleTask(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", rec.Code)
	}

	// Verify task is deleted
	deletedTask, err := taskRepo.Get(task.ID)
	if err != nil {
		t.Errorf("Error getting deleted task: %v", err)
	}
	if deletedTask != nil {
		t.Error("Expected task to be deleted")
	}
}

func intPtr(i int) *int {
	return &i
}

func TestMain(m *testing.M) {
	// Run tests
	code := m.Run()
	os.Exit(code)
}
