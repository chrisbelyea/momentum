package models

import "errors"

var (
	// ErrInvalidBackend is returned when a backend configuration is invalid
	ErrInvalidBackend = errors.New("invalid backend configuration")
)
