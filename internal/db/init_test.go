package db

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestInitializeSchema_CreatesTablesOnEmptyDB(t *testing.T) {
	database, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer database.Close()

	if err := InitializeSchema(database); err != nil {
		t.Fatalf("InitializeSchema returned unexpected error: %v", err)
	}

	// Verify all three tables exist
	tables := []string{"users", "backends", "tasks"}
	for _, table := range tables {
		var count int
		err := database.QueryRow(
			"SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query sqlite_master for table %q: %v", table, err)
		}
		if count != 1 {
			t.Errorf("Expected table %q to exist after InitializeSchema, got count=%d", table, count)
		}
	}
}

func TestInitializeSchema_TasksTableHasRequiredColumns(t *testing.T) {
	database, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer database.Close()

	if err := InitializeSchema(database); err != nil {
		t.Fatalf("InitializeSchema returned unexpected error: %v", err)
	}

	// Verify all required columns exist in the tasks table
	required := []string{
		"id", "backend_id", "external_id", "title", "description",
		"status", "priority", "due_at", "tags_json", "created_at", "updated_at",
	}

	rows, err := database.Query("SELECT name FROM pragma_table_info('tasks')")
	if err != nil {
		t.Fatalf("Failed to query pragma_table_info: %v", err)
	}
	defer rows.Close()

	found := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("Failed to scan column name: %v", err)
		}
		found[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("Error iterating pragma_table_info: %v", err)
	}

	for _, col := range required {
		if !found[col] {
			t.Errorf("Expected column %q to exist in tasks table, but it was not found", col)
		}
	}
}

func TestInitializeSchema_IdempotentOnExistingSchema(t *testing.T) {
	database, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer database.Close()

	// Initialize once
	if err := InitializeSchema(database); err != nil {
		t.Fatalf("First InitializeSchema call failed: %v", err)
	}

	// Initialize again — should be a no-op and not return an error
	if err := InitializeSchema(database); err != nil {
		t.Fatalf("Second InitializeSchema call failed: %v", err)
	}
}
