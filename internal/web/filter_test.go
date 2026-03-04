package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chrisbelyea/momentum/internal/db"
	"github.com/chrisbelyea/momentum/internal/models"
)

func strPtr(s string) *string { return &s }

// sampleTasks returns a set of tasks used across filter tests.
func sampleTasks() []*models.Task {
	t1 := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 3, 5, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2024, 6, 20, 0, 0, 0, 0, time.UTC)

	tagsA := `["backend","urgent"]`
	tagsB := `["frontend"]`

	return []*models.Task{
		{ID: 1, Title: "Alpha", Status: models.StatusNeedsAction, CreatedAt: t1, TagsJSON: &tagsA},
		{ID: 2, Title: "Beta", Status: models.StatusInProcess, DueAt: &t2, CreatedAt: t2, TagsJSON: &tagsB},
		{ID: 3, Title: "Gamma", Status: models.StatusCompleted, DueAt: &t3, CreatedAt: t3},
		{ID: 4, Title: "Delta", Status: models.StatusNeedsAction, CreatedAt: t1},
	}
}

func TestApplyFilter_NoFilter(t *testing.T) {
	tasks := sampleTasks()
	result := ApplyFilter(tasks, TaskFilter{Sort: "created_at", Order: "desc"})
	if len(result) != len(tasks) {
		t.Errorf("expected %d tasks, got %d", len(tasks), len(result))
	}
}

func TestApplyFilter_ByStatus(t *testing.T) {
	tasks := sampleTasks()
	result := ApplyFilter(tasks, TaskFilter{Status: models.StatusNeedsAction, Sort: "created_at", Order: "desc"})
	for _, task := range result {
		if task.Status != models.StatusNeedsAction {
			t.Errorf("expected only NEEDS-ACTION, got %s", task.Status)
		}
	}
	if len(result) != 2 {
		t.Errorf("expected 2 NEEDS-ACTION tasks, got %d", len(result))
	}
}

func TestApplyFilter_ByTag(t *testing.T) {
	tasks := sampleTasks()
	result := ApplyFilter(tasks, TaskFilter{Tag: "urgent", Sort: "created_at", Order: "desc"})
	if len(result) != 1 || result[0].ID != 1 {
		t.Errorf("expected task 1 only, got ids: %v", taskIDs(result))
	}
}

func TestApplyFilter_ByTagCaseInsensitive(t *testing.T) {
	tasks := sampleTasks()
	result := ApplyFilter(tasks, TaskFilter{Tag: "BACKEND", Sort: "created_at", Order: "desc"})
	if len(result) != 1 || result[0].ID != 1 {
		t.Errorf("expected task 1 only for case-insensitive match, got ids: %v", taskIDs(result))
	}
}

func TestApplyFilter_ByTagNoMatch(t *testing.T) {
	tasks := sampleTasks()
	result := ApplyFilter(tasks, TaskFilter{Tag: "nonexistent", Sort: "created_at", Order: "desc"})
	if len(result) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(result))
	}
}

func TestApplyFilter_SortByTitle_Asc(t *testing.T) {
	tasks := sampleTasks()
	result := ApplyFilter(tasks, TaskFilter{Sort: "title", Order: "asc"})
	expected := []string{"Alpha", "Beta", "Delta", "Gamma"}
	for i, task := range result {
		if task.Title != expected[i] {
			t.Errorf("position %d: expected %s, got %s", i, expected[i], task.Title)
		}
	}
}

func TestApplyFilter_SortByTitle_Desc(t *testing.T) {
	tasks := sampleTasks()
	result := ApplyFilter(tasks, TaskFilter{Sort: "title", Order: "desc"})
	expected := []string{"Gamma", "Delta", "Beta", "Alpha"}
	for i, task := range result {
		if task.Title != expected[i] {
			t.Errorf("position %d: expected %s, got %s", i, expected[i], task.Title)
		}
	}
}

func TestApplyFilter_SortByDueAt_Asc(t *testing.T) {
	tasks := sampleTasks()
	result := ApplyFilter(tasks, TaskFilter{Sort: "due_at", Order: "asc"})
	// tasks without due_at sort last
	if result[len(result)-1].DueAt != nil {
		t.Error("expected nil-DueAt tasks to sort last")
	}
	// first non-nil due_at should be Beta (Mar), then Gamma (Jun)
	if result[0].ID != 2 {
		t.Errorf("expected Beta (ID 2) first by due_at asc, got ID %d", result[0].ID)
	}
	if result[1].ID != 3 {
		t.Errorf("expected Gamma (ID 3) second by due_at asc, got ID %d", result[1].ID)
	}
}

func TestApplyFilter_SortByCreatedAt_Desc(t *testing.T) {
	tasks := sampleTasks()
	result := ApplyFilter(tasks, TaskFilter{Sort: "created_at", Order: "desc"})
	// Gamma (t3=Jun) is the latest
	if result[0].ID != 3 {
		t.Errorf("expected Gamma first by created_at desc, got ID %d", result[0].ID)
	}
}

func TestParseTaskFilter_Defaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/list", nil)
	f := ParseTaskFilter(req)
	if f.Sort != "created_at" {
		t.Errorf("expected default sort 'created_at', got %q", f.Sort)
	}
	if f.Order != "desc" {
		t.Errorf("expected default order 'desc', got %q", f.Order)
	}
}

func TestParseTaskFilter_InvalidSortFallback(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/list?sort=invalid&order=sideways", nil)
	f := ParseTaskFilter(req)
	if f.Sort != "created_at" {
		t.Errorf("invalid sort should fall back to 'created_at', got %q", f.Sort)
	}
	if f.Order != "desc" {
		t.Errorf("invalid order should fall back to 'desc', got %q", f.Order)
	}
}

func TestParseTaskFilter_ValidParams(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/list?status=COMPLETED&tag=urgent&sort=due_at&order=asc", nil)
	f := ParseTaskFilter(req)
	if f.Status != "COMPLETED" {
		t.Errorf("expected status COMPLETED, got %q", f.Status)
	}
	if f.Tag != "urgent" {
		t.Errorf("expected tag 'urgent', got %q", f.Tag)
	}
	if f.Sort != "due_at" {
		t.Errorf("expected sort 'due_at', got %q", f.Sort)
	}
	if f.Order != "asc" {
		t.Errorf("expected order 'asc', got %q", f.Order)
	}
}

func TestHandleList_MethodNotAllowed(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	taskRepo := db.NewTaskRepository(database)
	handler := &Handler{taskRepo: taskRepo, templates: nil}

	req := httptest.NewRequest(http.MethodPost, "/list", bytes.NewBufferString(""))
	rec := httptest.NewRecorder()

	handler.HandleList(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

// taskIDs is a helper that returns a slice of IDs from a task list.
func taskIDs(tasks []*models.Task) []int {
	ids := make([]int, len(tasks))
	for i, t := range tasks {
		ids[i] = t.ID
	}
	return ids
}
