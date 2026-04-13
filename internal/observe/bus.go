package observe

import (
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
)

// Subscriber processes events from the EventBus.
type Subscriber interface {
	HandleEvent(event Event)
}

// Ring buffer constants. 65,536 slots pre-allocated at init (~1.5MB).
// Codebase has ~5,654 trace points; 65K gives 11x headroom.
const (
	ringBits = 16
	ringSize = 1 << ringBits       // 65536
	ringMask = int64(ringSize - 1)
)

// slot holds one event in the ring buffer. The sequence counter tracks
// the slot's state: seq==pos means free for producer at position pos,
// seq==pos+1 means data ready for consumer.
type slot struct {
	seq   atomic.Int64
	event Event
}

// EventBus is the central event backbone. Lock-free MPSC ring buffer
// (Vyukov, 2010). Emit is non-blocking, zero per-call allocation.
// Every event is captured — no drops.
type EventBus struct {
	// --- producer hot path (cache-line isolated) ---
	head atomic.Int64 // next write position (producers CAS)
	_p1  [56]byte     // pad to 64-byte cache line

	// --- consumer hot path (cache-line isolated) ---
	tail int64    // next read position (single consumer, no atomic)
	_p2  [56]byte

	// --- shared, rarely touched ---
	slots     []slot
	wake      chan struct{} // capacity 1, consumer sleep signal
	done      chan struct{}
	closed    atomic.Bool
	drainOnce sync.Once

	subscribers []Subscriber
	subMu       sync.RWMutex // only for Subscribe/Unsubscribe, never on Emit path
}

// NewEventBus creates an EventBus with a pre-allocated lock-free ring buffer
// and starts the dispatch goroutine. The bufferSize parameter is accepted
// for API compatibility but ignored (ring size is fixed at 65,536).
func NewEventBus(_ int) *EventBus {
	b := &EventBus{
		slots: make([]slot, ringSize),
		wake:  make(chan struct{}, 1),
		done:  make(chan struct{}),
	}
	// Init sequence counters: slot[i].seq = i marks slot as free for position i.
	for i := range b.slots {
		b.slots[i].seq.Store(int64(i))
	}
	go b.dispatch()
	slog.Debug("EventBus created", "ringSize", ringSize)
	return b
}

// Emit enqueues an event into the ring buffer. Lock-free, zero per-call
// allocation. Multiple goroutines may call Emit concurrently.
// If the ring is full (consumer hasn't caught up), Emit yields via
// runtime.Gosched and retries — the consumer always makes progress.
func (b *EventBus) Emit(event Event) {
	if b.closed.Load() {
		return
	}
	for {
		pos := b.head.Load()
		s := &b.slots[pos&ringMask]
		seq := s.seq.Load()
		switch {
		case seq == pos:
			// Slot is free for this position — claim it with CAS.
			if b.head.CompareAndSwap(pos, pos+1) {
				s.event = event
				s.seq.Store(pos + 1) // signal consumer
				// Wake dispatch goroutine (non-blocking).
				select {
				case b.wake <- struct{}{}:
				default:
				}
				return
			}
			// CAS failed — another producer claimed it, retry.
		case seq-pos < 0:
			// Ring full — consumer behind. Yield and retry.
			runtime.Gosched()
		default:
			// seq > pos — slot not yet recycled by consumer for this
			// wrap-around position. Reload head (may have advanced).
		}
	}
}

// Subscribe adds a subscriber and returns an unsubscribe function.
// Not on the hot path — uses a mutex.
func (b *EventBus) Subscribe(sub Subscriber) func() {
	b.subMu.Lock()
	b.subscribers = append(b.subscribers, sub)
	idx := len(b.subscribers) - 1
	b.subMu.Unlock()

	return func() {
		b.subMu.Lock()
		defer b.subMu.Unlock()
		if idx < len(b.subscribers) {
			b.subscribers[idx] = nil
		}
	}
}

// Drain signals shutdown and waits for all pending events to be delivered.
// Idempotent — safe to call multiple times.
func (b *EventBus) Drain() {
	b.drainOnce.Do(func() {
		b.closed.Store(true)
		close(b.wake)
		<-b.done
	})
}

// dispatch is the single consumer goroutine. Wakes on signal, drains all
// ready slots, then sleeps until the next signal.
func (b *EventBus) dispatch() {
	defer close(b.done)
	spins := 0
	for {
		drained := b.drainRing()
		if drained {
			spins = 0
			continue // more events may be ready (re-entrant emit)
		}
		// Nothing drained this round.
		if b.closed.Load() {
			return // shutdown, ring empty
		}
		// Spin briefly for re-entrant events, then sleep on wake channel.
		spins++
		if spins < 4 {
			runtime.Gosched()
			continue
		}
		spins = 0
		// Sleep until next Emit signal or shutdown.
		_, ok := <-b.wake
		if !ok {
			_ = b.drainRing() // final drain
			return
		}
	}
}

// drainRing reads all ready slots and delivers events to subscribers.
// Returns true if at least one event was delivered.
func (b *EventBus) drainRing() bool {
	b.subMu.RLock()
	subs := b.subscribers
	b.subMu.RUnlock()

	delivered := false
	for {
		pos := b.tail
		s := &b.slots[pos&ringMask]
		seq := s.seq.Load()
		if seq != pos+1 {
			return delivered
		}
		delivered = true
		event := s.event
		s.event = nil                       // help GC
		s.seq.Store(pos + int64(ringSize))  // free slot for future producers
		b.tail = pos + 1

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
