package postgres

import (
	"context"
	"errors"
	"fmt"

	"bestelltool_be/internal/domain"

	"github.com/jackc/pgx/v5"
)

type userRoleRepository struct {
	q querier
}

func (r *userRoleRepository) RolesForUser(ctx context.Context, userID domain.UserID) ([]domain.ActorRole, error) {
	rows, err := r.q.Query(ctx, `SELECT role_code FROM user_roles WHERE user_id = $1`, string(userID))
	if err != nil {
		return nil, fmt.Errorf("query user roles: %w", err)
	}
	defer rows.Close()

	var roles []domain.ActorRole
	for rows.Next() {
		var role domain.ActorRole
		if err := rows.Scan(&role); err != nil {
			return nil, fmt.Errorf("scan user role: %w", err)
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user roles: %w", err)
	}
	return roles, nil
}

func (r *userRoleRepository) HasRoleForUpdate(ctx context.Context, userID domain.UserID, role domain.ActorRole) (bool, error) {
	row := r.q.QueryRow(ctx, `
SELECT 1
FROM user_roles
WHERE user_id = $1 AND role_code = $2
FOR UPDATE`, string(userID), string(role))

	var found int
	if err := row.Scan(&found); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("query user role for update: %w", err)
	}

	return true, nil
}
