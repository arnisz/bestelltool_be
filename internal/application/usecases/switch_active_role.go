package usecases

import (
	"context"
	"fmt"
	"slices"
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
		roles, err := tx.UserRoles().RolesForUser(ctx, in.UserID)
		if err != nil {
			return fmt.Errorf("load user roles: %w", err)
		}
		if !slices.Contains(roles, in.RequestedRole) {
			return ports.ErrForbidden
		}
		session, accessToken, refreshToken, err := issueSessionWithTokens(ctx, tx, uc.secretGenerator, uc.clock, in.UserID, in.RequestedRole, uc.accessTokenTTL, uc.refreshTokenTTL)
		if err != nil {
			return err
		}
		now := uc.clock.Now()
		if err := tx.Sessions().Revoke(ctx, in.CurrentSessionID, now); err != nil {
			return fmt.Errorf("revoke old session: %w", err)
		}
		meta := AuditMeta{ActorID: in.UserID, ActorRole: in.RequestedRole, Note: "role_switch"}
		oldEvent := newAuditEvent(meta, domain.EntityTypeSession, in.CurrentSessionID, string(domain.ActionSessionRevoke), "active", "revoked")
		if err := tx.AuditEvents().RecordEvent(ctx, oldEvent); err != nil {
			return fmt.Errorf("record old session audit event: %w", err)
		}
		newEvent := newAuditEvent(meta, domain.EntityTypeSession, session.ID, string(domain.ActionSessionCreate), "", "active")
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
