package observe

import (
	"sync"
	"sync/atomic"
	"testing"
)

type collectingSub struct {
	mu     sync.Mutex
	events []Event
}

func (s *collectingSub) HandleEvent(event Event) {
	s.mu.Lock()
	s.events = append(s.events, event)
	s.mu.Unlock()
}

func (s *collectingSub) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

func testEvent(kind string) Event {
	return ConversationStarted{
		EventHeader:    NewEventHeader(kind, "trace-1", "span-1", ""),
		ConversationID: "conv-1",
		Model:          "test-model",
		Provider:       "test",
		WorkDir:        "/tmp",
	}
}

func TestEventBusSingleSubscriber(t *testing.T) {
	bus := NewEventBus(100)
	sub := &collectingSub{}
	bus.Subscribe(sub)

	bus.Emit(testEvent("ConversationStarted"))
	bus.Drain()

	if sub.count() != 1 {
		t.Errorf("got %d events, want 1", sub.count())
	}
}

func TestEventBusMultipleSubscribers(t *testing.T) {
	bus := NewEventBus(100)
	sub1 := &collectingSub{}
	sub2 := &collectingSub{}
	bus.Subscribe(sub1)
	bus.Subscribe(sub2)

	bus.Emit(testEvent("ConversationStarted"))
	bus.Emit(testEvent("ConversationStarted"))
	bus.Drain()

	if sub1.count() != 2 {
		t.Errorf("sub1: got %d events, want 2", sub1.count())
	}
	if sub2.count() != 2 {
		t.Errorf("sub2: got %d events, want 2", sub2.count())
	}
}

func TestEventBusUnsubscribe(t *testing.T) {
	bus := NewEventBus(100)
	sub := &collectingSub{}
	unsub := bus.Subscribe(sub)

	bus.Emit(testEvent("ConversationStarted"))
	bus.Drain()

	count1 := sub.count()
	if count1 != 1 {
		t.Fatalf("expected 1 event before unsubscribe, got %d", count1)
	}

	// New bus to test unsubscribe (Drain closed the first)
	bus2 := NewEventBus(100)
	_ = unsub // original unsub not relevant to bus2
	unsub2 := bus2.Subscribe(sub)
	unsub2()

	bus2.Emit(testEvent("ConversationStarted"))
	bus2.Drain()

	if sub.count() != count1 {
		t.Errorf("expected no new events after unsubscribe, got %d (was %d)", sub.count(), count1)
	}
}

func TestEventBusDrainFlushes(t *testing.T) {
	bus := NewEventBus(1000)
	sub := &collectingSub{}
	bus.Subscribe(sub)

	n := 100
	for range n {
		bus.Emit(testEvent("ConversationStarted"))
	}
	bus.Drain()

	if sub.count() != n {
		t.Errorf("got %d events after drain, want %d", sub.count(), n)
	}
}

func TestEventBusConcurrentEmit(t *testing.T) {
	bus := NewEventBus(10000)

	var received atomic.Int64
	bus.Subscribe(subscriberFunc(func(event Event) {
		received.Add(1)
	}))

	var wg sync.WaitGroup
	n := 100
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			bus.Emit(testEvent("ConversationStarted"))
		}()
	}
	wg.Wait()
	bus.Drain()

	if got := received.Load(); got != int64(n) {
		t.Errorf("received %d events, want %d", got, n)
	}
}

func TestEventBusNeverDrops(t *testing.T) {
	// Ring buffer is fixed size — verify no events are dropped even
	// when emitting faster than dispatch can consume.
	bus := NewEventBus(0) // bufferSize ignored, ring is 65536
	sub := &collectingSub{}
	bus.Subscribe(sub)

	bus.Emit(testEvent("ConversationStarted"))
	bus.Emit(testEvent("ConversationStarted"))
	bus.Drain()

	if sub.count() != 2 {
		t.Errorf("got %d events, want 2 (must never drop)", sub.count())
	}
}

func TestEventBusHighVolume(t *testing.T) {
	// 10 goroutines × 1,000 events = 10,000 total — stress test the MPSC ring.
	bus := NewEventBus(0)

	var received atomic.Int64
	bus.Subscribe(subscriberFunc(func(_ Event) {
		received.Add(1)
	}))

	var wg sync.WaitGroup
	goroutines := 10
	perGoroutine := 1000
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range perGoroutine {
				bus.Emit(testEvent("ConversationStarted"))
			}
		}()
	}
	wg.Wait()
	bus.Drain()

	want := int64(goroutines * perGoroutine)
	if got := received.Load(); got != want {
		t.Errorf("received %d events, want %d (zero drops required)", got, want)
	}
}

func TestPanickedSubscriberDoesNotCrashBus(t *testing.T) {
	bus := NewEventBus(100)

	// Panicking subscriber
	bus.Subscribe(subscriberFunc(func(event Event) {
		panic("test panic")
	}))

	// Normal subscriber that should still receive events
	good := &collectingSub{}
	bus.Subscribe(good)

	bus.Emit(testEvent("ConversationStarted"))
	bus.Emit(testEvent("ConversationStarted"))
	bus.Drain()

	if good.count() != 2 {
		t.Errorf("good subscriber got %d events, want 2 (panicking sub must not crash bus)", good.count())
	}
}

func TestEventBusDrainIdempotent(t *testing.T) {
	bus := NewEventBus(100)
	sub := &collectingSub{}
	bus.Subscribe(sub)

	bus.Emit(testEvent("ConversationStarted"))

	// Double drain must not panic
	bus.Drain()
	bus.Drain()

	if sub.count() != 1 {
		t.Errorf("got %d events, want 1", sub.count())
	}
}

// subscriberFunc adapts a function to the Subscriber interface.
type subscriberFunc func(Event)

func (f subscriberFunc) HandleEvent(event Event) { f(event) }
