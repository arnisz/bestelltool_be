package usecases

import (
	"context"
	"fmt"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

// LogoutInput identifies the authenticated session to revoke.
type LogoutInput struct {
	SessionID string
	ActorID   domain.UserID
	ActorRole domain.ActorRole
}

// LogoutUseCase revokes the authenticated session and audits the state change.
type LogoutUseCase struct {
	uow   ports.UnitOfWork
	clock ports.Clock
}

func NewLogoutUseCase(uow ports.UnitOfWork, clock ports.Clock) *LogoutUseCase {
	return &LogoutUseCase{uow: uow, clock: clock}
}

func (uc *LogoutUseCase) Execute(ctx context.Context, in LogoutInput) error {
	if in.SessionID == "" || in.ActorID == "" || in.ActorRole == "" {
		return fmt.Errorf("logout input: %w", domain.ErrRequiredField)
	}

	return uc.uow.WithinTransaction(ctx, func(ctx context.Context, tx ports.Transaction) error {
		session, err := tx.Sessions().GetForUpdate(ctx, in.SessionID)
		if err != nil || session == nil {
			return ports.ErrUnauthenticated
		}
		if session.UserID != in.ActorID || session.ActiveRole != in.ActorRole || session.RevokedAt != nil {
			return ports.ErrUnauthenticated
		}

		now := uc.clock.Now()
		if err := tx.Sessions().Revoke(ctx, session.ID, now); err != nil {
			return fmt.Errorf("revoke session: %w", err)
		}
		event := newAuditEvent(AuditMeta{ActorID: in.ActorID, ActorRole: in.ActorRole}, domain.EntityTypeSession, session.ID, string(domain.ActionSessionRevoke), "active", "revoked")
		if err := tx.AuditEvents().RecordEvent(ctx, event); err != nil {
			return fmt.Errorf("record logout audit event: %w", err)
		}
		return nil
	})
}
