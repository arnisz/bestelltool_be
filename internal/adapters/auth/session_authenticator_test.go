package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

type authenticatorClock struct{ now time.Time }

func (c *authenticatorClock) Now() time.Time { return c.now }

type authenticatorSessions struct {
	session *ports.Session
	calls   int
}

type authenticatorPermissionResolver struct {
	permissions []string
	err         error
	calls       int
}

func (r *authenticatorPermissionResolver) PermissionsForRole(_ context.Context, _ domain.ActorRole) ([]string, error) {
	r.calls++
	return r.permissions, r.err
}

func (r *authenticatorSessions) Save(context.Context, *ports.Session) error   { return nil }
func (r *authenticatorSessions) Update(context.Context, *ports.Session) error { return nil }
func (r *authenticatorSessions) GetByID(context.Context, string) (*ports.Session, error) {
	return nil, ports.ErrNotFound
}
func (r *authenticatorSessions) GetForUpdate(context.Context, string) (*ports.Session, error) {
	return nil, ports.ErrNotFound
}
func (r *authenticatorSessions) Revoke(context.Context, string, time.Time) error { return nil }
func (r *authenticatorSessions) RevokeAllForUserExcept(context.Context, domain.UserID, string, time.Time) error {
	return nil
}
func (r *authenticatorSessions) GetByTokenHash(_ context.Context, tokenHash []byte) (*ports.Session, error) {
	r.calls++
	if r.session == nil || string(r.session.TokenHash) != string(tokenHash) {
		return nil, ports.ErrNotFound
	}
	return r.session, nil
}

type authenticatorTx struct{ sessions *authenticatorSessions }

func (tx authenticatorTx) Users() ports.UserRepository                    { return nil }
func (tx authenticatorTx) UserRoles() ports.UserRoleRepository            { return nil }
func (tx authenticatorTx) ResourceClasses() ports.ResourceClassRepository { return nil }
func (tx authenticatorTx) Requests() ports.RequestRepository              { return nil }
func (tx authenticatorTx) Resources() ports.ResourceRepository            { return nil }
func (tx authenticatorTx) Allocations() ports.AllocationRepository        { return nil }
func (tx authenticatorTx) Audits() ports.AuditWriter                      { return nil }
func (tx authenticatorTx) AuditEvents() ports.AuditRepository             { return nil }
func (tx authenticatorTx) Idempotency() ports.IdempotencyStore            { return nil }
func (tx authenticatorTx) AuthIdentities() ports.AuthIdentityRepository   { return nil }
func (tx authenticatorTx) Sessions() ports.SessionRepository              { return tx.sessions }
func (tx authenticatorTx) RefreshTokens() ports.RefreshTokenRepository    { return nil }

type authenticatorUoW struct{ tx authenticatorTx }

func (u authenticatorUoW) WithinTransaction(ctx context.Context, fn func(context.Context, ports.Transaction) error) error {
	return fn(ctx, u.tx)
}

func TestSessionAuthenticatorCachesUntilHardTTL_SEC11(t *testing.T) {
	now := time.Date(2026, time.July, 26, 14, 0, 0, 0, time.UTC)
	clock := &authenticatorClock{now: now}
	secret := "secret"
	hash := sha256.Sum256([]byte(secret))
	sessions := &authenticatorSessions{session: &ports.Session{ID: "session-1", UserID: "user-1", ActiveRole: domain.ActorRoleTechnician, TokenHash: hash[:], ExpiresAt: now.Add(time.Hour)}}
	resolver := &authenticatorPermissionResolver{permissions: []string{"request.create"}}
	a := NewSessionAuthenticatorWithCacheTTL(authenticatorUoW{tx: authenticatorTx{sessions: sessions}}, resolver, clock, time.Minute)

	for range 2 {
		principal, err := a.Authenticate(t.Context(), "rp_at_token-id."+secret)
		if err != nil {
			t.Fatalf("Authenticate() error = %v", err)
		}
		if principal.SessionID != "session-1" {
			t.Fatalf("principal session ID = %q, want session-1", principal.SessionID)
		}
		if !principal.HasPermission("request.create") {
			t.Fatal("principal missing resolved request.create permission")
		}
	}
	if sessions.calls != 1 {
		t.Fatalf("database calls = %d, want 1 while cache is valid", sessions.calls)
	}
	if resolver.calls != 1 {
		t.Fatalf("permission resolver calls = %d, want 1 while cache is valid", resolver.calls)
	}

	clock.now = now.Add(time.Minute)
	if _, err := a.Authenticate(t.Context(), "rp_at_token-id."+secret); err != nil {
		t.Fatalf("Authenticate() after TTL error = %v", err)
	}
	if sessions.calls != 2 {
		t.Fatalf("database calls = %d, want 2 after cache expiry", sessions.calls)
	}
	if resolver.calls != 2 {
		t.Fatalf("permission resolver calls = %d, want 2 after cache expiry", resolver.calls)
	}
}

