package usecases

import (
	"context"
	"fmt"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

// UpdateExecutionStateInput contains input for request execution-state updates.
type UpdateExecutionStateInput struct {
	RequestID domain.RequestID
	State     domain.ExecutionState
	Note      string
	Audit     AuditMeta
}

// UpdateExecutionStateUseCase updates the execution state of a request.
type UpdateExecutionStateUseCase struct {
	uow ports.UnitOfWork
}

// NewUpdateExecutionStateUseCase creates an UpdateExecutionStateUseCase.
func NewUpdateExecutionStateUseCase(uow ports.UnitOfWork) *UpdateExecutionStateUseCase {
	return &UpdateExecutionStateUseCase{uow: uow}
}

// Execute runs the transactional update-execution-state flow.
func (uc *UpdateExecutionStateUseCase) Execute(ctx context.Context, in UpdateExecutionStateInput) error {
	if in.RequestID == "" {
		return fmt.Errorf("request id: %w", domain.ErrRequiredField)
	}
	if err := validateAuditMeta(in.Audit); err != nil {
		return err
	}

	return uc.uow.WithinTransaction(ctx, func(ctx context.Context, tx ports.Transaction) error {
		repo := tx.Requests()
		req, err := repo.GetForUpdate(ctx, in.RequestID)
		if err != nil {
			return fmt.Errorf("load request %s: %w", in.RequestID, err)
		}
		if req == nil {
			return fmt.Errorf("request %s missing: %w", in.RequestID, domain.ErrInvalidState)
		}

		fromState := string(req.ExecutionState)
		if err := req.UpdateExecutionState(in.State, in.Note); err != nil {
			return fmt.Errorf("update execution state for request %s: %w", in.RequestID, err)
		}

		if err := repo.Save(ctx, req); err != nil {
			return fmt.Errorf("save request %s: %w", in.RequestID, err)
		}

		event := newAuditEvent(
			in.Audit,
			domain.EntityTypeRequest,
			string(req.ID),
			"update_execution_state",
			fromState,
			string(req.ExecutionState),
		)
		if err := tx.Audits().Write(ctx, event); err != nil {
			return fmt.Errorf("write request audit %s: %w", in.RequestID, err)
		}

		return nil
	})
}
