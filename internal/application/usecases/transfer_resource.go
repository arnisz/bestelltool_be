package usecases

import (
	"context"
	"fmt"
	"time"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

// TransferResourceInput contains input for a direct site-to-site resource transfer.
// ResourceID is derived from the old allocation — it is not accepted from the caller
// to prevent inconsistency.
type TransferResourceInput struct {
	OldAllocationID domain.AllocationID // allocation to complete
	NewAllocationID domain.AllocationID // new allocation to activate
	TargetRequestID domain.RequestID    // request receiving the resource
	PlannedFrom     time.Time           // planning window for the new allocation
	PlannedUntil    time.Time
	At              time.Time // timestamp for completing the old allocation / creating the new one
	Audit           AuditMeta
}

// TransferResourceUseCase performs a direct site-to-site resource transfer within a
// single atomic transaction. Transaction lock order: Request → Allocation → Resource.
type TransferResourceUseCase struct {
	uow ports.UnitOfWork
}

// NewTransferResourceUseCase creates a TransferResourceUseCase.
func NewTransferResourceUseCase(uow ports.UnitOfWork) *TransferResourceUseCase {
	return &TransferResourceUseCase{uow: uow}
}

// Execute runs the transactional direct-transfer flow.
//
// Critical ordering inside the transaction:
//  1. All locks are acquired first (Request → Allocation → Resource) and all guards
//     are checked before any mutations, so a failing guard never leaves partial state.
//  2. The old allocation is saved as completed BEFORE the new allocation is created.
//     This releases the unique partial index (uq_allocations_single_active_resource)
//     for the resource, preventing a spurious PG-23505 mid-transaction.
func (uc *TransferResourceUseCase) Execute(ctx context.Context, in TransferResourceInput) error {
	if in.OldAllocationID == "" {
		return fmt.Errorf("old allocation id: %w", domain.ErrRequiredField)
	}
	if in.NewAllocationID == "" {
		return fmt.Errorf("new allocation id: %w", domain.ErrRequiredField)
	}
	if in.TargetRequestID == "" {
		return fmt.Errorf("target request id: %w", domain.ErrRequiredField)
	}
	if in.At.IsZero() {
		return fmt.Errorf("transfer time: %w", domain.ErrRequiredField)
	}
	if err := validateAuditMeta(in.Audit); err != nil {
		return err
	}

	return uc.uow.WithinTransaction(ctx, func(ctx context.Context, tx ports.Transaction) error {
		// ── Phase 1: acquire all locks and run all guards (nothing saved yet) ────

		// Lock order step 1: target request
		targetReq, err := tx.Requests().GetForUpdate(ctx, in.TargetRequestID)
		if err != nil {
			return fmt.Errorf("load target request %s: %w", in.TargetRequestID, err)
		}
		if targetReq == nil {
			return fmt.Errorf("target request %s: %w", in.TargetRequestID, ports.ErrNotFound)
		}
		if targetReq.Status == domain.RequestStatusCompleted || targetReq.Status == domain.RequestStatusCancelled {
			return fmt.Errorf("target request %s has terminal status %s: %w",
				in.TargetRequestID, targetReq.Status, domain.ErrInvalidState)
		}

		// Lock order step 2: old allocation (consistency guard)
		oldAlloc, err := tx.Allocations().GetForUpdate(ctx, in.OldAllocationID)
		if err != nil {
			return fmt.Errorf("load old allocation %s: %w", in.OldAllocationID, err)
		}
		if oldAlloc == nil {
			return fmt.Errorf("old allocation %s: %w", in.OldAllocationID, ports.ErrNotFound)
		}
		if oldAlloc.RequestID == in.TargetRequestID {
			return fmt.Errorf("source and target request are identical (%s): %w",
				in.TargetRequestID, domain.ErrInvalidState)
		}

		// Lock order step 3: resource (block guard — checked before any saves)
		res, err := tx.Resources().GetForUpdate(ctx, oldAlloc.ResourceID)
		if err != nil {
			return fmt.Errorf("load resource %s: %w", oldAlloc.ResourceID, err)
		}
		if res == nil {
			return fmt.Errorf("resource %s: %w", oldAlloc.ResourceID, ports.ErrNotFound)
		}
		if res.BlockReason != nil {
			return fmt.Errorf("resource %s has active block: %w", res.ID, domain.ErrInvalidState)
		}

		// Detect whether the transfer overtakes a pending return request (for audit note)
		completionNote := in.Audit.Note
		if oldAlloc.ReturnRequestedAt != nil {
			const override = "direct transfer: overrides pending return request"
			if completionNote != "" {
				completionNote = override + "; " + completionNote
			} else {
				completionNote = override
			}
		}

		// ── Phase 2: state changes and saves (index-critical order) ─────────────

		// Step A: complete old allocation → save (RELEASES uq index for this resource)
		oldFromStatus := string(oldAlloc.Status)
		if err := oldAlloc.CompleteDirectTransfer(in.At); err != nil {
			return fmt.Errorf("complete old allocation %s: %w", in.OldAllocationID, err)
		}
		if err := tx.Allocations().Save(ctx, oldAlloc); err != nil {
			return fmt.Errorf("save old allocation %s: %w", in.OldAllocationID, err)
		}

		// Step B: transition resource in_use → reserved
		if err := res.TransferDirect(targetReq.TechnicianID); err != nil {
			return fmt.Errorf("transfer resource %s: %w", res.ID, err)
		}
		if err := tx.Resources().Save(ctx, res); err != nil {
			return fmt.Errorf("save resource %s: %w", res.ID, err)
		}

		// Step C: create new allocation (ACTIVATES uq index; old is now terminal)
		newAlloc, err := domain.NewAllocation(
			in.NewAllocationID, in.TargetRequestID, oldAlloc.ResourceID,
			in.PlannedFrom, in.PlannedUntil, in.At,
		)
		if err != nil {
			return fmt.Errorf("build new allocation: %w", err)
		}
		if err := tx.Allocations().Create(ctx, newAlloc); err != nil {
			return fmt.Errorf("create new allocation %s: %w", in.NewAllocationID, err)
		}

		// ── Phase 3: audit trail ─────────────────────────────────────────────────

		// Audit 1: completion of old allocation (with override note if applicable)
		auditMeta1 := in.Audit
		auditMeta1.Note = completionNote
		event1 := newAuditEvent(
			auditMeta1,
			domain.EntityTypeAllocation,
			string(oldAlloc.ID),
			"complete_direct_transfer",
			oldFromStatus,
			string(oldAlloc.Status),
		)
		if err := tx.Audits().Write(ctx, event1); err != nil {
			return fmt.Errorf("write audit for old allocation %s: %w", in.OldAllocationID, err)
		}

		// Audit 2: activation of new allocation
		event2 := newAuditEvent(
			in.Audit,
			domain.EntityTypeAllocation,
			string(newAlloc.ID),
			"direct_transfer_activate",
			"",
			string(newAlloc.Status),
		)
		if err := tx.Audits().Write(ctx, event2); err != nil {
			return fmt.Errorf("write audit for new allocation %s: %w", in.NewAllocationID, err)
		}

		return nil
	})
}
