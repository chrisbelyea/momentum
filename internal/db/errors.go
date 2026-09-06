package db

import "errors"

var (
	// ErrNotFound is returned when a task is not found
	ErrNotFound = errors.New("task not found")
	// Conflict resolution errors are exported so HTTP handlers can distinguish
	// invalid user choices from storage failures without matching strings.
	ErrInvalidConflictResolution = errors.New("invalid sync conflict resolution")
	ErrConflictAlreadyResolved   = errors.New("sync conflict already resolved")
	ErrConflictNoTask            = errors.New("sync conflict has no local task")
	ErrInvalidConflictSnapshot   = errors.New("invalid sync conflict snapshot")
)
