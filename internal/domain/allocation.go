package domain

import (
	"fmt"
	"time"
)

// Allocation represents assignment of a resource to a request.
type Allocation struct {
	ID                AllocationID
	RequestID         RequestID
	ResourceID        ResourceID
	Status            AllocationStatus
	PlannedFrom       time.Time
	PlannedUntil      time.Time
	ReturnRequestedAt *time.Time
	ShippedAt         *time.Time
	ReceivedAt        *time.Time
	Version           int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// NewAllocation creates an allocation in a valid initial state.
func NewAllocation(
	id AllocationID,
	requestID RequestID,
	resourceID ResourceID,
	plannedFrom time.Time,
	plannedUntil time.Time,
	createdAt time.Time,
) (*Allocation, error) {
	if id == "" {
		return nil, fmt.Errorf("allocation id: %w", ErrRequiredField)
	}
	if requestID == "" {
		return nil, fmt.Errorf("request id: %w", ErrRequiredField)
	}
	if resourceID == "" {
		return nil, fmt.Errorf("resource id: %w", ErrRequiredField)
	}
	if !plannedUntil.After(plannedFrom) {
		return nil, fmt.Errorf("planned time range: %w", ErrInvalidTimeRange)
	}

	return &Allocation{
		ID:           id,
		RequestID:    requestID,
		ResourceID:   resourceID,
		Status:       AllocationStatusAllocated,
		PlannedFrom:  plannedFrom,
		PlannedUntil: plannedUntil,
		Version:      1,
		CreatedAt:    createdAt,
		UpdatedAt:    createdAt,
	}, nil
}

// Reschedule updates the planning window without changing lifecycle status.
func (a *Allocation) Reschedule(from, until time.Time, updatedAt time.Time) error {
	if a.Status == AllocationStatusCompleted || a.Status == AllocationStatusCancelled {
		return fmt.Errorf("reschedule allocation in %s: %w", a.Status, ErrInvalidTransition)
	}
	if !until.After(from) {
		return fmt.Errorf("planned time range: %w", ErrInvalidTimeRange)
	}
	if updatedAt.Before(a.UpdatedAt) {
		return fmt.Errorf("updated at moved backwards: %w", ErrInvalidTimeRange)
	}

	a.PlannedFrom = from
	a.PlannedUntil = until
	a.UpdatedAt = updatedAt
	a.Version++
	return nil
}

// MarkShipped transitions allocation from allocated to shipped.
func (a *Allocation) MarkShipped(at time.Time) error {
	if a.Status != AllocationStatusAllocated {
		return fmt.Errorf("transition allocation from %s to %s: %w", a.Status, AllocationStatusShipped, ErrInvalidTransition)
	}
	if at.Before(a.UpdatedAt) {
		return fmt.Errorf("shipped time moved backwards: %w", ErrInvalidTimeRange)
	}

	a.Status = AllocationStatusShipped
	a.ShippedAt = &at
	a.UpdatedAt = at
	a.Version++
	return nil
}

// MarkReceivedByTechnician transitions allocation from shipped to with_technician.
func (a *Allocation) MarkReceivedByTechnician(at time.Time) error {
	if a.Status != AllocationStatusShipped {
		return fmt.Errorf("transition allocation from %s to %s: %w", a.Status, AllocationStatusWithTechnician, ErrInvalidTransition)
	}
	if a.ShippedAt != nil && at.Before(*a.ShippedAt) {
		return fmt.Errorf("received time moved backwards: %w", ErrInvalidTimeRange)
	}

	a.Status = AllocationStatusWithTechnician
	a.ReceivedAt = &at
	a.UpdatedAt = at
	a.Version++
	return nil
}

// RequestReturn transitions allocation from with_technician to return_requested.
func (a *Allocation) RequestReturn(at time.Time) error {
	if a.Status != AllocationStatusWithTechnician {
		return fmt.Errorf("transition allocation from %s to %s: %w", a.Status, AllocationStatusReturnRequested, ErrInvalidTransition)
	}
	if a.ReceivedAt != nil && at.Before(*a.ReceivedAt) {
		return fmt.Errorf("return request time moved backwards: %w", ErrInvalidTimeRange)
	}

	a.Status = AllocationStatusReturnRequested
	a.ReturnRequestedAt = &at
	a.UpdatedAt = at
	a.Version++
	return nil
}

// MarkShippedBack transitions allocation from return_requested to shipped_back.
func (a *Allocation) MarkShippedBack(at time.Time) error {
	if a.Status != AllocationStatusReturnRequested {
		return fmt.Errorf("transition allocation from %s to %s: %w", a.Status, AllocationStatusShippedBack, ErrInvalidTransition)
	}
	if a.ReturnRequestedAt != nil && at.Before(*a.ReturnRequestedAt) {
		return fmt.Errorf("shipped back time moved backwards: %w", ErrInvalidTimeRange)
	}

	a.Status = AllocationStatusShippedBack
	a.UpdatedAt = at
	a.Version++
	return nil
}

// StartInspection transitions allocation from shipped_back to inspection.
func (a *Allocation) StartInspection(at time.Time) error {
	if a.Status != AllocationStatusShippedBack {
		return fmt.Errorf("transition allocation from %s to %s: %w", a.Status, AllocationStatusInspection, ErrInvalidTransition)
	}
	if at.Before(a.UpdatedAt) {
		return fmt.Errorf("inspection start time moved backwards: %w", ErrInvalidTimeRange)
	}

	a.Status = AllocationStatusInspection
	a.UpdatedAt = at
	a.Version++
	return nil
}

// CompleteInspection transitions allocation from inspection to completed.
func (a *Allocation) CompleteInspection(at time.Time) error {
	if a.Status == AllocationStatusCompleted {
		return fmt.Errorf("allocation already completed: %w", ErrAlreadyCompleted)
	}
	if a.Status != AllocationStatusInspection {
		return fmt.Errorf("transition allocation from %s to %s: %w", a.Status, AllocationStatusCompleted, ErrInvalidTransition)
	}
	if at.Before(a.UpdatedAt) {
		return fmt.Errorf("inspection completion time moved backwards: %w", ErrInvalidTimeRange)
	}

	a.Status = AllocationStatusCompleted
	a.UpdatedAt = at
	a.Version++
	return nil
}

// Cancel transitions allocation from allocated to cancelled.
func (a *Allocation) Cancel(at time.Time) error {
	if a.Status == AllocationStatusCompleted {
		return fmt.Errorf("allocation already completed: %w", ErrAlreadyCompleted)
	}
	if a.Status != AllocationStatusAllocated {
		return fmt.Errorf("transition allocation from %s to %s: %w", a.Status, AllocationStatusCancelled, ErrInvalidTransition)
	}
	if at.Before(a.UpdatedAt) {
		return fmt.Errorf("cancel time moved backwards: %w", ErrInvalidTimeRange)
	}

	a.Status = AllocationStatusCancelled
	a.UpdatedAt = at
	a.Version++
	return nil
}

// CompleteDirectTransfer transitions allocation from with_technician to completed
// for a direct site-to-site transfer. A pending ReturnRequestedAt does not block
// the transfer — the direct transfer overtakes a pending return request.
func (a *Allocation) CompleteDirectTransfer(at time.Time) error {
	if a.Status == AllocationStatusCompleted {
		return fmt.Errorf("allocation already completed: %w", ErrAlreadyCompleted)
	}
	if a.Status != AllocationStatusWithTechnician {
		return fmt.Errorf("transition allocation from %s to %s: %w", a.Status, AllocationStatusCompleted, ErrInvalidTransition)
	}
	if at.Before(a.UpdatedAt) {
		return fmt.Errorf("completion time moved backwards: %w", ErrInvalidTimeRange)
	}

	a.Status = AllocationStatusCompleted
	a.UpdatedAt = at
	a.Version++
	return nil
}
