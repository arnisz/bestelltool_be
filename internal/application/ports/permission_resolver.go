package ports

import (
	"context"

	"bestelltool_be/internal/domain"
)

// PermissionResolver resolves the permission codes granted to a role.
// Permissions are always resolved from current database state (SEC-14),
// never trusted from a client-supplied claim or cached indefinitely on its
// own - the caller (SessionAuthenticator) is responsible for caching
// alongside the rest of the Principal.
type PermissionResolver interface {
	PermissionsForRole(ctx context.Context, role domain.ActorRole) ([]string, error)
}
