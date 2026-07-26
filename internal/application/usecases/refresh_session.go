package usecases

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"strings"
	"time"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

const defaultRefreshReplayGrace = 30 * time.Second

// RefreshSessionInput contains an opaque refresh token supplied by the client.
type RefreshSessionInput struct {
	RefreshToken string
}

// RefreshSessionOutput contains the rotated token pair.
type RefreshSessionOutput struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
}

// RefreshSessionUseCase rotates refresh tokens and enforces SEC-08 replay
// detection. It never stores plaintext tokens; D-2 retry recovery decrypts
// the predecessor's short-lived encrypted successor.
type RefreshSessionUseCase struct {
	uow             ports.UnitOfWork
	secretGenerator ports.SecretGenerator
	tokenEncryptor  ports.TokenEncryptor
	clock           ports.Clock
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	replayGrace     time.Duration
}

func NewRefreshSessionUseCase(uow ports.UnitOfWork, secretGenerator ports.SecretGenerator, tokenEncryptor ports.TokenEncryptor, clock ports.Clock) *RefreshSessionUseCase {
	return NewRefreshSessionUseCaseWithConfig(uow, secretGenerator, tokenEncryptor, clock, defaultAccessTokenTTL, defaultRefreshTokenTTL, defaultRefreshReplayGrace)
}

func NewRefreshSessionUseCaseWithConfig(uow ports.UnitOfWork, secretGenerator ports.SecretGenerator, tokenEncryptor ports.TokenEncryptor, clock ports.Clock, accessTokenTTL, refreshTokenTTL, replayGrace time.Duration) *RefreshSessionUseCase {
	return &RefreshSessionUseCase{uow: uow, secretGenerator: secretGenerator, tokenEncryptor: tokenEncryptor, clock: clock, accessTokenTTL: accessTokenTTL, refreshTokenTTL: refreshTokenTTL, replayGrace: replayGrace}
}

