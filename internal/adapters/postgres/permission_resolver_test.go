package postgres

import (
	"slices"
	"testing"

	"bestelltool_be/internal/domain"
)

func TestPermissionResolverPermissionsForRole_SEC14(t *testing.T) {
	resolver := NewPermissionResolver(testPool(t))

	permissions, err := resolver.PermissionsForRole(t.Context(), domain.ActorRoleTechnician)
	if err != nil {
		t.Fatalf("PermissionsForRole() error = %v", err)
	}
	if !slices.Contains(permissions, "request.create") {
		t.Fatalf("permissions = %v, want request.create", permissions)
	}
	if !slices.Contains(permissions, "event.stream.own") {
		t.Fatalf("permissions = %v, want event.stream.own", permissions)
	}
	if slices.Contains(permissions, "resource.transfer_direct") {
		t.Fatalf("permissions = %v, must not grant resource.transfer_direct", permissions)
	}
}

func TestPermissionResolverReturnsNoPermissionsForUnknownRole_SEC14(t *testing.T) {
	resolver := NewPermissionResolver(testPool(t))

	permissions, err := resolver.PermissionsForRole(t.Context(), domain.ActorRole("unknown"))
	if err != nil {
		t.Fatalf("PermissionsForRole() error = %v", err)
	}
	if len(permissions) != 0 {
		t.Fatalf("permissions = %v, want no permissions", permissions)
	}
}
