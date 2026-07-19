package sse

import (
	"context"
	"fmt"
	"sync"

	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/domain"
)

const (
	defaultSubscriberBuffer = 16
)

type subscriber struct {
	principal ports.Principal
	ch        chan ports.Event
}

// Broker fans out published events to connected SSE clients.
//
// Publish is intentionally best-effort and non-blocking for callers. Slow
// subscribers drop events when their buffer is full.
type Broker struct {
	mu               sync.RWMutex
	nextSubscriberID uint64
	subscribers      map[uint64]subscriber
	subscriberBuffer int
}

// NewBroker creates an in-memory SSE event broker.
func NewBroker(subscriberBuffer int) *Broker {
	if subscriberBuffer <= 0 {
		subscriberBuffer = defaultSubscriberBuffer
	}

	return &Broker{
		subscribers:      make(map[uint64]subscriber),
		subscriberBuffer: subscriberBuffer,
	}
}

// Publish sends an event to all matching subscribers (dispatcher: all,
// technician: own events only).
func (b *Broker) Publish(ctx context.Context, event ports.Event) error {
	if event.TechnicianID == "" {
		return fmt.Errorf("event technician id: %w", domain.ErrRequiredField)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, sub := range b.subscribers {
		if !canReceive(sub.principal, event) {
			continue
		}

		select {
		case sub.ch <- event:
		default:
		}
	}

	return nil
}

// Subscribe registers a client and returns its stream plus a mandatory cleanup
// callback that unsubscribes and closes the client channel.
func (b *Broker) Subscribe(principal ports.Principal) (<-chan ports.Event, func()) {
	b.mu.Lock()
	id := b.nextSubscriberID
	b.nextSubscriberID++

	ch := make(chan ports.Event, b.subscriberBuffer)
	b.subscribers[id] = subscriber{principal: principal, ch: ch}
	b.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			b.mu.Lock()
			sub, ok := b.subscribers[id]
			if ok {
				delete(b.subscribers, id)
			}
			b.mu.Unlock()

			if ok {
				close(sub.ch)
			}
		})
	}

	return ch, unsubscribe
}

func canReceive(principal ports.Principal, event ports.Event) bool {
	switch principal.Role {
	case domain.ActorRoleDispatcher:
		return true
	case domain.ActorRoleTechnician:
		return principal.UserID == event.TechnicianID
	default:
		return false
	}
}
