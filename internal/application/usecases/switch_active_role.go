package usecases

import (
	"context"
	"fmt"
	"time"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

type SwitchActiveRoleInput struct {
	UserID           domain.UserID
	CurrentSessionID string
	RequestedRole    domain.ActorRole
}

type SwitchActiveRoleOutput struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
	ActiveRole   domain.ActorRole
}

type SwitchActiveRoleUseCase struct {
	uow             ports.UnitOfWork
	secretGenerator ports.SecretGenerator
	clock           ports.Clock
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

func NewSwitchActiveRoleUseCase(uow ports.UnitOfWork, secretGenerator ports.SecretGenerator, clock ports.Clock) *SwitchActiveRoleUseCase {
	return &SwitchActiveRoleUseCase{uow: uow, secretGenerator: secretGenerator, clock: clock, accessTokenTTL: defaultAccessTokenTTL, refreshTokenTTL: defaultRefreshTokenTTL}
}

func (uc *SwitchActiveRoleUseCase) Execute(ctx context.Context, in SwitchActiveRoleInput) (*SwitchActiveRoleOutput, error) {
	var out *SwitchActiveRoleOutput
	err := uc.uow.WithinTransaction(ctx, func(ctx context.Context, tx ports.Transaction) error {
		roleHeld, err := tx.UserRoles().HasRoleForUpdate(ctx, in.UserID, in.RequestedRole)
		if err != nil {
			return fmt.Errorf("lock requested role assignment: %w", err)
		}
		if !roleHeld {
			return ports.ErrForbidden
		}
		session, err := tx.Sessions().GetForUpdate(ctx, in.CurrentSessionID)
		if err != nil || session == nil {
			return ports.ErrTokenInvalid
		}
		now := uc.clock.Now()
		if session.UserID != in.UserID || session.RevokedAt != nil || !now.Before(session.ExpiresAt) {
			return ports.ErrTokenInvalid
		}
		if err := tx.Sessions().Revoke(ctx, in.CurrentSessionID, now); err != nil {
			return fmt.Errorf("revoke old session: %w", err)
		}
		newSession, accessToken, refreshToken, err := issueSessionWithTokens(ctx, tx, uc.secretGenerator, uc.clock, in.UserID, in.RequestedRole, uc.accessTokenTTL, uc.refreshTokenTTL)
		if err != nil {
			return err
		}
		meta := AuditMeta{ActorID: session.UserID, ActorRole: session.ActiveRole, Note: "role_switch"}
		if err := validateAuditMeta(meta); err != nil {
			return err
		}
		oldEvent := newAuditEvent(meta, domain.EntityTypeSession, in.CurrentSessionID, string(domain.ActionSessionRevoke), "active", "revoked")
		if err := tx.AuditEvents().RecordEvent(ctx, oldEvent); err != nil {
			return fmt.Errorf("record old session audit event: %w", err)
		}
		newEvent := newAuditEvent(meta, domain.EntityTypeSession, newSession.ID, string(domain.ActionSessionCreate), "", "active")
		if err := tx.AuditEvents().RecordEvent(ctx, newEvent); err != nil {
			return fmt.Errorf("record new session audit event: %w", err)
		}
		out = &SwitchActiveRoleOutput{AccessToken: accessToken, RefreshToken: refreshToken, ExpiresIn: int64(uc.accessTokenTTL.Seconds()), ActiveRole: in.RequestedRole}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
