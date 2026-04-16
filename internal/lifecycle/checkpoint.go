package lifecycle

import (
	"fmt"
	"github.com/artpar/pragma/internal/observe"
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &MemoryCheckpointer{\n\tentries: make(map[int]State),\n}")
	return &MemoryCheckpointer{
		entries: make(map[int]State),
	}
}

// Save stores a snapshot of state at the given step.
func (m *MemoryCheckpointer) Save(step int, state State) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[step] = state.Snapshot()
	if step > m.latest {
		observe.GlobalTrace("if: step > m.latest")
		m.latest = step
	}
	observe.GlobalTrace("return: nil")
	return nil
}

// Load retrieves the state at a given step.
func (m *MemoryCheckpointer) Load(step int) (State, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.entries[step]
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"lifecycle: no checkpoint at step %d\", step)")
		return nil, fmt.Errorf("lifecycle: no checkpoint at step %d", step)
	}
	observe.GlobalTrace("return: s.Snapshot(), nil")
	return s.Snapshot(), nil
}

// Latest returns the most recent checkpoint.
func (m *MemoryCheckpointer) Latest() (int, State, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.entries) == 0 {
		observe.GlobalTrace("if: len(m.entries) == 0")
		observe.GlobalTrace("return: 0, nil, fmt.Errorf(\"lifecycle: no checkpoints saved\")")
		return 0, nil, fmt.Errorf("lifecycle: no checkpoints saved")
	}
	s := m.entries[m.latest]
	observe.GlobalTrace("return: m.latest, s.Snapshot(), nil")
	return m.latest, s.Snapshot(), nil
}
