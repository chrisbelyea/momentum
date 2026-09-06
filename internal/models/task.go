package models

import (
	"database/sql"
	"fmt"
	"time"
)

// Task represents a task (VTODO) in the system
type Task struct {
	ID              int        `json:"id"`
	BackendID       int        `json:"backend_id"`
	UID             string     `json:"uid"`
	ExternalID      *string    `json:"external_id,omitempty"`
	Title           string     `json:"title"`
	Description     *string    `json:"description,omitempty"`
	Status          string     `json:"status"`
	Priority        *int       `json:"priority,omitempty"`
	DueAt           *time.Time `json:"due_at,omitempty"`
	DueDateOnly     bool       `json:"due_date_only,omitempty"`
	StartAt         *time.Time `json:"start_at,omitempty"`
	StartDateOnly   bool       `json:"start_date_only,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	DTStamp         time.Time  `json:"dtstamp"`
	LastModified    *time.Time `json:"last_modified,omitempty"`
	Sequence        int        `json:"sequence"`
	PercentComplete *int       `json:"percent_complete,omitempty"`
	TagsJSON        *string    `json:"tags_json,omitempty"`
	RelatedToJSON   *string    `json:"related_to_json,omitempty"`
	URL             *string    `json:"url,omitempty"`
	Location        *string    `json:"location,omitempty"`
	ExtraJSON       *string    `json:"extra_json,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

// TaskRow represents a task row from the database with nullable fields
type TaskRow struct {
	ID              int
	BackendID       int
	UID             sql.NullString
	ExternalID      sql.NullString
	Title           string
	Description     sql.NullString
	Status          string
	Priority        sql.NullInt64
	DueAt           sql.NullString
	DueDateOnly     sql.NullInt64
	StartAt         sql.NullString
	StartDateOnly   sql.NullInt64
	CompletedAt     sql.NullString
	DTStamp         sql.NullString
	LastModified    sql.NullString
	Sequence        int
	PercentComplete sql.NullInt64
	TagsJSON        sql.NullString
	RelatedToJSON   sql.NullString
	URL             sql.NullString
	Location        sql.NullString
	ExtraJSON       sql.NullString
	CreatedAt       string
	UpdatedAt       sql.NullString
}

// ToTask converts a TaskRow to a Task, parsing nullable fields
func (r *TaskRow) ToTask() (*Task, error) {
	task := &Task{
		ID:            r.ID,
		BackendID:     r.BackendID,
		UID:           r.UID.String,
		Title:         r.Title,
		Status:        r.Status,
		Sequence:      r.Sequence,
		DueDateOnly:   r.DueDateOnly.Valid && r.DueDateOnly.Int64 != 0,
		StartDateOnly: r.StartDateOnly.Valid && r.StartDateOnly.Int64 != 0,
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
	if r.RelatedToJSON.Valid {
		task.RelatedToJSON = &r.RelatedToJSON.String
	}
	if r.URL.Valid {
		task.URL = &r.URL.String
	}
	if r.Location.Valid {
		task.Location = &r.Location.String
	}
	if r.ExtraJSON.Valid {
		task.ExtraJSON = &r.ExtraJSON.String
	}
	if r.PercentComplete.Valid {
		v := int(r.PercentComplete.Int64)
		task.PercentComplete = &v
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
	for _, field := range []struct {
		value sql.NullString
		dst   **time.Time
		name  string
	}{{r.StartAt, &task.StartAt, "start_at"}, {r.CompletedAt, &task.CompletedAt, "completed_at"}, {r.LastModified, &task.LastModified, "last_modified"}} {
		if field.value.Valid {
			v, err := parseTimestamp(field.value.String)
			if err != nil {
				return nil, fmt.Errorf("failed to parse %s: %w", field.name, err)
			}
			*field.dst = &v
		}
	}
	if r.DTStamp.Valid {
		v, err := parseTimestamp(r.DTStamp.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse dtstamp: %w", err)
		}
		task.DTStamp = v
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
