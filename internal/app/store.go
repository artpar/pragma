package app

import "sync"

// StateStore provides thread-safe access to AppState.
// Snapshot() for reads, Update() for writes.
type StateStore struct {
	mu    sync.RWMutex
	state AppState
}

// NewStateStore creates a StateStore with the given initial state.
func NewStateStore(initial AppState) *StateStore {
	return &StateStore{state: initial}
}

// Snapshot returns a copy of the current state.
func (s *StateStore) Snapshot() AppState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// Update applies a mutation function under write lock.
func (s *StateStore) Update(fn func(*AppState)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.state)
}
