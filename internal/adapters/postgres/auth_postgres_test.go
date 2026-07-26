package postgres

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

// TestAuthIdentitySaveAndGet tests Save and GetByUserID for auth identities.
func TestAuthIdentitySaveAndGet(t *testing.T) {
	pool := testPool(t)

	now := time.Now()
	user, _ := domain.NewUser("user-auth-1", "alice", domain.ActorRoleTechnician, "Alice", nil, now)

	uow := NewUnitOfWork(pool)
	err := uow.WithinTransaction(t.Context(), func(ctx context.Context, tx ports.Transaction) error {
		// Create the user first (FK constraint)
		if err := tx.Users().Create(ctx, user); err != nil {
			return err
		}

		// Save an auth identity
		identity := &ports.AuthIdentity{
			UserID:       user.ID,
			PasswordHash: "argon2id$v=19$m=19456,t=2,p=1$abcdefghijklmnop$0123456789abcdef0123456789abcdef0123456789ab",
		}
		if err := tx.AuthIdentities().Save(ctx, identity); err != nil {
			return err
		}

		// Retrieve it
		retrieved, err := tx.AuthIdentities().GetByUserID(ctx, user.ID)
		if err != nil {
			return err
		}

		if retrieved == nil {
			t.Fatal("GetByUserID returned nil")
		}

		if retrieved.UserID != user.ID {
			t.Errorf("UserID mismatch: got %q, want %q", retrieved.UserID, user.ID)
		}

		if retrieved.PasswordHash != identity.PasswordHash {
			t.Errorf("PasswordHash mismatch: got %q, want %q", retrieved.PasswordHash, identity.PasswordHash)
		}

		return nil
	})

	if err != nil {
		t.Fatalf("WithinTransaction error = %v", err)
	}
}

// TestAuthIdentityUpdate tests updating an existing auth identity (rehashing).
func TestAuthIdentityUpdate(t *testing.T) {
	pool := testPool(t)

	now := time.Now()
	user, _ := domain.NewUser("user-auth-2", "bob", domain.ActorRoleDispatcher, "Bob", nil, now)
	oldHash := "old-argon2-hash"
	newHash := "new-argon2-hash-with-updated-params"

	uow := NewUnitOfWork(pool)
	err := uow.WithinTransaction(t.Context(), func(ctx context.Context, tx ports.Transaction) error {
		if err := tx.Users().Create(ctx, user); err != nil {
			return err
		}

		identity := &ports.AuthIdentity{UserID: user.ID, PasswordHash: oldHash}
		if err := tx.AuthIdentities().Save(ctx, identity); err != nil {
			return err
		}

		// Update the hash (rehashing scenario)
		identity.PasswordHash = newHash
		if err := tx.AuthIdentities().Save(ctx, identity); err != nil {
			return err
		}

		// Verify the update
		retrieved, err := tx.AuthIdentities().GetByUserID(ctx, user.ID)
		if err != nil {
			return err
		}

		if retrieved.PasswordHash != newHash {
			t.Errorf("Hash not updated: got %q, want %q", retrieved.PasswordHash, newHash)
		}

		return nil
	})

	if err != nil {
		t.Fatalf("WithinTransaction error = %v", err)
	}
}

