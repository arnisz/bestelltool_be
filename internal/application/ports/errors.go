package ports

import "errors"

var (
	// ErrNotFound signals that an entity does not exist.
	ErrNotFound = errors.New("not found")
	// ErrConflict signals a concurrency or uniqueness conflict.
	ErrConflict = errors.New("conflict")
	// ErrValidation signals invalid input or violated constraints.
	ErrValidation = errors.New("validation")
	// ErrUnauthenticated signals a missing or invalid authentication credential.
	ErrUnauthenticated = errors.New("unauthenticated")
)
