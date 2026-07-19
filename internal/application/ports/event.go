package ports

import (
	"context"
	"time"

	"bestelltool_be/internal/domain"
)

// EventType identifies a published domain event category.
type EventType string

const (
	EventTypeRequestCreated                    EventType = "request.created"
	EventTypeAllocationReturnRequested         EventType = "allocation.return_requested"
	EventTypeAllocationDirectTransferCompleted EventType = "allocation.direct_transfer.completed"
	EventTypeAllocationDirectTransferActivated EventType = "allocation.direct_transfer.activated"
)

// Event is emitted by write use cases and consumed by stream adapters.
//
// TechnicianID is mandatory and used by adapters for role-based fan-out:
// - dispatcher receives all events
// - technician receives only matching TechnicianID events
type Event struct {
	Type         EventType           `json:"type"`
	RequestID    domain.RequestID    `json:"request_id,omitzero"`
	AllocationID domain.AllocationID `json:"allocation_id,omitzero"`
	ResourceID   domain.ResourceID   `json:"resource_id,omitzero"`
	TechnicianID domain.UserID       `json:"technician_id"`
	OccurredAt   time.Time           `json:"occurred_at"`
	Data         map[string]string   `json:"data,omitzero"`
}

// EventPublisher publishes events asynchronously and should never block callers.
type EventPublisher interface {
	Publish(ctx context.Context, event Event) error
}
