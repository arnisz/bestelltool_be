package domain

import (
	"errors"
	"testing"
)

func TestNewResource(t *testing.T) {
	r, err := NewResource("res1", "class1", "SN-1", "Dispatcher", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Status != ResourceStatusAvailable {
		t.Fatalf("unexpected status: %s", r.Status)
	}
	if r.Version != 1 {
		t.Fatalf("unexpected version: %d", r.Version)
	}
}

func TestNewResourceInvalid(t *testing.T) {
	_, err := NewResource("", "class1", "SN-1", "Dispatcher", nil, nil)
	if !errors.Is(err, ErrRequiredField) {
		t.Fatalf("expected ErrRequiredField, got %v", err)
	}
}

func TestResourceAllowedTransitions(t *testing.T) {
	r, _ := NewResource("res1", "class1", "SN-1", "Dispatcher", nil, nil)

	steps := []struct {
		name   string
		status ResourceStatus
		run    func() error
	}{
		{"reserve", ResourceStatusReserved, func() error { return r.Reserve("u1") }},
		{"issue", ResourceStatusIssued, func() error { return r.MarkIssued() }},
		{"in_use", ResourceStatusInUse, func() error { return r.MarkInUse() }},
		{"shipped_back", ResourceStatusShippedBack, func() error { return r.MarkShippedBack() }},
		{"inspection", ResourceStatusInspection, func() error { return r.StartInspection() }},
		{"available", ResourceStatusAvailable, func() error { return r.CompleteInspectionAvailable() }},
	}

	prevVersion := r.Version
	for _, tt := range steps {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if r.Status != tt.status {
				t.Fatalf("expected %s, got %s", tt.status, r.Status)
			}
			if r.Version != prevVersion+1 {
				t.Fatalf("expected version increase")
			}
			prevVersion = r.Version
		})
	}
}

func TestResourceInspectionCanBlockAndReactivate(t *testing.T) {
	r, _ := NewResource("res1", "class1", "SN-1", "Dispatcher", nil, nil)
	_ = r.Reserve("u1")
	_ = r.MarkIssued()
	_ = r.MarkInUse()
	_ = r.MarkShippedBack()
	_ = r.StartInspection()

	if err := r.CompleteInspectionBlocked(BlockReasonDefective, "damage"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Status != ResourceStatusBlocked || r.BlockReason == nil {
		t.Fatalf("expected blocked with reason")
	}

	if err := r.Reactivate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Status != ResourceStatusAvailable || r.BlockReason != nil || r.BlockNote != "" {
		t.Fatalf("expected available without block info")
	}
}

func TestResourceInvalidTransitionKeepsState(t *testing.T) {
	r, _ := NewResource("res1", "class1", "SN-1", "Dispatcher", nil, nil)
	oldStatus := r.Status
	oldVersion := r.Version
	err := r.MarkIssued()
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
	if r.Status != oldStatus || r.Version != oldVersion {
		t.Fatalf("resource changed on failed transition")
	}
}

func TestResourceBlockedNeedsReason(t *testing.T) {
	r, _ := NewResource("res1", "class1", "SN-1", "Dispatcher", nil, nil)
	_ = r.Reserve("u1")
	_ = r.MarkIssued()
	_ = r.MarkInUse()
	_ = r.MarkShippedBack()
	_ = r.StartInspection()

	oldStatus := r.Status
	oldVersion := r.Version
	err := r.CompleteInspectionBlocked("", "")
	if !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("expected ErrReasonRequired, got %v", err)
	}
	if r.Status != oldStatus || r.Version != oldVersion {
		t.Fatalf("resource changed on invalid block")
	}
}

func TestResourceNoAutomaticAvailabilityAfterShippedBack(t *testing.T) {
	r, _ := NewResource("res1", "class1", "SN-1", "Dispatcher", nil, nil)
	_ = r.Reserve("u1")
	_ = r.MarkIssued()
	_ = r.MarkInUse()
	if err := r.MarkShippedBack(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Status == ResourceStatusAvailable {
		t.Fatalf("resource must not become available automatically")
	}
}

func TestResourceTransferDirectSuccess(t *testing.T) {
	r, _ := NewResource("res1", "class1", "SN-1", "Dispatcher", nil, nil)
	_ = r.Reserve("u1")
	_ = r.MarkIssued()
	_ = r.MarkInUse()

	prevVersion := r.Version
	if err := r.TransferDirect("u2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Status != ResourceStatusReserved {
		t.Fatalf("expected reserved, got %s", r.Status)
	}
	if r.HolderID == nil || *r.HolderID != "u2" {
		t.Fatalf("expected holder u2, got %v", r.HolderID)
	}
	if r.Version != prevVersion+1 {
		t.Fatalf("expected version %d, got %d", prevVersion+1, r.Version)
	}
}

func TestResourceTransferDirectBlockedResource(t *testing.T) {
	r, _ := NewResource("res1", "class1", "SN-1", "Dispatcher", nil, nil)
	_ = r.Reserve("u1")
	_ = r.MarkIssued()
	_ = r.MarkInUse()
	// Manually inject block reason (defense-in-depth guard)
	reason := BlockReasonDefective
	r.BlockReason = &reason

	oldStatus := r.Status
	oldVersion := r.Version
	err := r.TransferDirect("u2")
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("expected ErrInvalidState, got %v", err)
	}
	if r.Status != oldStatus || r.Version != oldVersion {
		t.Fatalf("resource changed on blocked transfer")
	}
}

func TestResourceTransferDirectEmptyHolderID(t *testing.T) {
	r, _ := NewResource("res1", "class1", "SN-1", "Dispatcher", nil, nil)
	_ = r.Reserve("u1")
	_ = r.MarkIssued()
	_ = r.MarkInUse()

	oldStatus := r.Status
	oldVersion := r.Version
	err := r.TransferDirect("")
	if !errors.Is(err, ErrRequiredField) {
		t.Fatalf("expected ErrRequiredField, got %v", err)
	}
	if r.Status != oldStatus || r.Version != oldVersion {
		t.Fatalf("resource changed on empty holder id")
	}
}

func TestResourceTransferDirectFromShippedBack(t *testing.T) {
	r, _ := NewResource("res1", "class1", "SN-1", "Dispatcher", nil, nil)
	_ = r.Reserve("u1")
	_ = r.MarkIssued()
	_ = r.MarkInUse()
	_ = r.MarkShippedBack()

	oldStatus := r.Status
	oldVersion := r.Version
	err := r.TransferDirect("u2")
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
	if r.Status != oldStatus || r.Version != oldVersion {
		t.Fatalf("resource changed on invalid transfer from shipped_back")
	}
}
