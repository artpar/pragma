package observe

import (
	"log/slog"
	"sync"
)

// Subscriber processes events from the EventBus.
type Subscriber interface {
	HandleEvent(event Event)
}

// EventBus is the central event backbone. Every component that does
// significant work receives *EventBus via constructor injection.
// Emit is non-blocking until the buffer fills (backpressure).
type EventBus struct {
	subscribers []Subscriber
	mu          sync.RWMutex
	buffer      chan Event
	done        chan struct{}
	drainOnce   sync.Once
}

// NewEventBus creates an EventBus with the given buffer size and starts
// the dispatch goroutine.
func NewEventBus(bufferSize int) *EventBus {
	b := &EventBus{
		buffer: make(chan Event, bufferSize),
		done:   make(chan struct{}),
	}
	go b.dispatch()
	return b
}

// Emit sends an event to the buffer. Blocks if the buffer is full
// (backpressure — better than silent drop per SPEC.md).
func (b *EventBus) Emit(event Event) {
	b.buffer <- event
}

// Subscribe adds a subscriber and returns an unsubscribe function.
func (b *EventBus) Subscribe(sub Subscriber) func() {
	b.mu.Lock()
	b.subscribers = append(b.subscribers, sub)
	idx := len(b.subscribers) - 1
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		// Nil the slot rather than re-slicing to preserve indices
		if idx < len(b.subscribers) {
			b.subscribers[idx] = nil
		}
	}
}

// Drain closes the buffer and waits for all buffered events to be delivered.
// Call on shutdown.
func (b *EventBus) Drain() {
	b.drainOnce.Do(func() {
		close(b.buffer)
		<-b.done
	})
}

func (b *EventBus) dispatch() {
	defer close(b.done)
	for event := range b.buffer {
		b.mu.RLock()
		subs := b.subscribers
		b.mu.RUnlock()

		for _, sub := range subs {
			if sub != nil {
				func() {
					defer func() {
						if r := recover(); r != nil {
							slog.Error("subscriber panicked", "panic", r, "event", event.EventKind())
						}
					}()
					sub.HandleEvent(event)
				}()
			}
		}
	}
}
