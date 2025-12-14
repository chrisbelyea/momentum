package models

import (
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

// TaskStatus constants
const (
	StatusNeedsAction = "NEEDS-ACTION"
	StatusInProcess   = "IN-PROCESS"
	StatusCompleted   = "COMPLETED"
	StatusCancelled   = "CANCELLED"
)

// SyncStatus constants
const (
	SyncStatusSynced  = "synced"
	SyncStatusPending = "pending"
	SyncStatusError   = "error"
)