// TestAuthIdentityNotFound tests getting a non-existent auth identity.
func TestAuthIdentityNotFound(t *testing.T) {
	pool := testPool(t)

	uow := NewUnitOfWork(pool)
	err := uow.WithinTransaction(t.Context(), func(ctx context.Context, tx ports.Transaction) error {
		retrieved, err := tx.AuthIdentities().GetByUserID(ctx, "nonexistent-user-id")
		if err != nil {
			// mapReadError wraps ErrNotFound with "entity: " prefix, so we use errors.Is
			if !errors.Is(err, ports.ErrNotFound) {
				t.Errorf("Expected ErrNotFound, got %v", err)
			}
			return nil
		}

		if retrieved != nil {
			t.Errorf("Expected nil, got %v", retrieved)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("WithinTransaction error = %v", err)
	}
}

// TestSessionSaveAndGet tests saving and retrieving a session.
func TestSessionSaveAndGet(t *testing.T) {
	pool := testPool(t)

	now := time.Now()
	user, _ := domain.NewUser("user-session-1", "charlie", domain.ActorRoleAdmin, "Charlie", nil, now)

	uow := NewUnitOfWork(pool)
	err := uow.WithinTransaction(t.Context(), func(ctx context.Context, tx ports.Transaction) error {
		if err := tx.Users().Create(ctx, user); err != nil {
			return err
		}

		// Create and save a session with a valid UUID
		tokenHash := sha256.Sum256([]byte("token-secret"))
		sessionID := uuid.New().String()
		session := &ports.Session{
			ID:         sessionID,
			UserID:     user.ID,
			ActiveRole: domain.ActorRoleAdmin,
			TokenHash:  tokenHash[:],
			CreatedAt:  now,
			ExpiresAt:  now.Add(15 * time.Minute),
			RevokedAt:  nil,
		}

		if err := tx.Sessions().Save(ctx, session); err != nil {
			return err
		}

		// Retrieve it
		retrieved, err := tx.Sessions().GetByID(ctx, session.ID)
		if err != nil {
			return err
		}

		if retrieved == nil {
			t.Fatal("GetByID returned nil")
		}

		if retrieved.ID != session.ID {
			t.Errorf("ID mismatch: got %q, want %q", retrieved.ID, session.ID)
		}

		if retrieved.UserID != session.UserID {
			t.Errorf("UserID mismatch: got %q, want %q", retrieved.UserID, session.UserID)
		}

		if retrieved.ActiveRole != session.ActiveRole {
			t.Errorf("ActiveRole mismatch: got %q, want %q", retrieved.ActiveRole, session.ActiveRole)
		}

		if len(retrieved.TokenHash) != len(tokenHash) {
			t.Errorf("TokenHash length mismatch: got %d, want %d", len(retrieved.TokenHash), len(tokenHash))
		}

		if retrieved.RevokedAt != nil {
			t.Errorf("RevokedAt should be nil, got %v", retrieved.RevokedAt)
		}

		return nil
	})

	if err != nil {
		t.Fatalf("WithinTransaction error = %v", err)
	}
}

func TestSessionGetByTokenHash(t *testing.T) {
	pool := testPool(t)
	now := time.Now()
	user, _ := domain.NewUser("user-session-hash", "session-hash", domain.ActorRoleTechnician, "Session Hash", nil, now)
	tokenHash := sha256.Sum256([]byte("access-token-secret"))
	session := &ports.Session{
		ID:         uuid.New().String(),
		UserID:     user.ID,
		ActiveRole: domain.ActorRoleTechnician,
		TokenHash:  tokenHash[:],
		CreatedAt:  now,
		ExpiresAt:  now.Add(15 * time.Minute),
	}

	uow := NewUnitOfWork(pool)
	err := uow.WithinTransaction(t.Context(), func(ctx context.Context, tx ports.Transaction) error {
		if err := tx.Users().Create(ctx, user); err != nil {
			return err
		}
		if err := tx.Sessions().Save(ctx, session); err != nil {
			return err
		}
		retrieved, err := tx.Sessions().GetByTokenHash(ctx, tokenHash[:])
		if err != nil {
			return err
		}
		if retrieved.ID != session.ID || retrieved.UserID != user.ID {
			t.Fatalf("GetByTokenHash() = %#v, want session %q for user %q", retrieved, session.ID, user.ID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithinTransaction error = %v", err)
	}
}

// TestSessionRevoke tests the Revoke operation.
func TestSessionRevoke(t *testing.T) {
	pool := testPool(t)

	now := time.Now()
	user, _ := domain.NewUser("user-session-2", "diana", domain.ActorRoleTechnician, "Diana", nil, now)

	uow := NewUnitOfWork(pool)
	err := uow.WithinTransaction(t.Context(), func(ctx context.Context, tx ports.Transaction) error {
		if err := tx.Users().Create(ctx, user); err != nil {
			return err
		}

		tokenHash := sha256.Sum256([]byte("token-secret"))
		sessionID := uuid.New().String()
		session := &ports.Session{
			ID:         sessionID,
			UserID:     user.ID,
			ActiveRole: domain.ActorRoleTechnician,
			TokenHash:  tokenHash[:],
			CreatedAt:  now,
			ExpiresAt:  now.Add(15 * time.Minute),
			RevokedAt:  nil,
		}

		if err := tx.Sessions().Save(ctx, session); err != nil {
			return err
		}

		// Revoke it
		revokeTime := now.Add(1 * time.Minute)
		if err := tx.Sessions().Revoke(ctx, session.ID, revokeTime); err != nil {
			return err
		}

		// Verify revocation
		retrieved, err := tx.Sessions().GetByID(ctx, session.ID)
		if err != nil {
			return err
		}

		if retrieved.RevokedAt == nil {
			t.Errorf("RevokedAt should not be nil after Revoke")
		} else {
			// Compare with millisecond precision (databases truncate nanoseconds)
			if retrieved.RevokedAt.Truncate(time.Millisecond) != revokeTime.Truncate(time.Millisecond) {
				t.Errorf("RevokedAt mismatch: got %v, want %v",
					retrieved.RevokedAt.Truncate(time.Millisecond), revokeTime.Truncate(time.Millisecond))
			}
		}

		return nil
	})

	if err != nil {
		t.Fatalf("WithinTransaction error = %v", err)
	}
}

func TestSessionRevokeAllForUserExcept(t *testing.T) {
	pool := testPool(t)
	now := time.Now()
	user, err := domain.NewUser("user-session-revoke-others", "revoke-others", domain.ActorRoleTechnician, "Revoke Others", nil, now)
	if err != nil {
		t.Fatalf("NewUser() error = %v", err)
	}
	currentHash := sha256.Sum256([]byte("current"))
	otherHash := sha256.Sum256([]byte("other"))
	current := &ports.Session{
		ID:         uuid.New().String(),
		UserID:     user.ID,
		ActiveRole: domain.ActorRoleTechnician,
		TokenHash:  currentHash[:],
		CreatedAt:  now,
		ExpiresAt:  now.Add(time.Hour),
	}
	other := &ports.Session{
		ID:         uuid.New().String(),
		UserID:     user.ID,
		ActiveRole: domain.ActorRoleTechnician,
		TokenHash:  otherHash[:],
		CreatedAt:  now,
		ExpiresAt:  now.Add(time.Hour),
	}

	uow := NewUnitOfWork(pool)
	err = uow.WithinTransaction(t.Context(), func(ctx context.Context, tx ports.Transaction) error {
		if err := tx.Users().Create(ctx, user); err != nil {
			return err
		}
		if err := tx.Sessions().Save(ctx, current); err != nil {
			return err
		}
		if err := tx.Sessions().Save(ctx, other); err != nil {
			return err
		}
		return tx.Sessions().RevokeAllForUserExcept(ctx, user.ID, current.ID, now)
	})
	if err != nil {
		t.Fatalf("WithinTransaction() error = %v", err)
	}

	err = uow.WithinTransaction(t.Context(), func(ctx context.Context, tx ports.Transaction) error {
		currentSession, err := tx.Sessions().GetByID(ctx, current.ID)
		if err != nil {
			return err
		}
		otherSession, err := tx.Sessions().GetByID(ctx, other.ID)
		if err != nil {
			return err
		}
		if currentSession.RevokedAt != nil {
			t.Fatalf("current session was revoked")
		}
		if otherSession.RevokedAt == nil {
			t.Fatalf("other session was not revoked")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify sessions: %v", err)
	}
}

// TestRefreshTokenSaveAndGet tests saving and retrieving a refresh token.
func TestRefreshTokenSaveAndGet(t *testing.T) {
	pool := testPool(t)

	now := time.Now()
	user, _ := domain.NewUser("user-refresh-1", "eve", domain.ActorRoleDispatcher, "Eve", nil, now)

	uow := NewUnitOfWork(pool)
	err := uow.WithinTransaction(t.Context(), func(ctx context.Context, tx ports.Transaction) error {
		if err := tx.Users().Create(ctx, user); err != nil {
			return err
		}

		// Create and save a session first (FK requirement)
		tokenHash := sha256.Sum256([]byte("access-token-secret"))
		sessionID := uuid.New().String()
		session := &ports.Session{
			ID:         sessionID,
			UserID:     user.ID,
			ActiveRole: domain.ActorRoleDispatcher,
			TokenHash:  tokenHash[:],
			CreatedAt:  now,
			ExpiresAt:  now.Add(15 * time.Minute),
			RevokedAt:  nil,
		}

		if err := tx.Sessions().Save(ctx, session); err != nil {
			return err
		}

		// Create and save a refresh token
		refreshTokenHash := sha256.Sum256([]byte("refresh-token-secret"))
		refreshTokenID := uuid.New().String()
		familyID := uuid.New().String()
		refreshToken := &ports.RefreshToken{
			ID:               refreshTokenID,
			SessionID:        session.ID,
			TokenHash:        refreshTokenHash[:],
			FamilyID:         familyID,
			SuccessorTokenID: nil,
			CreatedAt:        now,
			ExpiresAt:        now.Add(7 * 24 * time.Hour),
			RevokedAt:        nil,
		}

		if err := tx.RefreshTokens().Save(ctx, refreshToken); err != nil {
			return err
		}

		// Retrieve it
		retrieved, err := tx.RefreshTokens().GetByID(ctx, refreshToken.ID)
		if err != nil {
			return err
		}

		if retrieved == nil {
			t.Fatal("GetByID returned nil")
		}

		if retrieved.ID != refreshToken.ID {
			t.Errorf("ID mismatch: got %q, want %q", retrieved.ID, refreshToken.ID)
		}

		if retrieved.SessionID != refreshToken.SessionID {
			t.Errorf("SessionID mismatch: got %q, want %q", retrieved.SessionID, refreshToken.SessionID)
		}

		if retrieved.FamilyID != refreshToken.FamilyID {
			t.Errorf("FamilyID mismatch: got %q, want %q", retrieved.FamilyID, refreshToken.FamilyID)
		}

		if retrieved.SuccessorTokenID != nil {
			t.Errorf("SuccessorTokenID should be nil, got %v", retrieved.SuccessorTokenID)
		}

		return nil
	})

	if err != nil {
		t.Fatalf("WithinTransaction error = %v", err)
	}
}

// TestRefreshTokenUpdate tests updating a refresh token (marking as consumed).
func TestRefreshTokenUpdate(t *testing.T) {
	pool := testPool(t)

	now := time.Now()
	user, _ := domain.NewUser("user-refresh-2", "frank", domain.ActorRoleTechnician, "Frank", nil, now)

	uow := NewUnitOfWork(pool)
	err := uow.WithinTransaction(t.Context(), func(ctx context.Context, tx ports.Transaction) error {
		if err := tx.Users().Create(ctx, user); err != nil {
			return err
		}

		// Create session
		tokenHash := sha256.Sum256([]byte("access-token-secret"))
		sessionID := uuid.New().String()
		session := &ports.Session{
			ID:         sessionID,
			UserID:     user.ID,
			ActiveRole: domain.ActorRoleTechnician,
			TokenHash:  tokenHash[:],
			CreatedAt:  now,
			ExpiresAt:  now.Add(15 * time.Minute),
			RevokedAt:  nil,
		}

		if err := tx.Sessions().Save(ctx, session); err != nil {
			return err
		}

		// Create and save refresh token
		refreshTokenHash := sha256.Sum256([]byte("refresh-token-secret"))
		refreshTokenID := uuid.New().String()
		familyID := uuid.New().String()
		refreshToken := &ports.RefreshToken{
			ID:               refreshTokenID,
			SessionID:        session.ID,
			TokenHash:        refreshTokenHash[:],
			FamilyID:         familyID,
			SuccessorTokenID: nil,
			CreatedAt:        now,
			ExpiresAt:        now.Add(7 * 24 * time.Hour),
			RevokedAt:        nil,
		}

		if err := tx.RefreshTokens().Save(ctx, refreshToken); err != nil {
			return err
		}

		// Create the successor token first (FK constraint)
		successorHash := sha256.Sum256([]byte("successor-token-secret"))
		successorID := uuid.New().String()
		successor := &ports.RefreshToken{
			ID:               successorID,
			SessionID:        session.ID,
			TokenHash:        successorHash[:],
			FamilyID:         familyID,
			SuccessorTokenID: nil,
			CreatedAt:        now.Add(1 * time.Minute),
			ExpiresAt:        now.Add(7*24*time.Hour + 1*time.Minute),
			RevokedAt:        nil,
		}

		if err := tx.RefreshTokens().Save(ctx, successor); err != nil {
			return err
		}

		// Now update original to point to successor
		refreshToken.SuccessorTokenID = &successorID
		refreshToken.EncryptedSuccessor = []byte("AES-GCM-ciphertext")
		if err := tx.RefreshTokens().Update(ctx, refreshToken); err != nil {
			return err
		}

		// Verify update
		retrieved, err := tx.RefreshTokens().GetByID(ctx, refreshToken.ID)
		if err != nil {
			return err
		}

		if retrieved.SuccessorTokenID == nil || *retrieved.SuccessorTokenID != successorID {
			t.Errorf("SuccessorTokenID not updated: got %v, want %q", retrieved.SuccessorTokenID, successorID)
		}
		if string(retrieved.EncryptedSuccessor) != string(refreshToken.EncryptedSuccessor) {
			t.Errorf("EncryptedSuccessor = %q, want %q", retrieved.EncryptedSuccessor, refreshToken.EncryptedSuccessor)
		}

		return nil
	})

	if err != nil {
		t.Fatalf("WithinTransaction error = %v", err)
	}
}

// TestRefreshTokenGetFamily tests retrieving all tokens from a family (SEC-08).
func TestRefreshTokenGetFamily(t *testing.T) {
	pool := testPool(t)

	now := time.Now()
	user, _ := domain.NewUser("user-refresh-3", "grace", domain.ActorRoleAdmin, "Grace", nil, now)
	familyID := uuid.New().String()

	uow := NewUnitOfWork(pool)
	err := uow.WithinTransaction(t.Context(), func(ctx context.Context, tx ports.Transaction) error {
		if err := tx.Users().Create(ctx, user); err != nil {
			return err
		}

		// Create session
		tokenHash := sha256.Sum256([]byte("access-token-secret"))
		sessionID := uuid.New().String()
		session := &ports.Session{
			ID:         sessionID,
			UserID:     user.ID,
			ActiveRole: domain.ActorRoleAdmin,
			TokenHash:  tokenHash[:],
			CreatedAt:  now,
			ExpiresAt:  now.Add(15 * time.Minute),
			RevokedAt:  nil,
		}

		if err := tx.Sessions().Save(ctx, session); err != nil {
			return err
		}

		// Create three tokens from the same family (simulating refresh rotation)
		for i := 0; i < 3; i++ {
			tokenHash := sha256.Sum256([]byte("refresh-token-" + string(rune('0'+i))))
			refreshTokenID := uuid.New().String()
			refreshToken := &ports.RefreshToken{
				ID:               refreshTokenID,
				SessionID:        session.ID,
				TokenHash:        tokenHash[:],
				FamilyID:         familyID,
				SuccessorTokenID: nil,
				CreatedAt:        now.Add(time.Duration(i) * time.Minute),
				ExpiresAt:        now.Add(7 * 24 * time.Hour),
				RevokedAt:        nil,
			}

			if err := tx.RefreshTokens().Save(ctx, refreshToken); err != nil {
				return err
			}
		}

		// Retrieve all tokens from the family
		family, err := tx.RefreshTokens().GetFamily(ctx, familyID)
		if err != nil {
			return err
		}

		if len(family) != 3 {
			t.Errorf("Expected 3 tokens in family, got %d", len(family))
		}

		// Verify all tokens have correct family
		for _, token := range family {
			if token.FamilyID != familyID {
				t.Errorf("Token %s has wrong family: got %q, want %q", token.ID, token.FamilyID, familyID)
			}
		}

		return nil
	})

	if err != nil {
		t.Fatalf("WithinTransaction error = %v", err)
	}
}

// TestRefreshTokenRevokeFamilySEC08 tests the SEC-08 requirement: revoking an entire
// token family when replay is detected.
func TestRefreshTokenRevokeFamilySEC08(t *testing.T) {
	pool := testPool(t)

	now := time.Now()
	user, _ := domain.NewUser("user-refresh-4", "henry", domain.ActorRoleDispatcher, "Henry", nil, now)
	familyID := uuid.New().String()

	uow := NewUnitOfWork(pool)
	err := uow.WithinTransaction(t.Context(), func(ctx context.Context, tx ports.Transaction) error {
		if err := tx.Users().Create(ctx, user); err != nil {
			return err
		}

		// Create session
		tokenHash := sha256.Sum256([]byte("access-token-secret"))
		sessionID := uuid.New().String()
		session := &ports.Session{
			ID:         sessionID,
			UserID:     user.ID,
			ActiveRole: domain.ActorRoleDispatcher,
			TokenHash:  tokenHash[:],
			CreatedAt:  now,
			ExpiresAt:  now.Add(15 * time.Minute),
			RevokedAt:  nil,
		}

		if err := tx.Sessions().Save(ctx, session); err != nil {
			return err
		}

		// Create three tokens from the same family
		var tokenIDs []string
		for i := 0; i < 3; i++ {
			tokenHash := sha256.Sum256([]byte("token-" + string(rune('0'+i))))
			refreshTokenID := uuid.New().String()
			tokenIDs = append(tokenIDs, refreshTokenID)
			token := &ports.RefreshToken{
				ID:               refreshTokenID,
				SessionID:        session.ID,
				TokenHash:        tokenHash[:],
				FamilyID:         familyID,
				SuccessorTokenID: nil,
				CreatedAt:        now,
				ExpiresAt:        now.Add(7 * 24 * time.Hour),
				RevokedAt:        nil,
			}

			if err := tx.RefreshTokens().Save(ctx, token); err != nil {
				return err
			}
		}

		// Now simulate SEC-08: get family and update all to revoked_at
		family, err := tx.RefreshTokens().GetFamily(ctx, familyID)
		if err != nil {
			return err
		}

		revokeTime := now.Add(5 * time.Minute)
		for _, token := range family {
			token.RevokedAt = &revokeTime
			if err := tx.RefreshTokens().Update(ctx, token); err != nil {
				return err
			}
		}

		// Verify all are revoked
		family, err = tx.RefreshTokens().GetFamily(ctx, familyID)
		if err != nil {
			return err
		}

		for _, token := range family {
			if token.RevokedAt == nil {
				t.Errorf("Token %s should be revoked", token.ID)
			} else {
				// Compare times with millisecond precision (databases truncate nanoseconds)
				if token.RevokedAt.Truncate(time.Millisecond) != revokeTime.Truncate(time.Millisecond) {
					t.Errorf("Token %s has wrong revoke time: got %v, want %v",
						token.ID, token.RevokedAt.Truncate(time.Millisecond), revokeTime.Truncate(time.Millisecond))
				}
			}
		}

		return nil
	})

	if err != nil {
		t.Fatalf("WithinTransaction error = %v", err)
	}
}

// TestRefreshTokenGetFamilyEmptyResult tests GetFamily when no tokens match.
func TestRefreshTokenGetFamilyEmptyResult(t *testing.T) {
	pool := testPool(t)

	uow := NewUnitOfWork(pool)
	err := uow.WithinTransaction(t.Context(), func(ctx context.Context, tx ports.Transaction) error {
		// Use a non-existent but valid UUID
		nonexistentFamily := uuid.New().String()
		family, err := tx.RefreshTokens().GetFamily(ctx, nonexistentFamily)
		if err != nil {
			return err
		}

		if len(family) != 0 {
			t.Errorf("Expected empty family, got %d tokens", len(family))
		}

		return nil
	})

	if err != nil {
		t.Fatalf("WithinTransaction error = %v", err)
	}
}
