package models

import (
	"database/sql"
	"fmt"
	"time"
)

// Task represents a task (VTODO) in the system
type Task struct {
	ID          int        `json:"id"`
	BackendID   int        `json:"backend_id"`
	ExternalID  *string    `json:"external_id,omitempty"`
	Title       string     `json:"title"`
	Description *string    `json:"description,omitempty"`
	Status      string     `json:"status"`
	Priority    *int       `json:"priority,omitempty"`
	DueAt       *time.Time `json:"due_at,omitempty"`
	TagsJSON    *string    `json:"tags_json,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}

// TaskRow represents a task row from the database with nullable fields
type TaskRow struct {
	ID          int
	BackendID   int
	ExternalID  sql.NullString
	Title       string
	Description sql.NullString
	Status      string
	Priority    sql.NullInt64
	DueAt       sql.NullString
	TagsJSON    sql.NullString
	CreatedAt   string
	UpdatedAt   sql.NullString
}

// ToTask converts a TaskRow to a Task, parsing nullable fields
func (r *TaskRow) ToTask() (*Task, error) {
	task := &Task{
		ID:        r.ID,
		BackendID: r.BackendID,
		Title:     r.Title,
		Status:    r.Status,
	}

	// Handle nullable string fields
	if r.ExternalID.Valid {
		task.ExternalID = &r.ExternalID.String
	}
	if r.Description.Valid {
		task.Description = &r.Description.String
	}
	if r.TagsJSON.Valid {
		task.TagsJSON = &r.TagsJSON.String
	}

	// Handle nullable int fields
	if r.Priority.Valid {
		priority := int(r.Priority.Int64)
		task.Priority = &priority
	}

	// Parse timestamps
	createdAt, err := parseTimestamp(r.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse created_at: %w", err)
	}
	task.CreatedAt = createdAt

	if r.UpdatedAt.Valid {
		updatedAt, err := parseTimestamp(r.UpdatedAt.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse updated_at: %w", err)
		}
		task.UpdatedAt = &updatedAt
	}

	if r.DueAt.Valid {
		dueAt, err := parseTimestamp(r.DueAt.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse due_at: %w", err)
		}
		task.DueAt = &dueAt
	}

	return task, nil
}

// parseTimestamp attempts to parse a timestamp from various formats
func parseTimestamp(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05.999999999Z07:00",
	}

	for _, format := range formats {
		t, err := time.Parse(format, s)
		if err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse timestamp: %s", s)
}

// TaskStatus constants
const (
	StatusNeedsAction = "NEEDS-ACTION"
	StatusInProcess   = "IN-PROCESS"
	StatusCompleted   = "COMPLETED"
	StatusCancelled   = "CANCELLED"
)
