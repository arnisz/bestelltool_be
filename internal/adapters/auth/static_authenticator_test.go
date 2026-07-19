package auth

import (
	"context"
	"errors"
	"testing"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

func TestParseStaticTokens_Valid(t *testing.T) {
	a, err := ParseStaticTokens("tok1:user-1:dispatcher,tok2:user-2:technician")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p, err := a.Authenticate(context.Background(), "tok1")
	if err != nil {
		t.Fatalf("Authenticate error = %v", err)
	}
	if p.UserID != domain.UserID("user-1") {
		t.Fatalf("user id = %q, want user-1", p.UserID)
	}
	if p.Role != domain.ActorRoleDispatcher {
		t.Fatalf("role = %q, want dispatcher", p.Role)
	}

	p2, err := a.Authenticate(context.Background(), "tok2")
	if err != nil {
		t.Fatalf("Authenticate error = %v", err)
	}
	if p2.Role != domain.ActorRoleTechnician {
		t.Fatalf("role = %q, want technician", p2.Role)
	}
}

func TestParseStaticTokens_AllRoles(t *testing.T) {
	_, err := ParseStaticTokens("t1:u1:dispatcher,t2:u2:technician,t3:u3:system,t4:u4:admin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseStaticTokens_AdminRoleValid(t *testing.T) {
	a, err := ParseStaticTokens("tok-admin:admin-user:admin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p, err := a.Authenticate(context.Background(), "tok-admin")
	if err != nil {
		t.Fatalf("Authenticate error = %v", err)
	}
	if p.Role != domain.ActorRoleAdmin {
		t.Fatalf("role = %q, want admin", p.Role)
	}
}

func TestParseStaticTokens_Empty(t *testing.T) {
	_, err := ParseStaticTokens("")
	if err == nil {
		t.Fatal("expected error for empty string")
	}
}

func TestParseStaticTokens_WhitespaceOnly(t *testing.T) {
	_, err := ParseStaticTokens("   ")
	if err == nil {
		t.Fatal("expected error for whitespace-only string")
	}
}

func TestParseStaticTokens_InvalidFormat_MissingRole(t *testing.T) {
	_, err := ParseStaticTokens("tok1:user-1") // only two parts
	if err == nil {
		t.Fatal("expected error for entry with missing role")
	}
}

func TestParseStaticTokens_InvalidRole(t *testing.T) {
	_, err := ParseStaticTokens("tok1:user-1:superadmin")
	if err == nil {
		t.Fatal("expected error for unknown role")
	}
}

func TestParseStaticTokens_EmptyTokenField(t *testing.T) {
	_, err := ParseStaticTokens(":user-1:dispatcher")
	if err == nil {
		t.Fatal("expected error for empty token field")
	}
}

func TestParseStaticTokens_EmptyUserIDField(t *testing.T) {
	_, err := ParseStaticTokens("tok1::dispatcher")
	if err == nil {
		t.Fatal("expected error for empty user-id field")
	}
}

func TestParseStaticTokens_EmptyRoleField(t *testing.T) {
	_, err := ParseStaticTokens("tok1:user-1:")
	if err == nil {
		t.Fatal("expected error for empty role field")
	}
}

func TestParseStaticTokens_OnlyCommas(t *testing.T) {
	_, err := ParseStaticTokens(",,,")
	if err == nil {
		t.Fatal("expected error for only commas (no valid entries)")
	}
}

func TestAuthenticate_UnknownToken(t *testing.T) {
	a, err := ParseStaticTokens("tok1:user-1:dispatcher")
	if err != nil {
		t.Fatalf("setup error: %v", err)
	}

	_, err = a.Authenticate(context.Background(), "unknown")
	if err == nil {
		t.Fatal("expected error for unknown token")
	}
	if !errors.Is(err, ports.ErrUnauthenticated) {
		t.Fatalf("expected ErrUnauthenticated, got %v", err)
	}
}

func TestAuthenticate_CorrectPrincipalReturned(t *testing.T) {
	a, err := ParseStaticTokens("secret:alice:dispatcher")
	if err != nil {
		t.Fatalf("setup error: %v", err)
	}

	p, err := a.Authenticate(context.Background(), "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.UserID != "alice" {
		t.Fatalf("user id = %q, want alice", p.UserID)
	}
	if p.Role != domain.ActorRoleDispatcher {
		t.Fatalf("role = %q, want dispatcher", p.Role)
	}
}
