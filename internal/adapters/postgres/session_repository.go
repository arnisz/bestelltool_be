package postgres

import (
	"context"
	"fmt"
	"time"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
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

func (r *sessionRepository) Update(ctx context.Context, session *ports.Session) error {
	if session == nil {
		return fmt.Errorf("session nil: %w", ErrValidation)
	}
	if _, err := r.q.Exec(ctx, `
UPDATE sessions
SET token_hash = $2, expires_at = $3, revoked_at = $4
WHERE id = $1`, session.ID, session.TokenHash, session.ExpiresAt, session.RevokedAt); err != nil {
		return mapWriteError("session_update", err)
	}
	return nil
}

func (r *sessionRepository) GetByID(ctx context.Context, id string) (*ports.Session, error) {
	return r.get(ctx, id, "")
}

func (r *sessionRepository) GetByTokenHash(ctx context.Context, tokenHash []byte) (*ports.Session, error) {
	row := r.q.QueryRow(ctx, `
SELECT id, user_id, active_role, token_hash, created_at, expires_at, revoked_at
FROM sessions
WHERE token_hash = $1`, tokenHash)

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
		return nil, mapReadError("session_by_token_hash", err)
	}

	return &session, nil
}

func (r *sessionRepository) GetForUpdate(ctx context.Context, id string) (*ports.Session, error) {
	return r.get(ctx, id, " FOR UPDATE")
}

func (r *sessionRepository) get(ctx context.Context, id, lockClause string) (*ports.Session, error) {
	row := r.q.QueryRow(ctx, `
SELECT id, user_id, active_role, token_hash, created_at, expires_at, revoked_at
FROM sessions
WHERE id = $1`+lockClause, id)

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
	ct, err := r.q.Exec(ctx, `
UPDATE sessions
SET revoked_at = $2
WHERE id = $1`, id, at)
	if err != nil {
		return mapWriteError("session_revoke", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("session_revoke: %w", ErrNotFound)
	}

	return nil
}

func (r *sessionRepository) RevokeAllForUserExcept(ctx context.Context, userID domain.UserID, exceptSessionID string, at time.Time) error {
	if _, err := r.q.Exec(ctx, `
UPDATE sessions
SET revoked_at = $3
WHERE user_id = $1
  AND id <> $2
  AND revoked_at IS NULL`, string(userID), exceptSessionID, at); err != nil {
		return mapWriteError("user_sessions_revoke", err)
	}

	return nil
}
