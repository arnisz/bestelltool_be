package usecases

import (
	"fmt"
	"time"

	"bestelltool_be/internal/domain"
)

// AuditMeta contains shared metadata for audit creation in use cases.
type AuditMeta struct {
	ClientOccurredAt *time.Time
	ClientSeq        *int64
	ActorID          domain.UserID
	ActorRole        domain.ActorRole
	Note             string
}

func newAuditEvent(
	meta AuditMeta,
	entityType domain.EntityType,
	entityID string,
	action string,
	fromStatus string,
	toStatus string,
) domain.AuditEvent {
	return domain.AuditEvent{
		ClientOccurredAt: meta.ClientOccurredAt,
		ClientSeq:        meta.ClientSeq,
		ActorID:          meta.ActorID,
		ActorRole:        meta.ActorRole,
		EntityType:       entityType,
		EntityID:         entityID,
		Action:           action,
		FromStatus:       fromStatus,
		ToStatus:         toStatus,
		Note:             meta.Note,
	}
}

func validateAuditMeta(meta AuditMeta) error {
	if meta.ActorID == "" {
		return fmt.Errorf("audit actor id: %w", domain.ErrRequiredField)
	}
	if meta.ActorRole == "" {
		return fmt.Errorf("audit actor role: %w", domain.ErrRequiredField)
	}

	return nil
}