func (uc *RefreshSessionUseCase) Execute(ctx context.Context, in RefreshSessionInput) (*RefreshSessionOutput, error) {
	id, secret, err := splitRefreshToken(in.RefreshToken)
	if err != nil {
		return nil, ports.ErrTokenInvalid
	}
	hash := sha256.Sum256([]byte(secret))
	var (
		out    *RefreshSessionOutput
		replay bool
	)
	err = uc.uow.WithinTransaction(ctx, func(ctx context.Context, tx ports.Transaction) error {
		presented, err := tx.RefreshTokens().GetForUpdate(ctx, id)
		if err != nil || presented == nil || subtle.ConstantTimeCompare(hash[:], presented.TokenHash) != 1 {
			return ports.ErrTokenInvalid
		}
		session, err := tx.Sessions().GetForUpdate(ctx, presented.SessionID)
		if err != nil || session == nil || session.RevokedAt != nil {
			return ports.ErrTokenInvalid
		}
		if !uc.clock.Now().Before(session.ExpiresAt) {
			return ports.ErrTokenExpired
		}
		now := uc.clock.Now()
		if presented.SuccessorTokenID != nil {
			successor, err := tx.RefreshTokens().GetByID(ctx, *presented.SuccessorTokenID)
			if err != nil {
				return fmt.Errorf("load refresh successor: %w", err)
			}
			elapsed := now.Sub(successor.CreatedAt)
			if elapsed >= 0 && elapsed <= uc.replayGrace && len(presented.EncryptedSuccessor) > 0 {
				refreshToken, err := uc.tokenEncryptor.Decrypt(presented.EncryptedSuccessor)
				if err != nil {
					return fmt.Errorf("decrypt refresh successor: %w", err)
				}
				accessToken, accessHash, err := uc.issueAccessToken()
				if err != nil {
					return err
				}
				session.TokenHash = accessHash
				session.ExpiresAt = now.Add(uc.refreshTokenTTL)
				if err := tx.Sessions().Update(ctx, session); err != nil {
					return fmt.Errorf("update session access token: %w", err)
				}
				out = &RefreshSessionOutput{AccessToken: accessToken, RefreshToken: string(refreshToken), ExpiresIn: int64(uc.accessTokenTTL.Seconds())}
				return nil
			}
			if err := uc.revokeReplay(ctx, tx, session, presented.FamilyID, now); err != nil {
				return err
			}
			replay = true
			return nil
		}
		if presented.RevokedAt != nil || !now.Before(presented.ExpiresAt) {
			if !now.Before(presented.ExpiresAt) {
				return ports.ErrTokenExpired
			}
			return ports.ErrTokenInvalid
		}
		accessToken, accessHash, err := uc.issueAccessToken()
		if err != nil {
			return err
		}
		refreshSecret, err := uc.secretGenerator.GenerateToken()
		if err != nil {
			return fmt.Errorf("generate refresh token secret: %w", err)
		}
		refreshID, err := generateTokenID()
		if err != nil {
			return fmt.Errorf("generate refresh token id: %w", err)
		}
		refreshToken := refreshTokenPrefix + refreshID + "." + refreshSecret
		encryptedSuccessor, err := uc.tokenEncryptor.Encrypt([]byte(refreshToken))
		if err != nil {
			return fmt.Errorf("encrypt refresh successor: %w", err)
		}
		presented.SuccessorTokenID = &refreshID
		presented.EncryptedSuccessor = encryptedSuccessor
		if err := tx.RefreshTokens().Update(ctx, presented); err != nil {
			return fmt.Errorf("consume refresh token: %w", err)
		}
		refreshHash := sha256.Sum256([]byte(refreshSecret))
		if err := tx.RefreshTokens().Save(ctx, &ports.RefreshToken{ID: refreshID, SessionID: session.ID, TokenHash: refreshHash[:], FamilyID: presented.FamilyID, CreatedAt: now, ExpiresAt: now.Add(uc.refreshTokenTTL)}); err != nil {
			return fmt.Errorf("save rotated refresh token: %w", err)
		}
		session.TokenHash = accessHash
		session.ExpiresAt = now.Add(uc.refreshTokenTTL)
		if err := tx.Sessions().Update(ctx, session); err != nil {
			return fmt.Errorf("update session access token: %w", err)
		}
		event := newAuditEvent(AuditMeta{ActorID: session.UserID, ActorRole: session.ActiveRole}, domain.EntityTypeSession, session.ID, string(domain.ActionSessionRefresh), "active", "active")
		if err := tx.AuditEvents().RecordEvent(ctx, event); err != nil {
			return fmt.Errorf("record refresh audit event: %w", err)
		}
		out = &RefreshSessionOutput{AccessToken: accessToken, RefreshToken: refreshToken, ExpiresIn: int64(uc.accessTokenTTL.Seconds())}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if replay {
		return nil, ports.ErrTokenInvalid
	}
	return out, nil
}

func (uc *RefreshSessionUseCase) issueAccessToken() (string, []byte, error) {
	secret, err := uc.secretGenerator.GenerateToken()
	if err != nil {
		return "", nil, fmt.Errorf("generate access token secret: %w", err)
	}
	id, err := generateTokenID()
	if err != nil {
		return "", nil, fmt.Errorf("generate access token id: %w", err)
	}
	hash := sha256.Sum256([]byte(secret))
	return accessTokenPrefix + id + "." + secret, hash[:], nil
}

func (uc *RefreshSessionUseCase) revokeReplay(ctx context.Context, tx ports.Transaction, session *ports.Session, familyID string, now time.Time) error {
	tokens, err := tx.RefreshTokens().GetFamilyForUpdate(ctx, familyID)
	if err != nil {
		return fmt.Errorf("lock refresh family: %w", err)
	}
	for _, token := range tokens {
		token.RevokedAt = &now
		if err := tx.RefreshTokens().Update(ctx, token); err != nil {
			return fmt.Errorf("revoke refresh token: %w", err)
		}
	}
	if err := tx.Sessions().Revoke(ctx, session.ID, now); err != nil {
		return fmt.Errorf("revoke replayed session: %w", err)
	}
	event := newAuditEvent(AuditMeta{ActorID: session.UserID, ActorRole: session.ActiveRole}, domain.EntityTypeSession, session.ID, string(domain.ActionSessionReplayDetected), "active", "revoked")
	if err := tx.AuditEvents().RecordEvent(ctx, event); err != nil {
		return fmt.Errorf("record replay audit event: %w", err)
	}
	return nil
}

func splitRefreshToken(token string) (string, string, error) {
	rest, ok := strings.CutPrefix(token, refreshTokenPrefix)
	if !ok {
		return "", "", fmt.Errorf("invalid refresh token prefix")
	}
	id, secret, ok := strings.Cut(rest, ".")
	if !ok || id == "" || secret == "" || strings.Contains(secret, ".") {
		return "", "", fmt.Errorf("invalid refresh token format")
	}
	return id, secret, nil
}
