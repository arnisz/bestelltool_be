package domain

import (
	"fmt"
	"time"
)

// Request represents a technician request.
type Request struct {
	ID                       RequestID
	TechnicianID             UserID
	Status                   RequestStatus
	ExecutionState           ExecutionState
	ExecutionNote            string
	ContextRef               string
	ContextLabel             string
	WishFrom                 *time.Time
	WishUntil                *time.Time
	Note                     string
	RequestedResourceClasses []ResourceClassID
	Version                  int64
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// NewRequest creates a request in a valid initial state.
func NewRequest(
	id RequestID,
	technicianID UserID,
	contextRef string,
	contextLabel string,
	wishFrom *time.Time,
	wishUntil *time.Time,
	note string,
	requestedResourceClasses []ResourceClassID,
	createdAt time.Time,
) (*Request, error) {
	if id == "" {
		return nil, fmt.Errorf("request id: %w", ErrRequiredField)
	}
	if technicianID == "" {
		return nil, fmt.Errorf("technician id: %w", ErrRequiredField)
	}
	if len(requestedResourceClasses) == 0 {
		return nil, fmt.Errorf("requested resource classes: %w", ErrRequiredField)
	}
	if wishFrom != nil && wishUntil != nil && !wishUntil.After(*wishFrom) {
		return nil, fmt.Errorf("wish time range: %w", ErrInvalidTimeRange)
	}

	return &Request{
		ID:                       id,
		TechnicianID:             technicianID,
		Status:                   RequestStatusOpen,
		ExecutionState:           ExecutionStateExecutable,
		ContextRef:               contextRef,
		ContextLabel:             contextLabel,
		WishFrom:                 wishFrom,
		WishUntil:                wishUntil,
		Note:                     note,
		RequestedResourceClasses: requestedResourceClasses,
		Version:                  1,
		CreatedAt:                createdAt,
		UpdatedAt:                createdAt,
	}, nil
}

// StartProgress transitions request from open to in_progress.
func (r *Request) StartProgress(updatedAt time.Time) error {
	if r.Status != RequestStatusOpen {
		return fmt.Errorf("transition request from %s to %s: %w", r.Status, RequestStatusInProgress, ErrInvalidTransition)
	}
	if updatedAt.Before(r.UpdatedAt) {
		return fmt.Errorf("updated at moved backwards: %w", ErrInvalidTimeRange)
	}

	r.Status = RequestStatusInProgress
	r.UpdatedAt = updatedAt
	r.Version++
	return nil
}

// MarkPartiallyAllocated transitions request from in_progress to partially_allocated.
func (r *Request) MarkPartiallyAllocated(updatedAt time.Time) error {
	if r.Status != RequestStatusInProgress {
		return fmt.Errorf("transition request from %s to %s: %w", r.Status, RequestStatusPartiallyAllocated, ErrInvalidTransition)
	}
	if updatedAt.Before(r.UpdatedAt) {
		return fmt.Errorf("updated at moved backwards: %w", ErrInvalidTimeRange)
	}

	r.Status = RequestStatusPartiallyAllocated
	r.UpdatedAt = updatedAt
	r.Version++
	return nil
}

// MarkAllocated transitions request to allocated.
func (r *Request) MarkAllocated(updatedAt time.Time) error {
	if r.Status != RequestStatusInProgress && r.Status != RequestStatusPartiallyAllocated {
		return fmt.Errorf("transition request from %s to %s: %w", r.Status, RequestStatusAllocated, ErrInvalidTransition)
	}
	if updatedAt.Before(r.UpdatedAt) {
		return fmt.Errorf("updated at moved backwards: %w", ErrInvalidTimeRange)
	}

	r.Status = RequestStatusAllocated
	r.UpdatedAt = updatedAt
	r.Version++
	return nil
}

// Activate transitions request from allocated to active.
func (r *Request) Activate(updatedAt time.Time) error {
	if r.Status != RequestStatusAllocated {
		return fmt.Errorf("transition request from %s to %s: %w", r.Status, RequestStatusActive, ErrInvalidTransition)
	}
	if updatedAt.Before(r.UpdatedAt) {
		return fmt.Errorf("updated at moved backwards: %w", ErrInvalidTimeRange)
	}

	r.Status = RequestStatusActive
	r.UpdatedAt = updatedAt
	r.Version++
	return nil
}

// Complete transitions request from active to completed.
func (r *Request) Complete(updatedAt time.Time, allAllocationsCompleted bool) error {
	if r.Status == RequestStatusCompleted {
		return fmt.Errorf("request already completed: %w", ErrAlreadyCompleted)
	}
	if r.Status != RequestStatusActive {
		return fmt.Errorf("transition request from %s to %s: %w", r.Status, RequestStatusCompleted, ErrInvalidTransition)
	}
	if !allAllocationsCompleted {
		return fmt.Errorf("complete request precondition failed: %w", ErrInvalidState)
	}
	if updatedAt.Before(r.UpdatedAt) {
		return fmt.Errorf("updated at moved backwards: %w", ErrInvalidTimeRange)
	}

	r.Status = RequestStatusCompleted
	r.UpdatedAt = updatedAt
	r.Version++
	return nil
}

// Cancel transitions request to cancelled from open or in_progress.
func (r *Request) Cancel(updatedAt time.Time) error {
	if r.Status == RequestStatusCompleted {
		return fmt.Errorf("request already completed: %w", ErrAlreadyCompleted)
	}
	if r.Status != RequestStatusOpen && r.Status != RequestStatusInProgress {
		return fmt.Errorf("transition request from %s to %s: %w", r.Status, RequestStatusCancelled, ErrInvalidTransition)
	}
	if updatedAt.Before(r.UpdatedAt) {
		return fmt.Errorf("updated at moved backwards: %w", ErrInvalidTimeRange)
	}

	r.Status = RequestStatusCancelled
	r.UpdatedAt = updatedAt
	r.Version++
	return nil
}

// UpdateExecutionState updates execution state with domain validation.
func (r *Request) UpdateExecutionState(state ExecutionState, note string) error {
	if !isValidExecutionState(state) {
		return fmt.Errorf("execution state %q: %w", state, ErrInvalidState)
	}

	if (state == ExecutionStatePartiallyBlocked || state == ExecutionStateBlocked) && note == "" {
		return fmt.Errorf("execution state %s: %w", state, ErrReasonRequired)
	}

	r.ExecutionState = state
	if state == ExecutionStateExecutable {
		r.ExecutionNote = ""
	} else {
		r.ExecutionNote = note
	}
	r.Version++
	return nil
}

func isValidExecutionState(state ExecutionState) bool {
	switch state {
	case ExecutionStateExecutable, ExecutionStatePartiallyBlocked, ExecutionStateBlocked:
		return true
	default:
		return false
	}
}