func TestSessionAuthenticatorReturnsPermissionMapIsolatedFromCache_SEC11(t *testing.T) {
	now := time.Date(2026, time.July, 26, 14, 0, 0, 0, time.UTC)
	secret := "secret"
	hash := sha256.Sum256([]byte(secret))
	sessions := &authenticatorSessions{session: &ports.Session{ID: "session-1", UserID: "user-1", ActiveRole: domain.ActorRoleTechnician, TokenHash: hash[:], ExpiresAt: now.Add(time.Hour)}}
	resolver := &authenticatorPermissionResolver{permissions: []string{"request.create"}}
	a := NewSessionAuthenticatorWithCacheTTL(authenticatorUoW{tx: authenticatorTx{sessions: sessions}}, resolver, &authenticatorClock{now: now}, time.Minute)

	principal, err := a.Authenticate(t.Context(), "rp_at_token-id."+secret)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	principal.Permissions["request.create"] = struct{}{}
	principal.Permissions["resource.transfer"] = struct{}{}

	cached, err := a.Authenticate(t.Context(), "rp_at_token-id."+secret)
	if err != nil {
		t.Fatalf("Authenticate() from cache error = %v", err)
	}
	if cached.HasPermission("resource.transfer") {
		t.Fatal("cached principal contained permission mutated by a prior caller")
	}
}

func TestSessionAuthenticatorRejectsPermissionResolutionFailure_SEC14(t *testing.T) {
	now := time.Date(2026, time.July, 26, 14, 0, 0, 0, time.UTC)
	secret := "secret"
	hash := sha256.Sum256([]byte(secret))
	want := errors.New("database unavailable")
	sessions := &authenticatorSessions{session: &ports.Session{ID: "session-1", UserID: "user-1", ActiveRole: domain.ActorRoleTechnician, TokenHash: hash[:], ExpiresAt: now.Add(time.Hour)}}
	resolver := &authenticatorPermissionResolver{err: want}
	a := NewSessionAuthenticatorWithCacheTTL(authenticatorUoW{tx: authenticatorTx{sessions: sessions}}, resolver, &authenticatorClock{now: now}, time.Minute)

	_, err := a.Authenticate(t.Context(), "rp_at_token-id."+secret)
	if !errors.Is(err, want) {
		t.Fatalf("Authenticate() error = %v, want wrapped %v", err, want)
	}
}

func TestSessionAuthenticatorRejectsRevokedSession_SEC11(t *testing.T) {
	now := time.Date(2026, time.July, 26, 14, 0, 0, 0, time.UTC)
	secret := "secret"
	hash := sha256.Sum256([]byte(secret))
	revokedAt := now.Add(-time.Minute)
	sessions := &authenticatorSessions{session: &ports.Session{ID: "session-1", UserID: "user-1", ActiveRole: domain.ActorRoleTechnician, TokenHash: hash[:], ExpiresAt: now.Add(time.Hour), RevokedAt: &revokedAt}}
	a := NewSessionAuthenticatorWithCacheTTL(authenticatorUoW{tx: authenticatorTx{sessions: sessions}}, &authenticatorPermissionResolver{}, &authenticatorClock{now: now}, time.Minute)

	_, err := a.Authenticate(t.Context(), "rp_at_token-id."+secret)
	if !errors.Is(err, ports.ErrUnauthenticated) {
		t.Fatalf("Authenticate() error = %v, want ErrUnauthenticated", err)
	}
}
