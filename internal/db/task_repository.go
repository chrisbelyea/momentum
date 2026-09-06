package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/chrisbelyea/momentum/internal/models"
)

// TaskRepository handles task database operations
type TaskRepository struct {
	db *sql.DB
}

// NewTaskRepository creates a new TaskRepository
func NewTaskRepository(db *sql.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

// List returns all tasks for a given backend
func (r *TaskRepository) List(backendID int) ([]*models.Task, error) {
	query := `
		SELECT id, backend_id, external_id, title, description, status, 
		       priority, due_at, tags_json, created_at, updated_at
		FROM tasks
		WHERE backend_id = ?
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(query, backendID)
	if err != nil {
		return nil, fmt.Errorf("failed to query tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*models.Task
	for rows.Next() {
		taskRow := &models.TaskRow{}
		err := rows.Scan(
			&taskRow.ID,
			&taskRow.BackendID,
			&taskRow.ExternalID,
			&taskRow.Title,
			&taskRow.Description,
			&taskRow.Status,
			&taskRow.Priority,
			&taskRow.DueAt,
			&taskRow.TagsJSON,
			&taskRow.CreatedAt,
			&taskRow.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan task: %w", err)
		}

		task, err := taskRow.ToTask()
		if err != nil {
			return nil, fmt.Errorf("failed to convert task row: %w", err)
		}
		tasks = append(tasks, task)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tasks: %w", err)
	}

	return tasks, nil
}

func (r *TaskRepository) ListForUser(backendID, userID int) ([]*models.Task, error) {
	if !r.backendOwned(backendID, userID) {
		return []*models.Task{}, nil
	}
	return r.List(backendID)
}
func (r *TaskRepository) GetForUser(id, userID int) (*models.Task, error) {
	t, err := r.Get(id)
	if err != nil || t == nil {
		return t, err
	}
	if !r.backendOwned(t.BackendID, userID) {
		return nil, nil
	}
	return t, nil
}
func (r *TaskRepository) UpdateForUser(task *models.Task, userID int) error {
	if !r.backendOwned(task.BackendID, userID) {
		return ErrNotFound
	}
	return r.Update(task)
}
func (r *TaskRepository) DeleteForUser(id, userID int) error {
	t, err := r.Get(id)
	if err != nil || t == nil || !r.backendOwned(t.BackendID, userID) {
		return ErrNotFound
	}
	return r.Delete(id)
}
func (r *TaskRepository) backendOwned(backendID, userID int) bool {
	var n int
	return r.db.QueryRow("SELECT COUNT(1) FROM backends WHERE id=? AND user_id=?", backendID, userID).Scan(&n) == nil && n == 1
}
func (r *TaskRepository) BackendOwned(backendID, userID int) bool {
	return r.backendOwned(backendID, userID)
}
func (r *TaskRepository) DefaultBackendForUser(userID int) (int, error) {
	var id int
	err := r.db.QueryRow("SELECT id FROM backends WHERE user_id=? ORDER BY id LIMIT 1", userID).Scan(&id)
	return id, err
}

// Get returns a single task by ID
func (r *TaskRepository) Get(id int) (*models.Task, error) {
	query := `
		SELECT id, backend_id, external_id, title, description, status, 
		       priority, due_at, tags_json, created_at, updated_at
		FROM tasks
		WHERE id = ?
	`

	taskRow := &models.TaskRow{}
	err := r.db.QueryRow(query, id).Scan(
		&taskRow.ID,
		&taskRow.BackendID,
		&taskRow.ExternalID,
		&taskRow.Title,
		&taskRow.Description,
		&taskRow.Status,
		&taskRow.Priority,
		&taskRow.DueAt,
		&taskRow.TagsJSON,
		&taskRow.CreatedAt,
		&taskRow.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	task, err := taskRow.ToTask()
	if err != nil {
		return nil, fmt.Errorf("failed to convert task row: %w", err)
	}

	return task, nil
}

// Create creates a new task
func (r *TaskRepository) Create(task *models.Task) error {
	// Set timestamps
	now := time.Now()
	task.CreatedAt = now
	task.UpdatedAt = &now

	query := `
		INSERT INTO tasks (
			backend_id, external_id, title, description, status,
			priority, due_at, tags_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := r.db.Exec(
		query,
		task.BackendID,
		task.ExternalID,
		task.Title,
		task.Description,
		task.Status,
		task.Priority,
		task.DueAt,
		task.TagsJSON,
		task.CreatedAt,
		task.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create task: %w", err)
	}

	// Get the auto-generated ID
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	task.ID = int(id)

	return nil
}

// Update updates an existing task
func (r *TaskRepository) Update(task *models.Task) error {
	// Update timestamp
	now := time.Now()
	task.UpdatedAt = &now

	query := `
		UPDATE tasks
		SET backend_id = ?, external_id = ?, title = ?, description = ?,
		    status = ?, priority = ?, due_at = ?, tags_json = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := r.db.Exec(
		query,
		task.BackendID,
		task.ExternalID,
		task.Title,
		task.Description,
		task.Status,
		task.Priority,
		task.DueAt,
		task.TagsJSON,
		task.UpdatedAt,
		task.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// Delete deletes a task by ID
func (r *TaskRepository) Delete(id int) error {
	query := `DELETE FROM tasks WHERE id = ?`

	result, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete task: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}
