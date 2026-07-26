package usecases

import (
	"context"
	"fmt"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

type GetMeInput struct {
	UserID domain.UserID
}

type GetMeOutput struct {
	Roles []domain.ActorRole
}

type GetMeUseCase struct {
	uow ports.UnitOfWork
}

func NewGetMeUseCase(uow ports.UnitOfWork) *GetMeUseCase {
	return &GetMeUseCase{uow: uow}
}

func (uc *GetMeUseCase) Execute(ctx context.Context, in GetMeInput) (*GetMeOutput, error) {
	var out *GetMeOutput
	err := uc.uow.WithinTransaction(ctx, func(ctx context.Context, tx ports.Transaction) error {
		roles, err := tx.UserRoles().RolesForUser(ctx, in.UserID)
		if err != nil {
			return fmt.Errorf("load user roles: %w", err)
		}
		out = &GetMeOutput{Roles: roles}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
