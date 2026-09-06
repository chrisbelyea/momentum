package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

//go:embed schema/init.sql
var initSchemaSQL string

//go:embed schema/changelog/002-auth.sql
var authSchemaSQL string

//go:embed schema/changelog/003-vtodo.sql
var vtodoSchemaSQL string

//go:embed schema/changelog/004-sync.sql
var syncSchemaSQL string

//go:embed schema/changelog/005-vtodo-date-only.sql
var dateOnlySchemaSQL string

const schemaVersion = 5

// InitializeSchema creates a fresh database or upgrades an older shipped
// shape. The migration ledger prevents a partial schema from being treated as
// current and makes repeated startup safe.
func InitializeSchema(database *sql.DB) error {
	// SQLite in-memory databases (used by fixtures and release smoke tests)
	// are connection-scoped. Serializing initialization guarantees every
	// schema statement and follow-up migration query observes the same database.
	database.SetMaxOpenConns(1)
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
	if _, err := database.Exec(authSchemaSQL); err != nil {
		return fmt.Errorf("apply authentication schema: %w", err)
	}
	if err := applyVTodoSchema(database); err != nil {
		return fmt.Errorf("apply VTODO schema: %w", err)
	}
	if _, err := database.Exec(syncSchemaSQL); err != nil {
		return fmt.Errorf("apply synchronization schema: %w", err)
	}
	if err := applyDateOnlySchema(database); err != nil {
		return fmt.Errorf("apply date-only schema: %w", err)
	}
	_, err = database.Exec("INSERT OR REPLACE INTO momentum_schema_migrations(version) VALUES (?)", schemaVersion)
	return err
}

func applyDateOnlySchema(database *sql.DB) error {
	var sqlLines []string
	for _, line := range strings.Split(dateOnlySchemaSQL, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			sqlLines = append(sqlLines, line)
		}
	}
	for _, statement := range strings.Split(strings.Join(sqlLines, "\n"), ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		fields := strings.Fields(statement)
		if len(fields) < 6 {
			return fmt.Errorf("invalid date-only migration statement %q", statement)
		}
		column := fields[5]
		var count int
		if err := database.QueryRow("SELECT count(*) FROM pragma_table_info('tasks') WHERE name=?", column).Scan(&count); err != nil {
			return err
		}
		if count == 1 {
			continue
		}
		if _, err := database.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func applyVTodoSchema(database *sql.DB) error {
	// SQLite has no portable ALTER TABLE ... ADD COLUMN IF NOT EXISTS. Apply
	// each changelog statement only when its column is absent, then always run
	// idempotent index statements.
	var sqlLines []string
	for _, line := range strings.Split(vtodoSchemaSQL, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			sqlLines = append(sqlLines, line)
		}
	}
	for _, statement := range strings.Split(strings.Join(sqlLines, "\n"), ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		upper := strings.ToUpper(statement)
		if strings.HasPrefix(upper, "ALTER TABLE TASKS ADD COLUMN ") {
			fields := strings.Fields(statement)
			if len(fields) < 6 {
				return fmt.Errorf("invalid VTODO migration statement %q", statement)
			}
			column := strings.TrimSpace(fields[5])
			var count int
			if err := database.QueryRow("SELECT count(*) FROM pragma_table_info('tasks') WHERE name=?", column).Scan(&count); err != nil {
				return err
			}
			if count == 1 {
				continue
			}
		}
		if _, err := database.Exec(statement); err != nil {
			return err
		}
	}
	return backfillVTodoIdentity(database)
}

func backfillVTodoIdentity(database *sql.DB) error {
	rows, err := database.Query("SELECT id, created_at, updated_at FROM tasks WHERE uid IS NULL OR dtstamp IS NULL OR last_modified IS NULL")
	if err != nil {
		return err
	}
	type legacyTask struct {
		id               int
		created, updated sql.NullTime
	}
	var pending []legacyTask
	for rows.Next() {
		var id int
		var created, updated sql.NullTime
		if err := rows.Scan(&id, &created, &updated); err != nil {
			return err
		}
		pending = append(pending, legacyTask{id: id, created: created, updated: updated})
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range pending {
		id, created, updated := item.id, item.created, item.updated
		now := time.Now().UTC()
		createdAt := now
		if created.Valid {
			createdAt = created.Time
		}
		modified := createdAt
		if updated.Valid {
			modified = updated.Time
		}
		if _, err := database.Exec("UPDATE tasks SET uid=?, dtstamp=?, last_modified=? WHERE id=?", "legacy-"+uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("momentum-task-%d", id))).String(), createdAt, modified, id); err != nil {
			return err
		}
	}
	return rows.Err()
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
