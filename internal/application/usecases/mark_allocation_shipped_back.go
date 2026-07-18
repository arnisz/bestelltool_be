package usecases

import (
	"context"
	"fmt"
	"time"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

// MarkAllocationShippedBackInput contains input for shipping back an allocation.
type MarkAllocationShippedBackInput struct {
	AllocationID domain.AllocationID
	At           time.Time
	Audit        AuditMeta
}

// MarkAllocationShippedBackUseCase marks allocation as shipped back.
type MarkAllocationShippedBackUseCase struct {
	uow ports.UnitOfWork
}

// NewMarkAllocationShippedBackUseCase creates a MarkAllocationShippedBackUseCase.
func NewMarkAllocationShippedBackUseCase(uow ports.UnitOfWork) *MarkAllocationShippedBackUseCase {
	return &MarkAllocationShippedBackUseCase{uow: uow}
}

// Execute runs the transactional mark-shipped-back flow.
func (uc *MarkAllocationShippedBackUseCase) Execute(ctx context.Context, in MarkAllocationShippedBackInput) error {
	if in.AllocationID == "" {
		return fmt.Errorf("allocation id: %w", domain.ErrRequiredField)
	}
	if in.At.IsZero() {
		return fmt.Errorf("shipped back time: %w", domain.ErrRequiredField)
	}
	if err := validateAuditMeta(in.Audit); err != nil {
		return err
	}

	return uc.uow.WithinTransaction(ctx, func(ctx context.Context, tx ports.Transaction) error {
		repo := tx.Allocations()
		allocation, err := repo.GetForUpdate(ctx, in.AllocationID)
		if err != nil {
			return fmt.Errorf("load allocation %s: %w", in.AllocationID, err)
		}
		if allocation == nil {
			return fmt.Errorf("allocation %s missing: %w", in.AllocationID, domain.ErrInvalidState)
		}

		fromStatus := string(allocation.Status)
		if err := allocation.MarkShippedBack(in.At); err != nil {
			return fmt.Errorf("mark shipped back for allocation %s: %w", in.AllocationID, err)
		}

		if err := repo.Save(ctx, allocation); err != nil {
			return fmt.Errorf("save allocation %s: %w", in.AllocationID, err)
		}

		event := newAuditEvent(
			in.Audit,
			domain.EntityTypeAllocation,
			string(allocation.ID),
			"mark_shipped_back",
			fromStatus,
			string(allocation.Status),
		)
		if err := tx.Audits().Write(ctx, event); err != nil {
			return fmt.Errorf("write allocation audit %s: %w", in.AllocationID, err)
		}

		return nil
	})
}
