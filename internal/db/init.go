package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log"
)

//go:embed schema/init.sql
var initSchemaSQL string

// InitializeSchema checks whether the database schema exists and creates it if
// not. This enables zero-configuration first run: when the database file is new
// or empty the schema is automatically initialized from the embedded SQL.
//
// This function is SQLite-specific (it queries sqlite_master) and must be
// called before any other database operations, from a single goroutine during
// startup. It uses the presence of the 'tasks' table as a proxy for the full
// schema because tasks is the last table created in the changeset sequence.
func InitializeSchema(database *sql.DB) error {
	var count int
	err := database.QueryRow(
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND name='tasks'",
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check schema: %w", err)
	}

	if count > 0 {
		return nil
	}

	log.Println("Initializing database schema...")
	if _, err := database.Exec(initSchemaSQL); err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}
	log.Println("✓ Database schema initialized successfully")
	return nil
}
