package usecases

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
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
	defaultRefreshTokenTTL = 30 * 24 * time.Hour
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

	var (
		output             *LoginOutput
		credentialsInvalid bool
	)
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

		recordFailure := func(user *domain.User) error {
			if user == nil {
				return nil
			}
			meta := AuditMeta{ActorID: user.ID, ActorRole: user.Role}
			if err := validateAuditMeta(meta); err != nil {
				return err
			}

			event := newAuditEvent(
				meta,
				domain.EntityTypeAuthIdentity,
				string(user.ID),
				string(domain.ActionAuthLoginFailed),
				"",
				"",
			)
			if err := tx.AuditEvents().RecordEvent(ctx, event); err != nil {
				return fmt.Errorf("record failed login audit event: %w", err)
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
				if err := recordFailure(user); err != nil {
					return err
				}
				credentialsInvalid = true
				return nil
			}
		}

		// If user doesn't exist, return generic error now.
		if user == nil {
			if err := runDummyVerify(); err != nil {
				return err
			}
			credentialsInvalid = true
			return nil
		}

		roles, err := tx.UserRoles().RolesForUser(ctx, user.ID)
		if err != nil {
			return fmt.Errorf("load user roles: %w", err)
		}
		if !slices.Contains(roles, user.Role) {
			_, _, _ = uc.passwordHasher.Verify(in.Password, authIdentity.PasswordHash)
			if err := recordFailure(user); err != nil {
				return err
			}
			credentialsInvalid = true
			return nil
		}

		// Check if user is active.
		if !user.IsActive {
			// Still run the password verification for constant time (SEC-03).
			_, _, _ = uc.passwordHasher.Verify(in.Password, authIdentity.PasswordHash)
			if err := recordFailure(user); err != nil {
				return err
			}
			credentialsInvalid = true
			return nil
		}

		// Verify password.
		ok, needsRehash, err := uc.passwordHasher.Verify(in.Password, authIdentity.PasswordHash)
		if err != nil {
			return fmt.Errorf("verify password: %w", err)
		}
		if !ok {
			if err := recordFailure(user); err != nil {
				return err
			}
			credentialsInvalid = true
			return nil
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

		session, accessToken, refreshToken, err := issueSessionWithTokens(
			ctx, tx, uc.secretGenerator, uc.clock, user.ID, user.Role, uc.accessTokenTTL, uc.refreshTokenTTL,
		)
		if err != nil {
			return err
		}

		meta := AuditMeta{ActorID: user.ID, ActorRole: user.Role}
		if err := validateAuditMeta(meta); err != nil {
			return err
		}
		event := newAuditEvent(
			meta,
			domain.EntityTypeSession,
			session.ID,
			string(domain.ActionSessionCreate),
			"",
			"active",
		)
		if err := tx.AuditEvents().RecordEvent(ctx, event); err != nil {
			return fmt.Errorf("record login audit event: %w", err)
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
	if credentialsInvalid {
		return nil, ports.ErrCredentialsInvalid
	}

	return output, nil
}

func issueSessionWithTokens(
	ctx context.Context,
	tx ports.Transaction,
	secretGenerator ports.SecretGenerator,
	clock ports.Clock,
	userID domain.UserID,
	role domain.ActorRole,
	accessTokenTTL time.Duration,
	refreshTokenTTL time.Duration,
) (session *ports.Session, accessToken, refreshToken string, err error) {
	accessSecret, err := secretGenerator.GenerateToken()
	if err != nil {
		return nil, "", "", fmt.Errorf("generate access token secret: %w", err)
	}
	accessSecret = generatedTokenSecret(accessSecret)
	refreshSecret, err := secretGenerator.GenerateToken()
	if err != nil {
		return nil, "", "", fmt.Errorf("generate refresh token secret: %w", err)
	}
	refreshSecret = generatedTokenSecret(refreshSecret)
	accessTokenID, err := generateTokenID()
	if err != nil {
		return nil, "", "", fmt.Errorf("generate access token id: %w", err)
	}
	refreshTokenID, err := generateTokenID()
	if err != nil {
		return nil, "", "", fmt.Errorf("generate refresh token id: %w", err)
	}
	accessToken = accessTokenPrefix + accessTokenID + "." + accessSecret
	refreshToken = refreshTokenPrefix + refreshTokenID + "." + refreshSecret
	accessTokenHash := sha256.Sum256([]byte(accessSecret))
	refreshTokenHash := sha256.Sum256([]byte(refreshSecret))
	now := clock.Now()
	sessionID, err := generateTokenID()
	if err != nil {
		return nil, "", "", fmt.Errorf("generate session id: %w", err)
	}
	familyID, err := generateTokenID()
	if err != nil {
		return nil, "", "", fmt.Errorf("generate refresh family id: %w", err)
	}
	session = &ports.Session{ID: sessionID, UserID: userID, ActiveRole: role, TokenHash: accessTokenHash[:], CreatedAt: now, ExpiresAt: now.Add(accessTokenTTL)}
	if err := tx.Sessions().Save(ctx, session); err != nil {
		return nil, "", "", fmt.Errorf("save session: %w", err)
	}
	refreshTokenEntity := &ports.RefreshToken{ID: refreshTokenID, SessionID: sessionID, TokenHash: refreshTokenHash[:], FamilyID: familyID, CreatedAt: now, ExpiresAt: now.Add(refreshTokenTTL)}
	if err := tx.RefreshTokens().Save(ctx, refreshTokenEntity); err != nil {
		return nil, "", "", fmt.Errorf("save refresh token: %w", err)
	}
	return session, accessToken, refreshToken, nil
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

func generatedTokenSecret(generated string) string {
	if !(strings.HasPrefix(generated, accessTokenPrefix) || strings.HasPrefix(generated, refreshTokenPrefix)) {
		return generated
	}
	_, secret, ok := strings.Cut(generated, ".")
	if !ok || secret == "" || strings.Contains(secret, ".") {
		return generated
	}
	return secret
}
