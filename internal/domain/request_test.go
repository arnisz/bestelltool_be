package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewRequest(t *testing.T) {
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	from := now.Add(time.Hour)
	until := now.Add(2 * time.Hour)
	r, err := NewRequest("req1", "tech1", "ctx", "job", &from, &until, "note", []ResourceClassID{"class1"}, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Status != RequestStatusOpen {
		t.Fatalf("unexpected status: %s", r.Status)
	}
	if r.Version != 1 {
		t.Fatalf("unexpected version: %d", r.Version)
	}
}

func TestNewRequestInvalid(t *testing.T) {
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	_, err := NewRequest("", "tech1", "ctx", "job", nil, nil, "", []ResourceClassID{"class1"}, now)
	if !errors.Is(err, ErrRequiredField) {
		t.Fatalf("expected ErrRequiredField, got %v", err)
	}

	from := now.Add(time.Hour)
	until := now
	_, err = NewRequest("req1", "tech1", "ctx", "job", &from, &until, "", []ResourceClassID{"class1"}, now)
	if !errors.Is(err, ErrInvalidTimeRange) {
		t.Fatalf("expected ErrInvalidTimeRange, got %v", err)
	}
}

func TestRequestAllowedTransitions(t *testing.T) {
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	r, _ := NewRequest("req1", "tech1", "ctx", "job", nil, nil, "", []ResourceClassID{"class1"}, now)

	steps := []struct {
		name   string
		status RequestStatus
		run    func() error
	}{
		{"in_progress", RequestStatusInProgress, func() error { return r.StartProgress(now.Add(time.Minute)) }},
		{"partially_allocated", RequestStatusPartiallyAllocated, func() error { return r.MarkPartiallyAllocated(now.Add(2 * time.Minute)) }},
		{"allocated", RequestStatusAllocated, func() error { return r.MarkAllocated(now.Add(3 * time.Minute)) }},
		{"active", RequestStatusActive, func() error { return r.Activate(now.Add(4 * time.Minute)) }},
		{"completed", RequestStatusCompleted, func() error { return r.Complete(now.Add(5*time.Minute), true) }},
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

func TestRequestInvalidTransitionKeepsState(t *testing.T) {
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	r, _ := NewRequest("req1", "tech1", "ctx", "job", nil, nil, "", []ResourceClassID{"class1"}, now)

	oldStatus := r.Status
	oldVersion := r.Version
	err := r.Activate(now.Add(time.Minute))
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
	if r.Status != oldStatus || r.Version != oldVersion {
		t.Fatalf("request changed on failed transition")
	}
}

func TestRequestCancelRules(t *testing.T) {
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	r, _ := NewRequest("req1", "tech1", "ctx", "job", nil, nil, "", []ResourceClassID{"class1"}, now)
	if err := r.Cancel(now.Add(time.Minute)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Status != RequestStatusCancelled {
		t.Fatalf("expected cancelled")
	}

	r2, _ := NewRequest("req2", "tech1", "ctx", "job", nil, nil, "", []ResourceClassID{"class1"}, now)
	_ = r2.StartProgress(now.Add(time.Minute))
	_ = r2.MarkAllocated(now.Add(2 * time.Minute))
	oldStatus := r2.Status
	oldVersion := r2.Version
	err := r2.Cancel(now.Add(3 * time.Minute))
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
	if r2.Status != oldStatus || r2.Version != oldVersion {
		t.Fatalf("request changed on invalid cancel")
	}
}

func TestRequestCompleteNeedsPrecondition(t *testing.T) {
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	r, _ := NewRequest("req1", "tech1", "ctx", "job", nil, nil, "", []ResourceClassID{"class1"}, now)
	_ = r.StartProgress(now.Add(time.Minute))
	_ = r.MarkAllocated(now.Add(2 * time.Minute))
	_ = r.Activate(now.Add(3 * time.Minute))

	oldStatus := r.Status
	oldVersion := r.Version
	err := r.Complete(now.Add(4*time.Minute), false)
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("expected ErrInvalidState, got %v", err)
	}
	if r.Status != oldStatus || r.Version != oldVersion {
		t.Fatalf("request changed on invalid completion")
	}
}

func TestUpdateExecutionStateRules(t *testing.T) {
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	r, _ := NewRequest("req1", "tech1", "ctx", "job", nil, nil, "", []ResourceClassID{"class1"}, now)

	oldVersion := r.Version
	err := r.UpdateExecutionState(ExecutionStateBlocked, "")
	if !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("expected ErrReasonRequired, got %v", err)
	}
	if r.Version != oldVersion {
		t.Fatalf("version changed on failed execution update")
	}

	if err := r.UpdateExecutionState(ExecutionStatePartiallyBlocked, "missing item"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.ExecutionNote == "" {
		t.Fatalf("expected execution note")
	}

	if err := r.UpdateExecutionState(ExecutionStateExecutable, "ignored"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.ExecutionNote != "" {
		t.Fatalf("note must be cleared for executable")
	}
}
