package domain

import "errors"

var (
	// ErrInvalidTransition signals a forbidden state transition.
	ErrInvalidTransition = errors.New("invalid state transition")
	// ErrInvalidState signals an invalid state value.
	ErrInvalidState = errors.New("invalid state")
	// ErrInvalidTimeRange signals an invalid time range.
	ErrInvalidTimeRange = errors.New("invalid time range")
	// ErrReasonRequired signals that a mandatory reason is missing.
	ErrReasonRequired = errors.New("reason required")
	// ErrAlreadyCompleted signals that an entity is already completed.
	ErrAlreadyCompleted = errors.New("entity already completed")
	// ErrRequiredField signals that a required field is missing.
	ErrRequiredField = errors.New("required field missing")
)
