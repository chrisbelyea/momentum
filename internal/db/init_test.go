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
