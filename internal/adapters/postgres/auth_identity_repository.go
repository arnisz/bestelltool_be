package postgres

import (
	"context"
	"fmt"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

type authIdentityRepository struct {
	q querier
}

func (r *authIdentityRepository) GetByUserID(ctx context.Context, userID domain.UserID) (*ports.AuthIdentity, error) {
	row := r.q.QueryRow(ctx, `
SELECT user_id, password_hash
FROM auth_identities
WHERE user_id = $1`, string(userID))

	var identity ports.AuthIdentity
	if err := row.Scan(&identity.UserID, &identity.PasswordHash); err != nil {
		return nil, mapReadError("auth_identity", err)
	}

	return &identity, nil
}

func (r *authIdentityRepository) Save(ctx context.Context, identity *ports.AuthIdentity) error {
	if identity == nil {
		return fmt.Errorf("auth identity nil: %w", ErrValidation)
	}

	if _, err := r.q.Exec(ctx, `
INSERT INTO auth_identities(user_id, password_hash)
VALUES ($1, $2)
ON CONFLICT(user_id) DO UPDATE SET password_hash = $2`,
		string(identity.UserID),
		identity.PasswordHash,
	); err != nil {
		return mapWriteError("auth_identity", err)
	}

	return nil
}
