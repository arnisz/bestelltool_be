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
	// ErrCredentialsInvalid signals an invalid username/password combination,
	// unknown user, or disabled account (SEC-03: response is indistinguishable).
	ErrCredentialsInvalid = errors.New("credentials invalid")
	// ErrForbidden signals that an authenticated principal lacks the required role.
	ErrForbidden = errors.New("forbidden")
	// ErrThrottled signals that the operation is rate-limited (retry with Retry-After).
	ErrThrottled = errors.New("throttled")
)
