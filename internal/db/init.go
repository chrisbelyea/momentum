package db

import (
	"database/sql"
	_ "embed"
	"fmt"
)

//go:embed schema/init.sql
var initSchemaSQL string

// InitializeSchema creates a fresh database or upgrades an older shipped
// shape. The migration ledger prevents a partial schema from being treated as
// current and makes repeated startup safe.
func InitializeSchema(database *sql.DB) error {
	if _, err := database.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return err
	}
	if _, err := database.Exec(`CREATE TABLE IF NOT EXISTS momentum_schema_migrations (version INTEGER PRIMARY KEY, applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		return err
	}
	current, err := currentSchema(database)
	if err != nil {
		return err
	}
	if !current {
		if err := migrateLegacySchema(database); err != nil {
			return fmt.Errorf("migrate database schema: %w", err)
		}
	}
	_, err = database.Exec("INSERT OR REPLACE INTO momentum_schema_migrations(version) VALUES (1)")
	return err
}

func currentSchema(database *sql.DB) (bool, error) {
	for _, item := range []struct{ table, column string }{{"users", "id"}, {"users", "updated_at"}, {"backends", "id"}, {"backends", "backend_type"}, {"backends", "config_encrypted"}, {"backends", "updated_at"}, {"tasks", "id"}, {"tasks", "due_at"}, {"tasks", "tags_json"}, {"tasks", "updated_at"}} {
		var n int
		if err := database.QueryRow("SELECT count(*) FROM pragma_table_info(?) WHERE name=?", item.table, item.column).Scan(&n); err != nil {
			return false, err
		}
		if n != 1 {
			return false, nil
		}
	}
	for _, table := range []string{"users", "backends", "tasks"} {
		var typ string
		if err := database.QueryRow("SELECT type FROM pragma_table_info(?) WHERE name='id'", table).Scan(&typ); err != nil {
			return false, err
		}
		if typ != "INTEGER" {
			return false, nil
		}
	}
	return true, nil
}
