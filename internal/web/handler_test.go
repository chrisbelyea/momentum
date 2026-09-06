package web

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

func TestTaskWorkflowAPI(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()
	handler := &Handler{taskRepo: db.NewTaskRepository(database)}

	create := httptest.NewRequest(http.MethodPost, "/api/tasks", strings.NewReader(`{"title":"Ship UI","backend_id":1}`))
	created := httptest.NewRecorder()
	handler.HandleTasks(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create returned %d: %s", created.Code, created.Body.String())
	}
	var task models.Task
	if err := json.NewDecoder(created.Body).Decode(&task); err != nil || task.ID == 0 {
		t.Fatalf("invalid created task: %+v (%v)", task, err)
	}

	update := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/tasks/%d", task.ID), strings.NewReader(`{"title":"Ship UI v2"}`))
	updated := httptest.NewRecorder()
	handler.HandleTasks(updated, update)
	if updated.Code != http.StatusOK {
		t.Fatalf("update returned %d: %s", updated.Code, updated.Body.String())
	}
	if got, _ := handler.taskRepo.Get(task.ID); got == nil || got.Title != "Ship UI v2" {
		t.Fatalf("update did not persist: %+v", got)
	}

	deleted := httptest.NewRecorder()
	handler.HandleTasks(deleted, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/tasks/%d", task.ID), nil))
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete returned %d", deleted.Code)
	}
	if got, _ := handler.taskRepo.Get(task.ID); got != nil {
		t.Fatalf("task still exists after delete: %+v", got)
	}
}

func TestBackendSelectionUsesURLStateAndOwnership(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()
	if _, err := database.Exec("INSERT INTO backends (id,user_id,backend_type,name) VALUES (2,1,'internal','Second backend')"); err != nil {
		t.Fatal(err)
	}
	repo := db.NewTaskRepository(database)
	for _, task := range []*models.Task{{BackendID: 1, Title: "First"}, {BackendID: 2, Title: "Second"}} {
		if err := repo.Create(task); err != nil {
			t.Fatal(err)
		}
	}
	handler := NewHandler(repo, db.NewBackendRepository(database))
	rec := httptest.NewRecorder()
	handler.HandleIndex(rec, httptest.NewRequest(http.MethodGet, "/?backend_id=2", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("selection returned %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Second") || strings.Contains(rec.Body.String(), "First") {
		t.Fatalf("URL-selected backend rendered wrong tasks: %s", rec.Body.String())
	}
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

func TestRenderedViewsDoNotDependOnWorkingDirectory(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	// A release binary is commonly started by a service manager whose working
	// directory is unrelated to the installation directory. The handler must
	// still load and render its templates in that situation.
	t.Chdir(t.TempDir())
	handler := NewHandler(db.NewTaskRepository(database))

	for _, path := range []string{"/", "/list"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			if path == "/" {
				handler.HandleIndex(rec, req)
			} else {
				handler.HandleList(rec, req)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s returned %d: %s", path, rec.Code, rec.Body.String())
			}
			if !bytes.Contains(rec.Body.Bytes(), []byte("Momentum")) {
				t.Fatalf("GET %s did not render embedded template", path)
			}
		})
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

func TestSyncConflictRecoveryAPIListsAndResolvesOwnedConflict(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()
	if _, err := database.Exec("INSERT INTO tasks(id,backend_id,uid,title,status,dtstamp,created_at) VALUES(1,1,'task-1','Local title','NEEDS-ACTION',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	remote := models.Task{ID: 1, BackendID: 1, UID: "task-1", Title: "Remote title", Status: models.StatusInProcess}
	remoteSnapshot, err := json.Marshal(remote)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO sync_entities(id,backend_id,task_id,remote_uid,state) VALUES(1,1,1,'task-1','active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO sync_conflicts(backend_id,entity_id,task_id,local_snapshot,remote_snapshot,policy,status) VALUES(1,1,1,'{"title":"Local title"}',?,'manual','open')`, string(remoteSnapshot)); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(db.NewTaskRepository(database))
	handler.SetSyncRepository(db.NewSyncRepository(database))
	list := httptest.NewRecorder()
	handler.HandleSyncConflicts(list, httptest.NewRequest(http.MethodGet, "/api/sync/conflicts?status=open", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "Remote title") {
		t.Fatalf("list returned %d: %s", list.Code, list.Body.String())
	}
	resolve := httptest.NewRecorder()
	handler.HandleSyncConflicts(resolve, httptest.NewRequest(http.MethodPatch, "/api/sync/conflicts/1", strings.NewReader(`{"resolution":"remote"}`)))
	if resolve.Code != http.StatusOK || !strings.Contains(resolve.Body.String(), `"status":"resolved"`) {
		t.Fatalf("resolve returned %d: %s", resolve.Code, resolve.Body.String())
	}
	task, err := db.NewTaskRepository(database).Get(1)
	if err != nil || task == nil || task.Title != "Remote title" || task.Status != models.StatusInProcess {
		t.Fatalf("remote resolution not reflected in task: %#v err=%v", task, err)
	}
	for _, method := range []string{http.MethodGet, http.MethodPatch} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/sync/conflicts/999", strings.NewReader(`{"resolution":"local"}`))
		handler.HandleSyncConflicts(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("missing conflict %s returned %d", method, rec.Code)
		}
	}
}

func TestMain(m *testing.M) {
	// Run tests
	code := m.Run()
	os.Exit(code)
}
