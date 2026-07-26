package postgres

import (
	"context"
	"fmt"

	"bestelltool_be/internal/domain"
)

type userRepository struct {
	q querier
}

func (r *userRepository) GetByID(ctx context.Context, id domain.UserID) (*domain.User, error) {
	row := r.q.QueryRow(ctx, `
SELECT id, username, role, display_name, email, is_active, version, created_at, updated_at
FROM users
WHERE id = $1`, string(id))

	var u domain.User
	var email *string
	if err := row.Scan(
		&u.ID,
		&u.Username,
		&u.Role,
		&u.DisplayName,
		&email,
		&u.IsActive,
		&u.Version,
		&u.CreatedAt,
		&u.UpdatedAt,
	); err != nil {
		return nil, mapReadError("user", err)
	}
	u.Email = email

	return &u, nil
}

func (r *userRepository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	row := r.q.QueryRow(ctx, `
SELECT id, username, role, display_name, email, is_active, version, created_at, updated_at
FROM users
WHERE username = $1`, username)

	var u domain.User
	var email *string
	if err := row.Scan(
		&u.ID,
		&u.Username,
		&u.Role,
		&u.DisplayName,
		&email,
		&u.IsActive,
		&u.Version,
		&u.CreatedAt,
		&u.UpdatedAt,
	); err != nil {
		return nil, mapReadError("user", err)
	}
	u.Email = email

	return &u, nil
}

// Create inserts a new user. Returns ErrConflict if the id or username
// already exists (uq_users_username, migration 000007).
func (r *userRepository) Create(ctx context.Context, u *domain.User) error {
	if u == nil {
		return fmt.Errorf("user nil: %w", ErrValidation)
	}

	if _, err := r.q.Exec(ctx, `
INSERT INTO users(id, username, role, display_name, email, is_active, version, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		string(u.ID),
		u.Username,
		string(u.Role),
		u.DisplayName,
		u.Email,
		u.IsActive,
		u.Version,
		u.CreatedAt,
		u.UpdatedAt,
	); err != nil {
		return mapWriteError("user", err)
	}

	return nil
}
