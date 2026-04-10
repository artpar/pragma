package observe

import "sync"

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

// Emit sends an event to the buffer. Drops the event if the buffer is full
// to prevent deadlocking callers (streaming goroutines, tool execution).
func (b *EventBus) Emit(event Event) {
	select {
	case b.buffer <- event:
	default:
		// Buffer full — drop event rather than block.
		// This prevents a slow subscriber from freezing the entire application.
	}
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
	close(b.buffer)
	<-b.done
}

func (b *EventBus) dispatch() {
	defer close(b.done)
	for event := range b.buffer {
		b.mu.RLock()
		subs := b.subscribers
		b.mu.RUnlock()

		for _, sub := range subs {
			if sub != nil {
				sub.HandleEvent(event)
			}
		}
	}
}
