package usecases

import (
	"context"
	"errors"
	"fmt"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

// ChangeOwnPasswordInput contains credentials for an authenticated user to
// change their own password. ActorRole and CurrentSessionID are sourced from
// the authenticated principal by the HTTP adapter.
type ChangeOwnPasswordInput struct {
	UserID           domain.UserID
	ActorRole        domain.ActorRole
	CurrentSessionID string
	OldPassword      string
	NewPassword      string
}

// ChangeOwnPasswordUseCase changes an authenticated user's local password.
type ChangeOwnPasswordUseCase struct {
	uow            ports.UnitOfWork
	passwordHasher ports.PasswordHasher
	clock          ports.Clock
}

func NewChangeOwnPasswordUseCase(uow ports.UnitOfWork, passwordHasher ports.PasswordHasher, clock ports.Clock) *ChangeOwnPasswordUseCase {
	return &ChangeOwnPasswordUseCase{uow: uow, passwordHasher: passwordHasher, clock: clock}
}

func (uc *ChangeOwnPasswordUseCase) Execute(ctx context.Context, in ChangeOwnPasswordInput) error {
	if in.UserID == "" {
		return fmt.Errorf("user id: %w", domain.ErrRequiredField)
	}
	if in.ActorRole == "" {
		return fmt.Errorf("actor role: %w", domain.ErrRequiredField)
	}
	if in.CurrentSessionID == "" {
		return fmt.Errorf("current session id: %w", domain.ErrRequiredField)
	}
	if in.OldPassword == "" {
		return fmt.Errorf("old password: %w", domain.ErrRequiredField)
	}
	if in.NewPassword == "" {
		return fmt.Errorf("new password: %w", domain.ErrRequiredField)
	}

	return uc.uow.WithinTransaction(ctx, func(ctx context.Context, tx ports.Transaction) error {
		user, err := tx.Users().GetByID(ctx, in.UserID)
		if err != nil {
			return fmt.Errorf("get user: %w", err)
		}
		if user == nil || !user.IsActive {
			return ports.ErrUnauthenticated
		}

		identity, err := tx.AuthIdentities().GetByUserID(ctx, in.UserID)
		if err != nil && !errors.Is(err, ports.ErrNotFound) {
			return fmt.Errorf("get auth identity: %w", err)
		}
		if identity == nil || errors.Is(err, ports.ErrNotFound) {
			return ports.ErrUnauthenticated
		}

		ok, _, err := uc.passwordHasher.Verify(in.OldPassword, identity.PasswordHash)
		if err != nil {
			return fmt.Errorf("verify old password: %w", err)
		}
		if !ok {
			event := newAuditEvent(AuditMeta{ActorID: in.UserID, ActorRole: in.ActorRole}, domain.EntityTypeAuthIdentity, string(in.UserID), string(domain.ActionAuthPasswordChangeFailed), "", "")
			if err := tx.AuditEvents().RecordEvent(ctx, event); err != nil {
				return fmt.Errorf("record password change failure audit event: %w", err)
			}
			return ports.ErrCredentialsInvalid
		}

		passwordHash, err := uc.passwordHasher.Hash(in.NewPassword)
		if err != nil {
			return fmt.Errorf("hash new password: %w", err)
		}
		identity.PasswordHash = passwordHash
		if err := tx.AuthIdentities().Save(ctx, identity); err != nil {
			return fmt.Errorf("save auth identity: %w", err)
		}
		if err := tx.Sessions().RevokeAllForUserExcept(ctx, in.UserID, in.CurrentSessionID, uc.clock.Now()); err != nil {
			return fmt.Errorf("revoke other user sessions: %w", err)
		}

		event := newAuditEvent(AuditMeta{ActorID: in.UserID, ActorRole: in.ActorRole}, domain.EntityTypeAuthIdentity, string(in.UserID), string(domain.ActionAuthPasswordChanged), "active", "changed")
		if err := tx.AuditEvents().RecordEvent(ctx, event); err != nil {
			return fmt.Errorf("record password change audit event: %w", err)
		}

		return nil
	})
}
