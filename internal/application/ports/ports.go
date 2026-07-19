package ports

import (
	"context"

	"bestelltool_be/internal/domain"
)

// Principal represents an authenticated user identity.
type Principal struct {
	UserID domain.UserID
	Role   domain.ActorRole
}

// Authenticator verifies a bearer token and returns the associated Principal.
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (*Principal, error)
}

// RequestRepository provides persistence access for requests.
type RequestRepository interface {
	GetByID(ctx context.Context, id domain.RequestID) (*domain.Request, error)
	GetForUpdate(ctx context.Context, id domain.RequestID) (*domain.Request, error)
	Create(ctx context.Context, req *domain.Request) error
	Save(ctx context.Context, req *domain.Request) error
}

// ResourceRepository provides persistence access for resources.
type ResourceRepository interface {
	GetByID(ctx context.Context, id domain.ResourceID) (*domain.Resource, error)
	GetForUpdate(ctx context.Context, id domain.ResourceID) (*domain.Resource, error)
	Save(ctx context.Context, res *domain.Resource) error
}

// AllocationRepository provides persistence access for allocations.
type AllocationRepository interface {
	GetByID(ctx context.Context, id domain.AllocationID) (*domain.Allocation, error)
	GetForUpdate(ctx context.Context, id domain.AllocationID) (*domain.Allocation, error)
	Create(ctx context.Context, a *domain.Allocation) error
	Save(ctx context.Context, allocation *domain.Allocation) error
}

// AuditWriter stores audit events in the same transaction context.
type AuditWriter interface {
	Write(ctx context.Context, event domain.AuditEvent) error
}

// IdempotencyResult stores replayable outcome information.
type IdempotencyResult struct {
	StatusCode int
	Payload    []byte
	ErrorText  string
}

// IdempotencyStore stores and returns previously processed outcomes.
type IdempotencyStore interface {
	Get(ctx context.Context, actionID string) (*IdempotencyResult, error)
	Save(ctx context.Context, actionID string, result IdempotencyResult) error
}

// EventPublisher is an optional outbound event port.
type EventPublisher interface {
	Publish(ctx context.Context, event any) error
}

// Transaction provides transaction-bound repositories and writers.
type Transaction interface {
	Requests() RequestRepository
	Resources() ResourceRepository
	Allocations() AllocationRepository
	Audits() AuditWriter
	Idempotency() IdempotencyStore
}

// UnitOfWork executes a function atomically inside one transaction.
type UnitOfWork interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context, tx Transaction) error) error
}
