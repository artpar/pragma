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

// Snapshot returns a deep copy of the current state.
// The Conversation is deep-copied so the caller cannot observe or
// mutate the store's internal data through the returned snapshot.
func (s *StateStore) Snapshot() AppState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap := s.state
	snap.Conversation = s.state.Conversation.DeepCopy()
	if s.state.Temperature != nil {
		t := *s.state.Temperature
		snap.Temperature = &t
	}
	if s.state.Thinking != nil {
		t := *s.state.Thinking
		snap.Thinking = &t
	}
	if len(s.state.Todos) > 0 {
		snap.Todos = make([]TodoItem, len(s.state.Todos))
		copy(snap.Todos, s.state.Todos)
	}
	return snap
}

// Update applies a mutation function under write lock.
func (s *StateStore) Update(fn func(*AppState)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.state)
}
