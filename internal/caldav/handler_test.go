package caldav

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/chrisbelyea/momentum/internal/db"
	"github.com/chrisbelyea/momentum/internal/models"
	"github.com/chrisbelyea/momentum/pkg/vtodo"
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

func TestICalendarTaskWorkflowAndConditionalRequests(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()
	handler := NewHandler(db.NewTaskRepository(database))
	body := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VTODO\r\nUID:ical-1@example.test\r\nDTSTAMP:20260905T120000Z\r\nSUMMARY:Calendar task\r\nSTATUS:NEEDS-ACTION\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"
	req := httptest.NewRequest(http.MethodPost, "/caldav/tasks?backend_id=1", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/calendar")
	rec := httptest.NewRecorder()
	handler.HandleTasks(rec, req)
	if rec.Code != http.StatusCreated || rec.Header().Get("Location") == "" || rec.Header().Get("ETag") == "" {
		t.Fatalf("calendar create: status=%d headers=%v body=%s", rec.Code, rec.Header(), rec.Body.String())
	}
	task, err := vtodo.Parse(rec.Body.Bytes())
	if err != nil || task.Summary != "Calendar task" {
		t.Fatalf("calendar response: %v %#v", err, task)
	}
	id := strings.TrimPrefix(rec.Header().Get("Location"), "/caldav/tasks/")
	get := httptest.NewRequest(http.MethodGet, "/caldav/tasks/"+id, nil)
	get.Header.Set("Accept", "text/calendar")
	getRec := httptest.NewRecorder()
	handler.HandleTask(getRec, get)
	if getRec.Code != http.StatusOK || getRec.Header().Get("Content-Type") != "text/calendar; charset=utf-8" {
		t.Fatalf("calendar get: %d %s", getRec.Code, getRec.Body.String())
	}
	etag := getRec.Header().Get("ETag")
	conditional := httptest.NewRequest(http.MethodGet, "/caldav/tasks/"+id, nil)
	conditional.Header.Set("Accept", "text/calendar")
	conditional.Header.Set("If-None-Match", etag)
	conditionalRec := httptest.NewRecorder()
	handler.HandleTask(conditionalRec, conditional)
	if conditionalRec.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", conditionalRec.Code)
	}
	updated := strings.Replace(body, "Calendar task", "Updated calendar task", 1)
	put := httptest.NewRequest(http.MethodPut, "/caldav/tasks/"+id, strings.NewReader(updated))
	put.Header.Set("Content-Type", "text/calendar")
	put.Header.Set("If-Match", etag)
	putRec := httptest.NewRecorder()
	handler.HandleTask(putRec, put)
	if putRec.Code != http.StatusOK || !strings.Contains(putRec.Body.String(), "Updated calendar task") {
		t.Fatalf("calendar put: %d %s", putRec.Code, putRec.Body.String())
	}
	del := httptest.NewRequest(http.MethodDelete, "/caldav/tasks/"+id, nil)
	del.Header.Set("If-Match", putRec.Header().Get("ETag"))
	delRec := httptest.NewRecorder()
	handler.HandleTask(delRec, del)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("calendar delete: %d", delRec.Code)
	}
}

func TestCollectionOptionsAndDiscovery(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()
	handler := NewHandler(db.NewTaskRepository(database))

	options := httptest.NewRequest(http.MethodOptions, "/caldav/tasks", nil)
	optionsRec := httptest.NewRecorder()
	handler.HandleTasks(optionsRec, options)
	if optionsRec.Code != http.StatusOK {
		t.Fatalf("OPTIONS status = %d", optionsRec.Code)
	}
	if got := optionsRec.Header().Get("Allow"); got != "OPTIONS, GET, POST, PROPFIND" {
		t.Fatalf("collection Allow = %q", got)
	}
	if got := optionsRec.Header().Get("DAV"); got != "1, calendar-access" {
		t.Fatalf("DAV = %q", got)
	}

	task := &models.Task{BackendID: 1, Title: "Discoverable", Status: models.StatusNeedsAction}
	if err := db.NewTaskRepository(database).Create(task); err != nil {
		t.Fatal(err)
	}
	propfind := httptest.NewRequest("PROPFIND", "/caldav/tasks", nil)
	propfind.Header.Set("Depth", "1")
	propfindRec := httptest.NewRecorder()
	handler.HandleTasks(propfindRec, propfind)
	if propfindRec.Code != http.StatusMultiStatus {
		t.Fatalf("PROPFIND status = %d: %s", propfindRec.Code, propfindRec.Body.String())
	}
	if got := propfindRec.Header().Get("Content-Type"); got != "application/xml; charset=utf-8" {
		t.Fatalf("PROPFIND content type = %q", got)
	}
	body := propfindRec.Body.String()
	for _, want := range []string{"/caldav/tasks/", "Momentum Tasks", "urn:ietf:params:xml:ns:caldav", "VTODO", fmt.Sprintf("/caldav/tasks/%d", task.ID), "text/calendar; component=VTODO", "getetag"} {
		if !strings.Contains(body, want) {
			t.Errorf("PROPFIND response missing %q: %s", want, body)
		}
	}
}

func TestCollectionDiscoveryRejectsUnsupportedDepth(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()
	handler := NewHandler(db.NewTaskRepository(database))
	req := httptest.NewRequest("PROPFIND", "/caldav/tasks", nil)
	req.Header.Set("Depth", "infinity")
	rec := httptest.NewRecorder()
	handler.HandleTasks(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for Depth infinity, got %d", rec.Code)
	}
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
