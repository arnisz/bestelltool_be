package usecases

import (
	"context"
	"fmt"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

// ReactivateResourceInput contains input for resource reactivation.
type ReactivateResourceInput struct {
	ResourceID domain.ResourceID
	Audit      AuditMeta
}

// ReactivateResourceUseCase reactivates a blocked resource.
type ReactivateResourceUseCase struct {
	uow ports.UnitOfWork
}

// NewReactivateResourceUseCase creates a ReactivateResourceUseCase.
func NewReactivateResourceUseCase(uow ports.UnitOfWork) *ReactivateResourceUseCase {
	return &ReactivateResourceUseCase{uow: uow}
}

// Execute runs the transactional resource-reactivation flow.
func (uc *ReactivateResourceUseCase) Execute(ctx context.Context, in ReactivateResourceInput) error {
	if in.ResourceID == "" {
		return fmt.Errorf("resource id: %w", domain.ErrRequiredField)
	}
	if err := validateAuditMeta(in.Audit); err != nil {
		return err
	}

	return uc.uow.WithinTransaction(ctx, func(ctx context.Context, tx ports.Transaction) error {
		repo := tx.Resources()
		res, err := repo.GetForUpdate(ctx, in.ResourceID)
		if err != nil {
			return fmt.Errorf("load resource %s: %w", in.ResourceID, err)
		}
		if res == nil {
			return fmt.Errorf("resource %s missing: %w", in.ResourceID, domain.ErrInvalidState)
		}

		fromStatus := string(res.Status)
		if err := res.Reactivate(); err != nil {
			return fmt.Errorf("reactivate resource %s: %w", in.ResourceID, err)
		}

		if err := repo.Save(ctx, res); err != nil {
			return fmt.Errorf("save resource %s: %w", in.ResourceID, err)
		}

		event := newAuditEvent(
			in.Audit,
			domain.EntityTypeResource,
			string(res.ID),
			"reactivate_resource",
			fromStatus,
			string(res.Status),
		)
		if err := tx.Audits().Write(ctx, event); err != nil {
			return fmt.Errorf("write resource audit %s: %w", in.ResourceID, err)
		}

		return nil
	})
}
