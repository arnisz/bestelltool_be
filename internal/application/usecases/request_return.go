package usecases

import (
	"context"
	"fmt"
	"time"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

// RequestReturnInput contains input for requesting allocation return.
type RequestReturnInput struct {
	AllocationID domain.AllocationID
	At           time.Time
	Audit        AuditMeta
}

// RequestReturnUseCase requests the return of an allocation.
type RequestReturnUseCase struct {
	uow            ports.UnitOfWork
	eventPublisher ports.EventPublisher
}

// NewRequestReturnUseCase creates a RequestReturnUseCase.
func NewRequestReturnUseCase(uow ports.UnitOfWork) *RequestReturnUseCase {
	return NewRequestReturnUseCaseWithPublisher(uow, nil)
}

// NewRequestReturnUseCaseWithPublisher creates a RequestReturnUseCase with
// outbound event publishing.
func NewRequestReturnUseCaseWithPublisher(uow ports.UnitOfWork, eventPublisher ports.EventPublisher) *RequestReturnUseCase {
	return &RequestReturnUseCase{uow: uow, eventPublisher: withEventPublisher(eventPublisher)}
}

// Execute runs the transactional request-return flow.
func (uc *RequestReturnUseCase) Execute(ctx context.Context, in RequestReturnInput) error {
	if in.AllocationID == "" {
		return fmt.Errorf("allocation id: %w", domain.ErrRequiredField)
	}
	if in.At.IsZero() {
		return fmt.Errorf("request return time: %w", domain.ErrRequiredField)
	}
	if err := validateAuditMeta(in.Audit); err != nil {
		return err
	}

	var streamEvent ports.Event
	err := uc.uow.WithinTransaction(ctx, func(ctx context.Context, tx ports.Transaction) error {
		repo := tx.Allocations()
		allocation, err := repo.GetForUpdate(ctx, in.AllocationID)
		if err != nil {
			return fmt.Errorf("load allocation %s: %w", in.AllocationID, err)
		}
		if allocation == nil {
			return fmt.Errorf("allocation %s missing: %w", in.AllocationID, domain.ErrInvalidState)
		}

		fromStatus := string(allocation.Status)
		if err := allocation.RequestReturn(in.At); err != nil {
			return fmt.Errorf("request return for allocation %s: %w", in.AllocationID, err)
		}

		if err := repo.Save(ctx, allocation); err != nil {
			return fmt.Errorf("save allocation %s: %w", in.AllocationID, err)
		}

		event := newAuditEvent(
			in.Audit,
			domain.EntityTypeAllocation,
			string(allocation.ID),
			"request_return",
			fromStatus,
			string(allocation.Status),
		)
		if err := tx.Audits().Write(ctx, event); err != nil {
			return fmt.Errorf("write allocation audit %s: %w", in.AllocationID, err)
		}

		req, err := tx.Requests().GetByID(ctx, allocation.RequestID)
		if err != nil {
			return fmt.Errorf("load request %s for allocation %s: %w", allocation.RequestID, in.AllocationID, err)
		}
		if req == nil {
			return fmt.Errorf("request %s for allocation %s: %w", allocation.RequestID, in.AllocationID, ports.ErrNotFound)
		}

		streamEvent = ports.Event{
			Type:         ports.EventTypeAllocationReturnRequested,
			RequestID:    allocation.RequestID,
			AllocationID: allocation.ID,
			ResourceID:   allocation.ResourceID,
			TechnicianID: req.TechnicianID,
			OccurredAt:   in.At,
		}

		return nil
	})
	if err != nil {
		return err
	}

	if err := uc.eventPublisher.Publish(ctx, streamEvent); err != nil {
		return fmt.Errorf("publish allocation return-requested event %s: %w", in.AllocationID, err)
	}

	return nil
}
