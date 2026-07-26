package postgres

import (
	"context"
	"fmt"
	"time"

	"bestelltool_be/internal/application/ports"
)

type sessionRepository struct {
	q querier
}

func (r *sessionRepository) Save(ctx context.Context, session *ports.Session) error {
	if session == nil {
		return fmt.Errorf("session nil: %w", ErrValidation)
	}

	if _, err := r.q.Exec(ctx, `
INSERT INTO sessions(id, user_id, active_role, token_hash, created_at, expires_at, revoked_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		session.ID,
		string(session.UserID),
		string(session.ActiveRole),
		session.TokenHash,
		session.CreatedAt,
		session.ExpiresAt,
		session.RevokedAt,
	); err != nil {
		return mapWriteError("session", err)
	}

	return nil
}

func (r *sessionRepository) GetByID(ctx context.Context, id string) (*ports.Session, error) {
	row := r.q.QueryRow(ctx, `
SELECT id, user_id, active_role, token_hash, created_at, expires_at, revoked_at
FROM sessions
WHERE id = $1`, id)

	var session ports.Session
	if err := row.Scan(
		&session.ID,
		&session.UserID,
		&session.ActiveRole,
		&session.TokenHash,
		&session.CreatedAt,
		&session.ExpiresAt,
		&session.RevokedAt,
	); err != nil {
		return nil, mapReadError("session", err)
	}

	return &session, nil
}

func (r *sessionRepository) Revoke(ctx context.Context, id string, at time.Time) error {
	if _, err := r.q.Exec(ctx, `
UPDATE sessions
SET revoked_at = $2
WHERE id = $1`, id, at); err != nil {
		return mapWriteError("session_revoke", err)
	}

	return nil
}
