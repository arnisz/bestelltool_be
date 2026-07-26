package ports

import (
	"context"
	"time"

	"bestelltool_be/internal/domain"
)

// AuthIdentity holds local-password credentials for a user (1:1 with users,
// systemdesign.md §7.2, migration 000008). Simplified to local-password-only
// for this increment - OIDC provider/provider_subject columns are deferred
// pending decision D-5 in systemdesign.md §12.
type AuthIdentity struct {
	UserID       domain.UserID
	PasswordHash string
}

// AuthIdentityRepository provides persistence access for local auth identities.
type AuthIdentityRepository interface {
	GetByUserID(ctx context.Context, userID domain.UserID) (*AuthIdentity, error)
	Save(ctx context.Context, identity *AuthIdentity) error
}

// Session represents an active login session bound to exactly one access
// token (systemdesign.md §7.3, migration 000008). TokenHash is the SHA-256
// hash of the access token's secret part - the secret itself is never stored.
type Session struct {
	ID         string
	UserID     domain.UserID
	ActiveRole domain.ActorRole
	TokenHash  []byte
	CreatedAt  time.Time
	ExpiresAt  time.Time
	RevokedAt  *time.Time
}

// SessionRepository provides persistence access for sessions.
type SessionRepository interface {
	Save(ctx context.Context, session *Session) error
	GetByID(ctx context.Context, id string) (*Session, error)
	Revoke(ctx context.Context, id string, at time.Time) error
}

// RefreshToken represents one link in a refresh-token rotation lineage
// (systemdesign.md §7.3, SEC-08, migration 000008). All tokens issued from
// the same login share FamilyID, so replay detection revokes an entire
// family in one statement instead of walking a predecessor chain.
// SuccessorTokenID being non-nil marks this token as already consumed - there
// is no separate ConsumedAt field.
type RefreshToken struct {
	ID               string
	SessionID        string
	TokenHash        []byte
	FamilyID         string
	SuccessorTokenID *string
	CreatedAt        time.Time
	ExpiresAt        time.Time
	RevokedAt        *time.Time
}

// RefreshTokenRepository provides persistence access for refresh tokens.
type RefreshTokenRepository interface {
	Save(ctx context.Context, token *RefreshToken) error
	GetByID(ctx context.Context, id string) (*RefreshToken, error)
	Update(ctx context.Context, token *RefreshToken) error
	// GetFamily returns every token sharing familyID, for the SEC-08 replay
	// case where the whole family must be revoked at once.
	GetFamily(ctx context.Context, familyID string) ([]*RefreshToken, error)
}

// PasswordHasher hashes and verifies passwords (Argon2id, SEC-02). The
// implementation lives entirely in an adapter - the domain and use-case
// layers only ever see this interface (agents.md §2).
type PasswordHasher interface {
	Hash(password string) (string, error)
	// Verify reports whether password matches hash, and whether hash was
	// produced with outdated parameters and should be re-hashed after a
	// successful login.
	Verify(password, hash string) (ok bool, needsRehash bool, err error)
}

// SecretGenerator produces cryptographically secure random secrets for
// tokens (SEC-04, SEC-07). Implementations must use crypto/rand, never
// math/rand.
type SecretGenerator interface {
	GenerateToken() (string, error)
}

// Clock provides the current time so session expiry, token lifetimes and
// lockout windows are testable without sleeping. Domain and use-case code
// must never call time.Now() directly (agents.md §2).
type Clock interface {
	Now() time.Time
}
