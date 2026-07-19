package usecases

import (
	"context"
	"fmt"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

// GetRequestUseCase loads one request by id.
type GetRequestUseCase struct {
	requests ports.RequestRepository
}

// NewGetRequestUseCase creates a GetRequestUseCase.
func NewGetRequestUseCase(requests ports.RequestRepository) *GetRequestUseCase {
	return &GetRequestUseCase{requests: requests}
}

// Execute loads and returns the request for the provided id.
func (uc *GetRequestUseCase) Execute(ctx context.Context, requestID domain.RequestID) (*domain.Request, error) {
	if requestID == "" {
		return nil, fmt.Errorf("request id: %w", domain.ErrRequiredField)
	}

	req, err := uc.requests.GetByID(ctx, requestID)
	if err != nil {
		return nil, fmt.Errorf("load request %s: %w", requestID, err)
	}
	if req == nil {
		return nil, fmt.Errorf("request %s: %w", requestID, ports.ErrNotFound)
	}

	return req, nil
}
