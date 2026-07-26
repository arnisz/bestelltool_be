package domain

import (
	"fmt"
	"time"
)

// User is the business person behind requests, allocations and audit events.
// This mirrors the `users` table shape since migration 000007: `id` is the
// stable internal identifier, `username` is the human-chosen, unique login
// name (backfilled from `id` for pre-existing rows), `email` is optional,
// and `version` supports optimistic locking for a future Update path.
type User struct {
	ID          UserID
	Username    string
	Role        ActorRole
	DisplayName string
	Email       *string
	IsActive    bool
	Version     int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NewUser creates a user in a valid initial state (active, single role,
// version 1). Role must be one of the roles a person can hold;
// ActorRoleSystem is rejected because the system role is never assignable to
// a person (SEC-17).
func NewUser(id UserID, username string, role ActorRole, displayName string, email *string, createdAt time.Time) (*User, error) {
	if id == "" {
		return nil, fmt.Errorf("user id: %w", ErrRequiredField)
	}
	if username == "" {
		return nil, fmt.Errorf("username: %w", ErrRequiredField)
	}
	if displayName == "" {
		return nil, fmt.Errorf("user display name: %w", ErrRequiredField)
	}
	if !isAssignablePersonRole(role) {
		return nil, fmt.Errorf("user role %q: %w", role, ErrInvalidState)
	}

	return &User{
		ID:          id,
		Username:    username,
		Role:        role,
		DisplayName: displayName,
		Email:       email,
		IsActive:    true,
		Version:     1,
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
	}, nil
}

func isAssignablePersonRole(role ActorRole) bool {
	switch role {
	case ActorRoleTechnician, ActorRoleDispatcher, ActorRoleAdmin:
		return true
	default:
		return false
	}
}
