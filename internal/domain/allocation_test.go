package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewAllocation(t *testing.T) {
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	from := now.Add(time.Hour)
	until := now.Add(2 * time.Hour)

	a, err := NewAllocation("a1", "r1", "res1", from, until, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Status != AllocationStatusAllocated {
		t.Fatalf("unexpected status: %s", a.Status)
	}
	if a.Version != 1 {
		t.Fatalf("unexpected version: %d", a.Version)
	}
}

func TestNewAllocationInvalid(t *testing.T) {
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	_, err := NewAllocation("", "r1", "res1", now, now.Add(time.Hour), now)
	if !errors.Is(err, ErrRequiredField) {
		t.Fatalf("expected ErrRequiredField, got %v", err)
	}

	_, err = NewAllocation("a1", "r1", "res1", now, now, now)
	if !errors.Is(err, ErrInvalidTimeRange) {
		t.Fatalf("expected ErrInvalidTimeRange, got %v", err)
	}
}

func TestAllocationAllowedTransitions(t *testing.T) {
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	a, err := NewAllocation("a1", "r1", "res1", now.Add(time.Hour), now.Add(2*time.Hour), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	steps := []struct {
		name   string
		status AllocationStatus
		run    func() error
	}{
		{"ship", AllocationStatusShipped, func() error { return a.MarkShipped(now.Add(time.Minute)) }},
		{"receive", AllocationStatusWithTechnician, func() error { return a.MarkReceivedByTechnician(now.Add(2 * time.Minute)) }},
		{"request_return", AllocationStatusReturnRequested, func() error { return a.RequestReturn(now.Add(3 * time.Minute)) }},
		{"ship_back", AllocationStatusShippedBack, func() error { return a.MarkShippedBack(now.Add(4 * time.Minute)) }},
		{"start_inspection", AllocationStatusInspection, func() error { return a.StartInspection(now.Add(5 * time.Minute)) }},
		{"complete", AllocationStatusCompleted, func() error { return a.CompleteInspection(now.Add(6 * time.Minute)) }},
	}

	prevVersion := a.Version
	for _, tt := range steps {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if a.Status != tt.status {
				t.Fatalf("expected status %s, got %s", tt.status, a.Status)
			}
			if a.Version != prevVersion+1 {
				t.Fatalf("expected version %d, got %d", prevVersion+1, a.Version)
			}
			prevVersion = a.Version
		})
	}
}

func TestAllocationInvalidTransitionKeepsState(t *testing.T) {
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	a, err := NewAllocation("a1", "r1", "res1", now.Add(time.Hour), now.Add(2*time.Hour), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	oldStatus := a.Status
	oldVersion := a.Version
	err = a.CompleteInspection(now.Add(time.Minute))
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
	if a.Status != oldStatus {
		t.Fatalf("status changed on failed transition")
	}
	if a.Version != oldVersion {
		t.Fatalf("version changed on failed transition")
	}
}

func TestAllocationReschedule(t *testing.T) {
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	a, _ := NewAllocation("a1", "r1", "res1", now.Add(time.Hour), now.Add(2*time.Hour), now)

	statusBefore := a.Status
	versionBefore := a.Version
	if err := a.Reschedule(now.Add(2*time.Hour), now.Add(3*time.Hour), now.Add(time.Minute)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Status != statusBefore {
		t.Fatalf("status must not change during reschedule")
	}
	if a.Version != versionBefore+1 {
		t.Fatalf("expected version increase")
	}

	oldVersion := a.Version
	err := a.Reschedule(now.Add(4*time.Hour), now.Add(3*time.Hour), now.Add(2*time.Minute))
	if !errors.Is(err, ErrInvalidTimeRange) {
		t.Fatalf("expected ErrInvalidTimeRange, got %v", err)
	}
	if a.Version != oldVersion {
		t.Fatalf("version changed on failed reschedule")
	}
}

func TestAllocationCancelOnlyBeforeShipment(t *testing.T) {
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	a, _ := NewAllocation("a1", "r1", "res1", now.Add(time.Hour), now.Add(2*time.Hour), now)

	if err := a.Cancel(now.Add(time.Minute)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Status != AllocationStatusCancelled {
		t.Fatalf("expected cancelled")
	}

	b, _ := NewAllocation("a2", "r1", "res1", now.Add(time.Hour), now.Add(2*time.Hour), now)
	if err := b.MarkShipped(now.Add(time.Minute)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	oldStatus := b.Status
	oldVersion := b.Version
	err := b.Cancel(now.Add(2 * time.Minute))
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
	if b.Status != oldStatus || b.Version != oldVersion {
		t.Fatalf("allocation changed on invalid cancel")
	}
}
