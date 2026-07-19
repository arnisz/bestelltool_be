package usecases

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"bestelltool_be/internal/domain"
)

// generateAuditID returns a random 32-character hex string for audit event IDs.
func generateAuditID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("generateAuditID: crypto/rand unavailable: %v", err))
	}
	return hex.EncodeToString(b)
}

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
		ID:               generateAuditID(),
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
