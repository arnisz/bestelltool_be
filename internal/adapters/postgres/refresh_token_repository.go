package postgres

import (
	"context"
	"fmt"

	"bestelltool_be/internal/application/ports"
)

type refreshTokenRepository struct {
	q querier
}

func (r *refreshTokenRepository) Save(ctx context.Context, token *ports.RefreshToken) error {
	if token == nil {
		return fmt.Errorf("refresh token nil: %w", ErrValidation)
	}

	if _, err := r.q.Exec(ctx, `
INSERT INTO refresh_tokens(id, session_id, token_hash, family_id, successor_token_id, encrypted_successor, created_at, expires_at, revoked_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		token.ID,
		token.SessionID,
		token.TokenHash,
		token.FamilyID,
		token.SuccessorTokenID,
		token.EncryptedSuccessor,
		token.CreatedAt,
		token.ExpiresAt,
		token.RevokedAt,
	); err != nil {
		return mapWriteError("refresh_token", err)
	}

	return nil
}

func (r *refreshTokenRepository) GetByID(ctx context.Context, id string) (*ports.RefreshToken, error) {
	return r.get(ctx, id, "")
}

func (r *refreshTokenRepository) GetForUpdate(ctx context.Context, id string) (*ports.RefreshToken, error) {
	return r.get(ctx, id, " FOR UPDATE")
}

func (r *refreshTokenRepository) get(ctx context.Context, id, lockClause string) (*ports.RefreshToken, error) {
	row := r.q.QueryRow(ctx, `
SELECT id, session_id, token_hash, family_id, successor_token_id, encrypted_successor, created_at, expires_at, revoked_at
FROM refresh_tokens
WHERE id = $1`+lockClause, id)

	var token ports.RefreshToken
	if err := row.Scan(
		&token.ID,
		&token.SessionID,
		&token.TokenHash,
		&token.FamilyID,
		&token.SuccessorTokenID,
		&token.EncryptedSuccessor,
		&token.CreatedAt,
		&token.ExpiresAt,
		&token.RevokedAt,
	); err != nil {
		return nil, mapReadError("refresh_token", err)
	}

	return &token, nil
}

func (r *refreshTokenRepository) Update(ctx context.Context, token *ports.RefreshToken) error {
	if token == nil {
		return fmt.Errorf("refresh token nil: %w", ErrValidation)
	}

	if _, err := r.q.Exec(ctx, `
UPDATE refresh_tokens
SET successor_token_id = $2, encrypted_successor = $3, revoked_at = $4
WHERE id = $1`,
		token.ID,
		token.SuccessorTokenID,
		token.EncryptedSuccessor,
		token.RevokedAt,
	); err != nil {
		return mapWriteError("refresh_token_update", err)
	}

	return nil
}

func (r *refreshTokenRepository) GetFamily(ctx context.Context, familyID string) ([]*ports.RefreshToken, error) {
	return r.getFamily(ctx, familyID, "")
}

func (r *refreshTokenRepository) GetFamilyForUpdate(ctx context.Context, familyID string) ([]*ports.RefreshToken, error) {
	return r.getFamily(ctx, familyID, " FOR UPDATE")
}

func (r *refreshTokenRepository) getFamily(ctx context.Context, familyID, lockClause string) ([]*ports.RefreshToken, error) {
	rows, err := r.q.Query(ctx, `
SELECT id, session_id, token_hash, family_id, successor_token_id, encrypted_successor, created_at, expires_at, revoked_at
FROM refresh_tokens
WHERE family_id = $1`+lockClause, familyID)
	if err != nil {
		return nil, mapReadError("refresh_token_family", err)
	}
	defer rows.Close()

	var tokens []*ports.RefreshToken
	for rows.Next() {
		var token ports.RefreshToken
		if err := rows.Scan(
			&token.ID,
			&token.SessionID,
			&token.TokenHash,
			&token.FamilyID,
			&token.SuccessorTokenID,
			&token.EncryptedSuccessor,
			&token.CreatedAt,
			&token.ExpiresAt,
			&token.RevokedAt,
		); err != nil {
			return nil, mapReadError("refresh_token_family", err)
		}
		tokens = append(tokens, &token)
	}

	if err := rows.Err(); err != nil {
		return nil, mapReadError("refresh_token_family", err)
	}

	return tokens, nil
}
