package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

type permissionResolver struct {
	pool *pgxpool.Pool
}

func NewPermissionResolver(pool *pgxpool.Pool) ports.PermissionResolver {
	return &permissionResolver{pool: pool}
}

func (r *permissionResolver) PermissionsForRole(ctx context.Context, role domain.ActorRole) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT permission_code FROM role_permissions WHERE role_code = $1 ORDER BY permission_code`, string(role))
	if err != nil {
		return nil, fmt.Errorf("query permissions for role %s: %w", role, err)
	}
	defer rows.Close()

	var perms []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, fmt.Errorf("scan permission code: %w", err)
		}
		perms = append(perms, code)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate permission codes: %w", err)
	}
	return perms, nil
}
