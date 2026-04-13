package lifecycle

import (
	"fmt"
	"sync"
)

// Checkpointer saves and restores execution state after each superstep.
type Checkpointer interface {
	Save(step int, state State) error
	Load(step int) (State, error)
	Latest() (step int, state State, err error)
}

// MemoryCheckpointer stores checkpoints in memory.
// Suitable for testing and short-lived executions.
type MemoryCheckpointer struct {
	mu      sync.RWMutex
	entries map[int]State
	latest  int
}

// NewMemoryCheckpointer creates an in-memory checkpointer.
func NewMemoryCheckpointer() *MemoryCheckpointer {
	return &MemoryCheckpointer{
		entries: make(map[int]State),
	}
}

// Save stores a snapshot of state at the given step.
func (m *MemoryCheckpointer) Save(step int, state State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[step] = state.Snapshot()
	if step > m.latest {
		m.latest = step
	}
	return nil
}

// Load retrieves the state at a given step.
func (m *MemoryCheckpointer) Load(step int) (State, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.entries[step]
	if !ok {
		return nil, fmt.Errorf("lifecycle: no checkpoint at step %d", step)
	}
	return s.Snapshot(), nil
}

// Latest returns the most recent checkpoint.
func (m *MemoryCheckpointer) Latest() (int, State, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.entries) == 0 {
		return 0, nil, fmt.Errorf("lifecycle: no checkpoints saved")
	}
	s := m.entries[m.latest]
	return m.latest, s.Snapshot(), nil
}
