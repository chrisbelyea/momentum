package db

import (
	"database/sql"
	"fmt"
)

// migrateLegacySchema preserves rows while rebuilding all three tables. Older
// releases used both UUID/text IDs and incomplete integer schemas, so IDs are
// remapped in dependency order rather than copied unsafely.
func migrateLegacySchema(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var tables int
	if err = tx.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('users','backends','tasks')").Scan(&tables); err != nil {
		return err
	}
	if tables == 0 {
		if _, err = tx.Exec(initSchemaSQL); err != nil {
			return err
		}
		return tx.Commit()
	}
	if tables != 3 {
		return fmt.Errorf("unsupported partial legacy schema; expected users, backends, and tasks")
	}
	if _, err = tx.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		return err
	}
	for _, name := range []string{"tasks", "backends", "users"} {
		if _, err = tx.Exec("ALTER TABLE " + name + " RENAME TO " + name + "_legacy"); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(initSchemaSQL); err != nil {
		return err
	}
	users := map[string]int{}
	rows, err := tx.Query("SELECT id,email,created_at FROM users_legacy")
	if err != nil {
		return err
	}
	for rows.Next() {
		var old, email string
		var created sql.NullString
		if err = rows.Scan(&old, &email, &created); err != nil {
			return err
		}
		r, e := tx.Exec("INSERT INTO users(email,created_at,updated_at) VALUES(?,COALESCE(?,CURRENT_TIMESTAMP),COALESCE(?,CURRENT_TIMESTAMP))", email, created, created)
		if e != nil {
			return e
		}
		id, _ := r.LastInsertId()
		users[old] = int(id)
	}
	if err = rows.Close(); err != nil {
		return err
	}
	backends := map[string]int{}
	var backendCreatedColumns int
	if err = tx.QueryRow("SELECT count(*) FROM pragma_table_info('backends_legacy') WHERE name='created_at'").Scan(&backendCreatedColumns); err != nil {
		return err
	}
	backendCreated := "NULL"
	if backendCreatedColumns == 1 {
		backendCreated = "created_at"
	}
	rows, err = tx.Query("SELECT id,user_id,backend_type,name," + backendCreated + " FROM backends_legacy")
	if err != nil {
		return err
	}
	for rows.Next() {
		var old, user, typ, name string
		var created sql.NullString
		if err = rows.Scan(&old, &user, &typ, &name, &created); err != nil {
			return err
		}
		uid, ok := users[user]
		if !ok {
			return fmt.Errorf("backend %s has no user", old)
		}
		r, e := tx.Exec("INSERT INTO backends(user_id,backend_type,name,created_at,updated_at) VALUES(?,?,?,COALESCE(?,CURRENT_TIMESTAMP),COALESCE(?,CURRENT_TIMESTAMP))", uid, typ, name, created, created)
		if e != nil {
			return e
		}
		id, _ := r.LastInsertId()
		backends[old] = int(id)
	}
	if err = rows.Close(); err != nil {
		return err
	}
	// Both prior layouts have these common columns; due/tag fields are selected
	// dynamically because v0.1.3 used due_date/vtodo_data.
	due, tags := "due_date", "vtodo_data"
	var n int
	if err = tx.QueryRow("SELECT count(*) FROM pragma_table_info('tasks_legacy') WHERE name='due_at'").Scan(&n); err != nil {
		return err
	}
	if n == 1 {
		due = "due_at"
	}
	if err = tx.QueryRow("SELECT count(*) FROM pragma_table_info('tasks_legacy') WHERE name='tags_json'").Scan(&n); err != nil {
		return err
	}
	if n == 1 {
		tags = "tags_json"
	}
	rows, err = tx.Query("SELECT id,backend_id,external_id,title,description,status,priority," + due + "," + tags + ",created_at FROM tasks_legacy")
	if err != nil {
		return err
	}
	for rows.Next() {
		var old, b string
		var ext, desc, dueV, tagsV, created sql.NullString
		var title, status string
		var priority sql.NullInt64
		if err = rows.Scan(&old, &b, &ext, &title, &desc, &status, &priority, &dueV, &tagsV, &created); err != nil {
			return err
		}
		bid, ok := backends[b]
		if !ok {
			return fmt.Errorf("task %s has no backend", old)
		}
		if _, err = tx.Exec("INSERT INTO tasks(backend_id,external_id,title,description,status,priority,due_at,tags_json,created_at) VALUES(?,?,?,?,?,?,?,?,COALESCE(?,CURRENT_TIMESTAMP))", bid, ext, title, desc, status, priority, dueV, tagsV, created); err != nil {
			return err
		}
	}
	if err = rows.Close(); err != nil {
		return err
	}
	for _, name := range []string{"tasks_legacy", "backends_legacy", "users_legacy"} {
		if _, err = tx.Exec("DROP TABLE " + name); err != nil {
			return err
		}
	}
	return tx.Commit()
}
