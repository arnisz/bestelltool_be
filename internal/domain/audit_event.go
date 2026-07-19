package domain

import "time"

// ActorRole describes the actor role for an audit event.
type ActorRole string

const (
	// ActorRoleTechnician identifies technician actors.
	ActorRoleTechnician ActorRole = "technician"
	// ActorRoleDispatcher identifies Dispatcher actors.
	// Both the domain layer and the database use the canonical value "dispatcher".
	ActorRoleDispatcher ActorRole = "dispatcher"
	// ActorRoleAdmin identifies admin actors.
	// Admin may only perform read-only operations; write operations require technician or dispatcher.
	ActorRoleAdmin ActorRole = "admin"
	// ActorRoleSystem identifies system actors.
	ActorRoleSystem ActorRole = "system"
)

// EntityType identifies the audited entity kind.
type EntityType string

const (
	// EntityTypeRequest identifies request events.
	EntityTypeRequest EntityType = "request"
	// EntityTypeAllocation identifies allocation events.
	EntityTypeAllocation EntityType = "allocation"
	// EntityTypeResource identifies resource events.
	EntityTypeResource EntityType = "resource"
)

// AuditEvent is a pure domain data structure for future auditing.
type AuditEvent struct {
	ID               string
	ClientOccurredAt *time.Time
	ClientSeq        *int64
	ServerRecordedAt time.Time
	ActorID          UserID
	ActorRole        ActorRole
	EntityType       EntityType
	EntityID         string
	Action           string
	FromStatus       string
	ToStatus         string
	Note             string
}
