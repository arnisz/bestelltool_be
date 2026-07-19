package usecases

import (
	"context"
	"fmt"
	"time"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

// CreateRequestInput contains input for creating a request aggregate.
type CreateRequestInput struct {
	RequestID                domain.RequestID
	TechnicianID             domain.UserID
	ContextRef               string
	ContextLabel             string
	WishFrom                 *time.Time
	WishUntil                *time.Time
	Note                     string
	RequestedResourceClasses []domain.ResourceClassID
	CreatedAt                time.Time
	Audit                    AuditMeta
}

// CreateRequestUseCase creates a new request and writes an audit event transactionally.
type CreateRequestUseCase struct {
	uow            ports.UnitOfWork
	eventPublisher ports.EventPublisher
}

// NewCreateRequestUseCase creates a CreateRequestUseCase.
func NewCreateRequestUseCase(uow ports.UnitOfWork) *CreateRequestUseCase {
	return NewCreateRequestUseCaseWithPublisher(uow, nil)
}

// NewCreateRequestUseCaseWithPublisher creates a CreateRequestUseCase with
// outbound event publishing.
func NewCreateRequestUseCaseWithPublisher(uow ports.UnitOfWork, eventPublisher ports.EventPublisher) *CreateRequestUseCase {
	return &CreateRequestUseCase{uow: uow, eventPublisher: withEventPublisher(eventPublisher)}
}

// Execute runs the transactional request-creation flow.
func (uc *CreateRequestUseCase) Execute(ctx context.Context, in CreateRequestInput) (*domain.Request, error) {
	if in.CreatedAt.IsZero() {
		return nil, fmt.Errorf("request created at: %w", domain.ErrRequiredField)
	}
	if err := validateAuditMeta(in.Audit); err != nil {
		return nil, err
	}

	req, err := domain.NewRequest(
		in.RequestID,
		in.TechnicianID,
		in.ContextRef,
		in.ContextLabel,
		in.WishFrom,
		in.WishUntil,
		in.Note,
		in.RequestedResourceClasses,
		in.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create request aggregate: %w", err)
	}

	err = uc.uow.WithinTransaction(ctx, func(ctx context.Context, tx ports.Transaction) error {
		if err := tx.Requests().Create(ctx, req); err != nil {
			return fmt.Errorf("create request %s: %w", req.ID, err)
		}

		event := newAuditEvent(
			in.Audit,
			domain.EntityTypeRequest,
			string(req.ID),
			"create_request",
			"",
			string(req.Status),
		)
		if err := tx.Audits().Write(ctx, event); err != nil {
			return fmt.Errorf("write request audit %s: %w", req.ID, err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := uc.eventPublisher.Publish(ctx, ports.Event{
		Type:         ports.EventTypeRequestCreated,
		RequestID:    req.ID,
		TechnicianID: req.TechnicianID,
		OccurredAt:   req.CreatedAt,
	}); err != nil {
		return nil, fmt.Errorf("publish request created event %s: %w", req.ID, err)
	}

	return req, nil
}
