package postgres

import (
	"bestelltool_be/internal/domain"
	"context"
)

type auditWriter struct {
	q querier
}

func (w *auditWriter) Write(ctx context.Context, event domain.AuditEvent) error {
	_, err := w.q.Exec(ctx, `
INSERT INTO audit_events(
    id,
    client_occurred_at,
    client_seq,
    actor_id,
    actor_role,
    entity_type,
    entity_id,
    action,
    from_status,
    to_status,
    note
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		event.ID,
		optionalTime(event.ClientOccurredAt),
		event.ClientSeq,
		string(event.ActorID),
		string(event.ActorRole),
		string(event.EntityType),
		event.EntityID,
		event.Action,
		event.FromStatus,
		event.ToStatus,
		event.Note,
	)
	if err != nil {
		return mapWriteError("audit event", err)
	}

	return nil
}
