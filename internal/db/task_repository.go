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
	task.CreatedAt = time.Now()
	now := time.Now()
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
		return fmt.Errorf("task not found: %d", task.ID)
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
		return fmt.Errorf("task not found: %d", id)
	}

	return nil
}
