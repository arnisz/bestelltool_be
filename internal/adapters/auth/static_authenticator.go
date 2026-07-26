// Package auth provides authentication adapters.
// TRANSITIONAL: StaticTokenAuthenticator is a temporary solution for development.
// Replace with a proper session/token-based authenticator before production use.
// See Tech Debt in status.md.
package auth

import (
	"context"
	"fmt"
	"strings"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

// StaticTokenAuthenticator maps static bearer tokens to principals.
// Configured via AUTH_STATIC_TOKENS (format: "token:user-id:role,...").
type StaticTokenAuthenticator struct {
	tokens map[string]*ports.Principal
}

// ParseStaticTokens parses the AUTH_STATIC_TOKENS string and returns a ready
// StaticTokenAuthenticator. Any parse or validation error is a startup failure.
//
// Format: "token:user-id:role,token2:user-id2:role2"
// Valid roles: technician, dispatcher, admin, system.
func ParseStaticTokens(raw string) (*StaticTokenAuthenticator, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("AUTH_STATIC_TOKENS is required")
	}

	tokens := make(map[string]*ports.Principal)
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, ":", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid token entry %q: expected token:user-id:role", entry)
		}
		token := strings.TrimSpace(parts[0])
		userID := strings.TrimSpace(parts[1])
		roleStr := strings.TrimSpace(parts[2])
		if token == "" || userID == "" || roleStr == "" {
			return nil, fmt.Errorf("invalid token entry %q: token, user-id and role must not be empty", entry)
		}
		role := domain.ActorRole(roleStr)
		switch role {
		case domain.ActorRoleTechnician, domain.ActorRoleDispatcher, domain.ActorRoleAdmin, domain.ActorRoleSystem:
		default:
			return nil, fmt.Errorf("invalid role %q in token entry %q", roleStr, entry)
		}
		tokens[token] = &ports.Principal{
			UserID:      domain.UserID(userID),
			Role:        role,
			Permissions: staticRolePermissions(role),
		}
	}

	if len(tokens) == 0 {
		return nil, fmt.Errorf("AUTH_STATIC_TOKENS contains no valid entries")
	}

	return &StaticTokenAuthenticator{tokens: tokens}, nil
}

// Authenticate implements ports.Authenticator.
func (a *StaticTokenAuthenticator) Authenticate(_ context.Context, token string) (*ports.Principal, error) {
	p, ok := a.tokens[token]
	if !ok {
		return nil, fmt.Errorf("token not recognized: %w", ports.ErrUnauthenticated)
	}
	return p, nil
}

func staticRolePermissions(role domain.ActorRole) map[string]struct{} {
	permissions := make(map[string]struct{})
	for _, code := range staticRolePermissionCodes(role) {
		permissions[code] = struct{}{}
	}
	return permissions
}

func staticRolePermissionCodes(role domain.ActorRole) []string {
	switch role {
	case domain.ActorRoleTechnician:
		return []string{
			domain.PermissionRequestCreate,
			domain.PermissionRequestRead,
			domain.PermissionAllocationReturnRequest,
			domain.PermissionEventStreamOwn,
		}
	case domain.ActorRoleDispatcher:
		return []string{
			domain.PermissionRequestRead,
			domain.PermissionAllocationReturnRequest,
			domain.PermissionResourceTransferDirect,
			domain.PermissionEventStreamAll,
		}
	default:
		return nil
	}
}
