package usecases

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

const (
	// tokenPrefix differentiates between access and refresh tokens in logs/scanners
	accessTokenPrefix  = "rp_at_"
	refreshTokenPrefix = "rp_rt_"

	// Session and token lifetimes
	defaultAccessTokenTTL  = 15 * time.Minute
	defaultRefreshTokenTTL = 7 * 24 * time.Hour
)

// LoginInput represents login credentials.
type LoginInput struct {
	Username string
	Password string
}

// LoginOutput contains the issued tokens (both in plaintext).
type LoginOutput struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64 // seconds until access token expiry
}

// LoginUseCase authenticates a user and creates a session + refresh token.
type LoginUseCase struct {
	uow             ports.UnitOfWork
	passwordHasher  ports.PasswordHasher
	secretGenerator ports.SecretGenerator
	clock           ports.Clock
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

// NewLoginUseCase creates a LoginUseCase with default token lifetimes.
func NewLoginUseCase(
	uow ports.UnitOfWork,
	passwordHasher ports.PasswordHasher,
	secretGenerator ports.SecretGenerator,
	clock ports.Clock,
) *LoginUseCase {
	return &LoginUseCase{
		uow:             uow,
		passwordHasher:  passwordHasher,
		secretGenerator: secretGenerator,
		clock:           clock,
		accessTokenTTL:  defaultAccessTokenTTL,
		refreshTokenTTL: defaultRefreshTokenTTL,
	}
}

// NewLoginUseCaseWithTTLs creates a LoginUseCase with custom token lifetimes
// (primarily for testing).
func NewLoginUseCaseWithTTLs(
	uow ports.UnitOfWork,
	passwordHasher ports.PasswordHasher,
	secretGenerator ports.SecretGenerator,
	clock ports.Clock,
	accessTokenTTL time.Duration,
	refreshTokenTTL time.Duration,
) *LoginUseCase {
	return &LoginUseCase{
		uow:             uow,
		passwordHasher:  passwordHasher,
		secretGenerator: secretGenerator,
		clock:           clock,
		accessTokenTTL:  accessTokenTTL,
		refreshTokenTTL: refreshTokenTTL,
	}
}

// Execute authenticates a user and creates a session + refresh token atomically.
// SEC-03: Response time is constant regardless of whether the user exists or the
// password matches — a dummy Argon2 verification runs even for non-existent users.
func (uc *LoginUseCase) Execute(ctx context.Context, in LoginInput) (*LoginOutput, error) {
	if in.Username == "" {
		return nil, fmt.Errorf("username: %w", domain.ErrRequiredField)
	}
	if in.Password == "" {
		return nil, fmt.Errorf("password: %w", domain.ErrRequiredField)
	}

	var output *LoginOutput
	err := uc.uow.WithinTransaction(ctx, func(ctx context.Context, tx ports.Transaction) error {
		dummyHash, err := uc.passwordHasher.Hash("dummy-password-never-used")
		if err != nil {
			return fmt.Errorf("dummy hash: %w", err)
		}

		runDummyVerify := func() error {
			_, _, err := uc.passwordHasher.Verify(in.Password, dummyHash)
			if err != nil {
				return fmt.Errorf("dummy verify: %w", err)
			}
			return nil
		}

		// Load user by username.
		user, err := tx.Users().GetByUsername(ctx, in.Username)
		if err != nil && !errors.Is(err, ports.ErrNotFound) {
			return fmt.Errorf("get user by username: %w", err)
		}

		// Load auth identity. If user exists but identity doesn't, also return generic error.
		var authIdentity *ports.AuthIdentity
		if user != nil {
			authIdentity, err = tx.AuthIdentities().GetByUserID(ctx, user.ID)
			if err != nil && !errors.Is(err, ports.ErrNotFound) {
				return fmt.Errorf("get auth identity: %w", err)
			}

			// SEC-03: If identity doesn't exist, run dummy verification anyway.
			if authIdentity == nil {
				if err := runDummyVerify(); err != nil {
					return err
				}
				return ports.ErrCredentialsInvalid
			}
		}

		// If user doesn't exist, return generic error now.
		if user == nil {
			if err := runDummyVerify(); err != nil {
				return err
			}
			return ports.ErrCredentialsInvalid
		}

		// Check if user is active.
		if !user.IsActive {
			// Still run the password verification for constant time (SEC-03).
			_, _, _ = uc.passwordHasher.Verify(in.Password, authIdentity.PasswordHash)
			return ports.ErrCredentialsInvalid
		}

		// Verify password.
		ok, needsRehash, err := uc.passwordHasher.Verify(in.Password, authIdentity.PasswordHash)
		if err != nil {
			return fmt.Errorf("verify password: %w", err)
		}
		if !ok {
			return ports.ErrCredentialsInvalid
		}

		// Optional: rehash password if algorithm parameters have changed.
		if needsRehash {
			newHash, err := uc.passwordHasher.Hash(in.Password)
			if err != nil {
				return fmt.Errorf("rehash password: %w", err)
			}
			authIdentity.PasswordHash = newHash
			if err := tx.AuthIdentities().Save(ctx, authIdentity); err != nil {
				return fmt.Errorf("save updated auth identity: %w", err)
			}
		}

		// Generate access and refresh tokens (both plaintext).
		accessSecret, err := uc.secretGenerator.GenerateToken()
		if err != nil {
			return fmt.Errorf("generate access token secret: %w", err)
		}
		refreshSecret, err := uc.secretGenerator.GenerateToken()
		if err != nil {
			return fmt.Errorf("generate refresh token secret: %w", err)
		}

		accessTokenID, err := generateTokenID()
		if err != nil {
			return fmt.Errorf("generate access token id: %w", err)
		}
		refreshTokenID, err := generateTokenID()
		if err != nil {
			return fmt.Errorf("generate refresh token id: %w", err)
		}

		accessToken := accessTokenPrefix + accessTokenID + "." + accessSecret
		refreshToken := refreshTokenPrefix + refreshTokenID + "." + refreshSecret

		// Hash both tokens for storage.
		accessTokenHash := sha256.Sum256([]byte(accessSecret))
		refreshTokenHash := sha256.Sum256([]byte(refreshSecret))

		now := uc.clock.Now()
		sessionID, err := generateTokenID() // Use same ID generation as token IDs
		if err != nil {
			return fmt.Errorf("generate session id: %w", err)
		}
		familyID, err := generateTokenID() // All tokens from this login share a family
		if err != nil {
			return fmt.Errorf("generate refresh family id: %w", err)
		}
		refreshTokenEntityID, err := generateTokenID()
		if err != nil {
			return fmt.Errorf("generate refresh token entity id: %w", err)
		}

		// Create and save Session.
		session := &ports.Session{
			ID:         sessionID,
			UserID:     user.ID,
			ActiveRole: user.Role,
			TokenHash:  accessTokenHash[:],
			CreatedAt:  now,
			ExpiresAt:  now.Add(uc.accessTokenTTL),
			RevokedAt:  nil,
		}
		if err := tx.Sessions().Save(ctx, session); err != nil {
			return fmt.Errorf("save session: %w", err)
		}

		// Create and save RefreshToken.
		refreshTokenEntity := &ports.RefreshToken{
			ID:               refreshTokenEntityID,
			SessionID:        sessionID,
			TokenHash:        refreshTokenHash[:],
			FamilyID:         familyID,
			SuccessorTokenID: nil,
			CreatedAt:        now,
			ExpiresAt:        now.Add(uc.refreshTokenTTL),
			RevokedAt:        nil,
		}
		if err := tx.RefreshTokens().Save(ctx, refreshTokenEntity); err != nil {
			return fmt.Errorf("save refresh token: %w", err)
		}

		// Return the tokens to the caller.
		output = &LoginOutput{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
			ExpiresIn:    int64(uc.accessTokenTTL.Seconds()),
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return output, nil
}

// generateTokenID returns a random UUID-style hex string (32 chars) for use in
// token IDs and family IDs. Uses crypto/rand for security.
func generateTokenID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto/rand read: %w", err)
	}
	return hex.EncodeToString(b), nil
}
