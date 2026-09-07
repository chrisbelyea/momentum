package db

import "errors"

var (
	// ErrNotFound is returned when a task is not found
	ErrNotFound = errors.New("task not found")
	// ErrTaskVersionConflict indicates that a task changed after a client read it.
	// Callers can use this to reconcile an optimistic browser update instead of
	// silently overwriting another device's change.
	ErrTaskVersionConflict = errors.New("task changed since it was read")
	// Conflict resolution errors are exported so HTTP handlers can distinguish
	// invalid user choices from storage failures without matching strings.
	ErrInvalidConflictResolution = errors.New("invalid sync conflict resolution")
	ErrConflictAlreadyResolved   = errors.New("sync conflict already resolved")
	ErrConflictNoTask            = errors.New("sync conflict has no local task")
	ErrInvalidConflictSnapshot   = errors.New("invalid sync conflict snapshot")
)
