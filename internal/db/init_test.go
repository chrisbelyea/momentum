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

	// Verify all production tables exist, including authentication state.
	tables := []string{"users", "backends", "tasks", "credentials", "sessions", "momentum_schema_migrations"}
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

func TestInitializeSchema_UpgradesLegacyStringIDSchemaWithoutDataLoss(t *testing.T) {
	database, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	legacy := `
CREATE TABLE users (id VARCHAR(36) PRIMARY KEY, email TEXT NOT NULL UNIQUE, created_at TIMESTAMP);
CREATE TABLE backends (id VARCHAR(36) PRIMARY KEY, user_id VARCHAR(36) NOT NULL, backend_type TEXT NOT NULL, name TEXT NOT NULL, config_encrypted TEXT, created_at TIMESTAMP, updated_at TIMESTAMP);
CREATE TABLE tasks (id VARCHAR(36) PRIMARY KEY, backend_id VARCHAR(36) NOT NULL, external_id TEXT, title TEXT NOT NULL, description TEXT, status TEXT NOT NULL, priority INTEGER, due_date TIMESTAMP, vtodo_data TEXT, sync_status TEXT, last_synced_at TIMESTAMP, created_at TIMESTAMP, updated_at TIMESTAMP);`
	if _, err := database.Exec(legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("INSERT INTO users VALUES ('user-1','legacy@example.com',CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("INSERT INTO backends VALUES ('backend-1','user-1','internal','Legacy backend',NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("INSERT INTO tasks VALUES ('task-1','backend-1',NULL,'Legacy task',NULL,'NEEDS-ACTION',1,NULL,'[\"legacy\"]','synced',NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	if err := InitializeSchema(database); err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}
	var id, backendID int
	var tags string
	if err := database.QueryRow("SELECT id, backend_id, tags_json FROM tasks WHERE title='Legacy task'").Scan(&id, &backendID, &tags); err != nil {
		t.Fatalf("migrated task missing: %v", err)
	}
	if id == 0 || backendID == 0 || tags != "[\"legacy\"]" {
		t.Fatalf("unexpected migrated task: id=%d backend=%d tags=%q", id, backendID, tags)
	}
	if err := InitializeSchema(database); err != nil {
		t.Fatalf("idempotent upgraded startup failed: %v", err)
	}
}

func TestInitializeSchema_UpgradesPreviouslyShippedIntegerSchema(t *testing.T) {
	database, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	legacy := `
CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, email TEXT NOT NULL UNIQUE, created_at TIMESTAMP);
CREATE TABLE backends (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER NOT NULL, backend_type TEXT NOT NULL, name TEXT NOT NULL, config_json TEXT);
CREATE TABLE tasks (id INTEGER PRIMARY KEY AUTOINCREMENT, backend_id INTEGER NOT NULL, external_id TEXT, title TEXT NOT NULL, description TEXT, status TEXT NOT NULL, priority INTEGER, due_at TIMESTAMP, tags_json TEXT, created_at TIMESTAMP, updated_at TIMESTAMP);`
	if _, err := database.Exec(legacy + "INSERT INTO users(id,email) VALUES(1,'old@example.com'); INSERT INTO backends(id,user_id,backend_type,name) VALUES(1,1,'internal','Old'); INSERT INTO tasks(backend_id,title,status) VALUES(1,'Old task','NEEDS-ACTION');"); err != nil {
		t.Fatal(err)
	}
	if err := InitializeSchema(database); err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}
	var count int
	if err := database.QueryRow("SELECT count(*) FROM tasks WHERE title='Old task'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("task lost during upgrade: count=%d err=%v", count, err)
	}
}
