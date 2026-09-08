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
	"time"

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

func TestTaskWorkflowAPIRoundTripsRichFieldsAndPreservesPartialUpdates(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()
	handler := &Handler{taskRepo: db.NewTaskRepository(database)}

	createBody := `{"title":"Plan launch","backend_id":1,"description":"Write the launch checklist","status":"IN-PROCESS","priority":3,"due_at":"2026-12-31T00:00:00Z","due_date_only":true,"tags_json":"[\"launch\",\"urgent\"]"}`
	created := httptest.NewRecorder()
	handler.HandleTasks(created, httptest.NewRequest(http.MethodPost, "/api/tasks", strings.NewReader(createBody)))
	if created.Code != http.StatusCreated {
		t.Fatalf("create returned %d: %s", created.Code, created.Body.String())
	}
	var task models.Task
	if err := json.NewDecoder(created.Body).Decode(&task); err != nil {
		t.Fatalf("decode created task: %v", err)
	}
	if task.Description == nil || *task.Description != "Write the launch checklist" || task.Priority == nil || *task.Priority != 3 {
		t.Fatalf("rich fields missing from created task: %+v", task)
	}
	if task.DueAt == nil || task.DueAt.Format("2006-01-02") != "2026-12-31" || !task.DueDateOnly {
		t.Fatalf("due date missing from created task: %+v", task)
	}
	if task.TagsJSON == nil || *task.TagsJSON != `["launch","urgent"]` {
		t.Fatalf("tags missing from created task: %+v", task.TagsJSON)
	}

	// Older clients send only a title. The update endpoint must not erase rich
	// fields that a client did not include in its partial update.
	updated := httptest.NewRecorder()
	handler.HandleTasks(updated, httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/tasks/%d", task.ID), strings.NewReader(`{"title":"Plan launch v2"}`)))
	if updated.Code != http.StatusOK {
		t.Fatalf("partial update returned %d: %s", updated.Code, updated.Body.String())
	}
	got, err := handler.taskRepo.Get(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Title != "Plan launch v2" || got.Description == nil || *got.Description != "Write the launch checklist" || got.Priority == nil || *got.Priority != 3 || got.DueAt == nil || got.TagsJSON == nil {
		t.Fatalf("partial update erased rich fields: %+v", got)
	}

	// Explicit null values clear optional editor fields.
	cleared := httptest.NewRecorder()
	handler.HandleTasks(cleared, httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/tasks/%d", task.ID), strings.NewReader(`{"title":"Plan launch final","description":null,"priority":null,"due_at":null,"due_date_only":false,"tags_json":null}`)))
	if cleared.Code != http.StatusOK {
		t.Fatalf("clear update returned %d: %s", cleared.Code, cleared.Body.String())
	}
	got, err = handler.taskRepo.Get(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != nil || got.Priority != nil || got.DueAt != nil || got.DueDateOnly || got.TagsJSON != nil {
		t.Fatalf("explicit nulls did not clear rich fields: %+v", got)
	}
}

func TestTaskWorkflowAPIRejectsStaleConditionalMutation(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()
	handler := &Handler{taskRepo: db.NewTaskRepository(database)}

	created := httptest.NewRecorder()
	handler.HandleTasks(created, httptest.NewRequest(http.MethodPost, "/api/tasks", strings.NewReader(`{"title":"Shared task","backend_id":1}`)))
	if created.Code != http.StatusCreated {
		t.Fatalf("create returned %d: %s", created.Code, created.Body.String())
	}
	var task models.Task
	if err := json.NewDecoder(created.Body).Decode(&task); err != nil || task.ID == 0 || created.Header().Get("ETag") == "" {
		t.Fatalf("create did not return a versioned task: %+v etag=%q err=%v", task, created.Header().Get("ETag"), err)
	}
	staleVersion := taskVersion(&task)

	// A separate client changes the task after the first client rendered it.
	fresh := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/tasks/%d", task.ID), strings.NewReader(`{"title":"Changed on another device"}`))
	fresh.Header.Set("If-Match", `"`+staleVersion+`"`)
	freshResponse := httptest.NewRecorder()
	handler.HandleTasks(freshResponse, fresh)
	if freshResponse.Code != http.StatusOK {
		t.Fatalf("fresh conditional update returned %d: %s", freshResponse.Code, freshResponse.Body.String())
	}

	stale := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/tasks/%d", task.ID), strings.NewReader(`{"title":"Stale overwrite"}`))
	stale.Header.Set("If-Match", `"`+staleVersion+`"`)
	staleResponse := httptest.NewRecorder()
	handler.HandleTasks(staleResponse, stale)
	if staleResponse.Code != http.StatusConflict {
		t.Fatalf("stale conditional update returned %d: %s", staleResponse.Code, staleResponse.Body.String())
	}
	got, err := handler.taskRepo.Get(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Title != "Changed on another device" {
		t.Fatalf("stale update overwrote newer task: %+v", got)
	}
}

func TestTaskWorkflowAPIValidatesRichFields(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want string
	}{
		{name: "priority", body: `{"title":"Bad priority","backend_id":1,"priority":10}`, want: "priority"},
		{name: "status", body: `{"title":"Bad status","backend_id":1,"status":"UNKNOWN"}`, want: "status"},
		{name: "tags", body: `{"title":"Bad tags","backend_id":1,"tags_json":"not-json"}`, want: "tags_json"},
		{name: "date-only", body: `{"title":"Missing date","backend_id":1,"due_date_only":true}`, want: "due_date_only"},
	} {
		t.Run(test.name, func(t *testing.T) {
			database := setupTestDB(t)
			defer database.Close()
			handler := &Handler{taskRepo: db.NewTaskRepository(database)}
			rec := httptest.NewRecorder()
			handler.HandleTasks(rec, httptest.NewRequest(http.MethodPost, "/api/tasks", strings.NewReader(test.body)))
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), test.want) {
				t.Fatalf("expected 400 mentioning %q, got %d: %s", test.want, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestBoardRendersAccessibleRichTaskEditor(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()
	description := "Review launch checklist"
	priority := 2
	due := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	tags := `["launch","urgent"]`
	task := &models.Task{BackendID: 1, Title: "Release task", Description: &description, Priority: &priority, DueAt: &due, DueDateOnly: true, TagsJSON: &tags, Status: models.StatusNeedsAction}
	repo := db.NewTaskRepository(database)
	if err := repo.Create(task); err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(repo, db.NewBackendRepository(database))
	rec := httptest.NewRecorder()
	handler.HandleIndex(rec, httptest.NewRequest(http.MethodGet, "/?backend_id=1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("board returned %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, required := range []string{
		`id="task-description"`, `id="task-due"`, `id="task-priority"`, `id="task-tags"`,
		`id="edit-description"`, `id="edit-due"`, `id="edit-priority"`, `id="edit-tags"`,
		`data-description="Review launch checklist"`, `data-due-at="2026-12-31"`, `data-priority="2"`,
		`description: description || null`, `due_date_only: Boolean(due)`, `tags_json: tags.length ? JSON.stringify(tags) : null`,
		`<input type="hidden" name="backend_id" value="1">`, `href="/?backend_id=1"`,
	} {
		if !strings.Contains(body, required) {
			t.Errorf("board is missing rich editor invariant %q", required)
		}
	}
	if strings.Contains(body, "alert(") {
		t.Fatal("board mutations must announce errors through ARIA live regions, not alert()")
	}
}

func TestListFilterPreservesSelectedBackend(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()
	if _, err := database.Exec("INSERT INTO backends (id,user_id,backend_type,name) VALUES (2,1,'internal','Second backend')"); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(db.NewTaskRepository(database), db.NewBackendRepository(database))
	rec := httptest.NewRecorder()
	handler.HandleList(rec, httptest.NewRequest(http.MethodGet, "/list?backend_id=2&status=IN-PROCESS&tag=launch", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list returned %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, required := range []string{`<input type="hidden" name="backend_id" value="2">`, `href="/list?backend_id=2"`} {
		if !strings.Contains(body, required) {
			t.Errorf("list filter is missing backend state %q", required)
		}
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
