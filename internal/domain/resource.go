package domain

import (
	"fmt"
	"time"
)

// Resource holds lifecycle and assignment state for a concrete resource.
type Resource struct {
	ID              ResourceID
	ResourceClassID ResourceClassID
	SerialNumber    string
	Status          ResourceStatus
	BlockReason     *BlockReason
	BlockNote       string
	HolderID        *UserID
	Location        string
	ValidUntil      *time.Time
	Metadata        map[string]any
	Version         int64
}

// NewResource creates a resource in a valid initial state.
func NewResource(
	id ResourceID,
	resourceClassID ResourceClassID,
	serialNumber string,
	location string,
	validUntil *time.Time,
	metadata map[string]any,
) (*Resource, error) {
	if id == "" {
		return nil, fmt.Errorf("resource id: %w", ErrRequiredField)
	}
	if resourceClassID == "" {
		return nil, fmt.Errorf("resource class id: %w", ErrRequiredField)
	}
	if serialNumber == "" {
		return nil, fmt.Errorf("resource serial number: %w", ErrRequiredField)
	}

	return &Resource{
		ID:              id,
		ResourceClassID: resourceClassID,
		SerialNumber:    serialNumber,
		Status:          ResourceStatusAvailable,
		Location:        location,
		ValidUntil:      validUntil,
		Metadata:        metadata,
		Version:         1,
	}, nil
}

// Reserve transitions resource from available to reserved.
func (r *Resource) Reserve(holderID UserID) error {
	if r.Status != ResourceStatusAvailable {
		return fmt.Errorf("transition resource from %s to %s: %w", r.Status, ResourceStatusReserved, ErrInvalidTransition)
	}
	if holderID == "" {
		return fmt.Errorf("reserve holder id: %w", ErrRequiredField)
	}

	r.Status = ResourceStatusReserved
	r.HolderID = &holderID
	r.Version++
	return nil
}

// MarkIssued transitions resource from reserved to issued.
func (r *Resource) MarkIssued() error {
	if r.Status != ResourceStatusReserved {
		return fmt.Errorf("transition resource from %s to %s: %w", r.Status, ResourceStatusIssued, ErrInvalidTransition)
	}

	r.Status = ResourceStatusIssued
	r.Version++
	return nil
}

// MarkInUse transitions resource from issued to in_use.
func (r *Resource) MarkInUse() error {
	if r.Status != ResourceStatusIssued {
		return fmt.Errorf("transition resource from %s to %s: %w", r.Status, ResourceStatusInUse, ErrInvalidTransition)
	}

	r.Status = ResourceStatusInUse
	r.Version++
	return nil
}

// MarkShippedBack transitions resource from in_use to shipped_back.
func (r *Resource) MarkShippedBack() error {
	if r.Status != ResourceStatusInUse {
		return fmt.Errorf("transition resource from %s to %s: %w", r.Status, ResourceStatusShippedBack, ErrInvalidTransition)
	}

	r.Status = ResourceStatusShippedBack
	r.Version++
	return nil
}

// StartInspection transitions resource from shipped_back to inspection.
func (r *Resource) StartInspection() error {
	if r.Status != ResourceStatusShippedBack {
		return fmt.Errorf("transition resource from %s to %s: %w", r.Status, ResourceStatusInspection, ErrInvalidTransition)
	}

	r.Status = ResourceStatusInspection
	r.Version++
	return nil
}

// CompleteInspectionAvailable transitions resource from inspection to available.
func (r *Resource) CompleteInspectionAvailable() error {
	if r.Status != ResourceStatusInspection {
		return fmt.Errorf("transition resource from %s to %s: %w", r.Status, ResourceStatusAvailable, ErrInvalidTransition)
	}

	r.Status = ResourceStatusAvailable
	r.BlockReason = nil
	r.BlockNote = ""
	r.Version++
	return nil
}

// CompleteInspectionBlocked transitions resource from inspection to blocked.
func (r *Resource) CompleteInspectionBlocked(reason BlockReason, note string) error {
	if r.Status != ResourceStatusInspection {
		return fmt.Errorf("transition resource from %s to %s: %w", r.Status, ResourceStatusBlocked, ErrInvalidTransition)
	}
	if !isValidBlockReason(reason) {
		return fmt.Errorf("invalid block reason %q: %w", reason, ErrReasonRequired)
	}

	r.Status = ResourceStatusBlocked
	r.BlockReason = &reason
	r.BlockNote = note
	r.Version++
	return nil
}

// Reactivate transitions resource from blocked to available.
func (r *Resource) Reactivate() error {
	if r.Status != ResourceStatusBlocked {
		return fmt.Errorf("transition resource from %s to %s: %w", r.Status, ResourceStatusAvailable, ErrInvalidTransition)
	}

	r.Status = ResourceStatusAvailable
	r.BlockReason = nil
	r.BlockNote = ""
	r.Version++
	return nil
}

// MarkExternallyProcured transitions resource from available to externally_procured.
func (r *Resource) MarkExternallyProcured() error {
	if r.Status != ResourceStatusAvailable {
		return fmt.Errorf("transition resource from %s to %s: %w", r.Status, ResourceStatusExternallyProcured, ErrInvalidTransition)
	}

	r.Status = ResourceStatusExternallyProcured
	r.Version++
	return nil
}

func isValidBlockReason(reason BlockReason) bool {
	switch reason {
	case BlockReasonDefective, BlockReasonMaintenance, BlockReasonInspectionDue:
		return true
	default:
		return false
	}
}
