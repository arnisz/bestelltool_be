package sse

import (
	"context"
	"errors"
	"testing"
	"time"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

func TestBrokerPublishRoleFiltering(t *testing.T) {
	b := NewBroker(4)

	dispatcherCh, closeDispatcher := b.Subscribe(ports.Principal{UserID: "disp-1", Role: domain.ActorRoleDispatcher})
	t.Cleanup(closeDispatcher)

	tech1Ch, closeTech1 := b.Subscribe(ports.Principal{UserID: "tech-1", Role: domain.ActorRoleTechnician})
	t.Cleanup(closeTech1)

	tech2Ch, closeTech2 := b.Subscribe(ports.Principal{UserID: "tech-2", Role: domain.ActorRoleTechnician})
	t.Cleanup(closeTech2)

	event1 := ports.Event{Type: ports.EventTypeRequestCreated, RequestID: "req-1", TechnicianID: "tech-1", OccurredAt: time.Now().UTC()}
	event2 := ports.Event{Type: ports.EventTypeRequestCreated, RequestID: "req-2", TechnicianID: "tech-2", OccurredAt: time.Now().UTC()}

	if err := b.Publish(t.Context(), event1); err != nil {
		t.Fatalf("publish event1: %v", err)
	}
	if err := b.Publish(t.Context(), event2); err != nil {
		t.Fatalf("publish event2: %v", err)
	}

	if got := mustReceiveEvent(t, dispatcherCh); got.RequestID != event1.RequestID {
		t.Fatalf("dispatcher first request id = %q, want %q", got.RequestID, event1.RequestID)
	}
	if got := mustReceiveEvent(t, dispatcherCh); got.RequestID != event2.RequestID {
		t.Fatalf("dispatcher second request id = %q, want %q", got.RequestID, event2.RequestID)
	}

	if got := mustReceiveEvent(t, tech1Ch); got.RequestID != event1.RequestID {
		t.Fatalf("tech-1 request id = %q, want %q", got.RequestID, event1.RequestID)
	}
	assertNoEvent(t, tech1Ch)

	if got := mustReceiveEvent(t, tech2Ch); got.RequestID != event2.RequestID {
		t.Fatalf("tech-2 request id = %q, want %q", got.RequestID, event2.RequestID)
	}
	assertNoEvent(t, tech2Ch)
}

func TestBrokerSubscribeUnsubscribe(t *testing.T) {
	b := NewBroker(1)

	ch, unsubscribe := b.Subscribe(ports.Principal{UserID: "tech-1", Role: domain.ActorRoleTechnician})
	if got := len(b.subscribers); got != 1 {
		t.Fatalf("subscriber count = %d, want 1", got)
	}

	unsubscribe()
	unsubscribe()

	if got := len(b.subscribers); got != 0 {
		t.Fatalf("subscriber count after unsubscribe = %d, want 0", got)
	}

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("channel must be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for closed channel")
	}
}

func TestBrokerPublishRejectsMissingTechnicianID(t *testing.T) {
	b := NewBroker(1)
	err := b.Publish(context.Background(), ports.Event{Type: ports.EventTypeRequestCreated})
	if !errors.Is(err, domain.ErrRequiredField) {
		t.Fatalf("errors.Is(err, ErrRequiredField) = false, err = %v", err)
	}
}

func mustReceiveEvent(t *testing.T, ch <-chan ports.Event) ports.Event {
	t.Helper()

	select {
	case event := <-ch:
		return event
	case <-time.After(250 * time.Millisecond):
		t.Fatal("timed out waiting for event")
		return ports.Event{}
	}
}

func assertNoEvent(t *testing.T, ch <-chan ports.Event) {
	t.Helper()

	select {
	case event := <-ch:
		t.Fatalf("unexpected event received: %+v", event)
	case <-time.After(50 * time.Millisecond):
	}
}
