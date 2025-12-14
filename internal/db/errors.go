package db

import "errors"

var (
	// ErrNotFound is returned when a task is not found
	ErrNotFound = errors.New("task not found")
)
