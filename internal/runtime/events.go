package runtime

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrSubscriberBackpressure = errors.New("runtime: subscriber backpressure")

type EventType string

const (
	EventDocumentChanged      EventType = "document_changed"
	EventDraftCreated         EventType = "draft_created"
	EventDraftStateChanged    EventType = "draft_state_changed"
	EventCheckpointRequested  EventType = "checkpoint_requested"
	EventCheckpointWritten    EventType = "checkpoint_written"
	EventDailyRollupRequested EventType = "daily_rollup_requested"
	EventDailyRollupWritten   EventType = "daily_rollup_written"
	EventAuditRecorded        EventType = "audit_recorded"
	EventHealthChanged        EventType = "health_changed"
)

type Event struct {
	ID         string    `json:"id"`
	Type       EventType `json:"type"`
	Source     string    `json:"source"`
	OccurredAt time.Time `json:"occurred_at"`
	Payload    any       `json:"payload,omitempty"`
}

type Broker interface {
	Publish(ctx context.Context, event Event) error
	Subscribe(eventType EventType, buffer int) (<-chan Event, func())
}

type InMemoryBroker struct {
	mu          sync.RWMutex
	subscribers map[EventType]map[chan Event]struct{}
}

func NewInMemoryBroker() *InMemoryBroker {
	return &InMemoryBroker{
		subscribers: make(map[EventType]map[chan Event]struct{}),
	}
}

func (b *InMemoryBroker) Publish(ctx context.Context, event Event) error {
	// Hold the read lock across the sends. A subscriber's cancel() closes its
	// channel under the write lock, so the previous version -- which snapshotted
	// the channels, released the lock, then sent -- could send on a channel a
	// concurrent cancel had already closed, panicking with "send on closed
	// channel" (review-v1 P2-7). Sending under RLock makes that impossible
	// (cancel's write lock waits for us). The sends are non-blocking (the
	// default case below), so this cannot stall cancel for long.
	b.mu.RLock()
	defer b.mu.RUnlock()

	for ch := range b.subscribers[event.Type] {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ch <- event:
		default:
			return ErrSubscriberBackpressure
		}
	}
	return nil
}

func (b *InMemoryBroker) Subscribe(eventType EventType, buffer int) (<-chan Event, func()) {
	if buffer <= 0 {
		buffer = 1
	}

	ch := make(chan Event, buffer)
	b.mu.Lock()
	if _, ok := b.subscribers[eventType]; !ok {
		b.subscribers[eventType] = make(map[chan Event]struct{})
	}
	b.subscribers[eventType][ch] = struct{}{}
	b.mu.Unlock()

	cancel := func() {
		b.mu.Lock()
		if subs, ok := b.subscribers[eventType]; ok {
			delete(subs, ch)
		}
		b.mu.Unlock()
		close(ch)
	}

	return ch, cancel
}
