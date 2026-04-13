package lifecycle

// State is the shared blackboard flowing through the graph.
// Keys are strings, values are any type. Nodes receive a snapshot
// (keys copied, values shared by reference). Nodes MUST NOT mutate
// values in place — they return new values via StateUpdate.
//
// The framework does NOT enforce a schema. Nodes and routers use
// type assertions on the values they expect.
type State map[string]any

// Snapshot creates a shallow copy of State (new map, same value pointers).
// Each node receives its own key map so concurrent nodes can't interfere
// with each other's key set, while sharing large values (provider instances,
// tool registries, message slices) by reference for efficiency.
func (s State) Snapshot() State {
	cp := make(State, len(s))
	for k, v := range s {
		cp[k] = v
	}
	return cp
}

// StateUpdate is a partial state update returned by a node.
// Only keys present are merged into state.
// A nil value removes the key from state.
type StateUpdate map[string]any

// ReducerFunc merges an incoming value into an existing value for a key.
// Called when a node updates a key that already has a value.
// Default (no reducer set): overwrite — incoming replaces existing.
type ReducerFunc func(existing, incoming any) any

// ReducerOverwrite replaces existing with incoming. This is the default.
func ReducerOverwrite(_, incoming any) any {
	return incoming
}

// ReducerAppendList appends incoming slice elements to existing slice.
// Both must be []any. If existing is nil, returns incoming as-is.
func ReducerAppendList(existing, incoming any) any {
	if existing == nil {
		return incoming
	}
	existSlice, ok1 := existing.([]any)
	incomSlice, ok2 := incoming.([]any)
	if !ok1 || !ok2 {
		return incoming // type mismatch fallback
	}
	result := make([]any, len(existSlice)+len(incomSlice))
	copy(result, existSlice)
	copy(result[len(existSlice):], incomSlice)
	return result
}

// ReducerMergeMap merges incoming map into existing map.
// Incoming keys overwrite existing keys. Both must be map[string]any.
func ReducerMergeMap(existing, incoming any) any {
	if existing == nil {
		return incoming
	}
	existMap, ok1 := existing.(map[string]any)
	incomMap, ok2 := incoming.(map[string]any)
	if !ok1 || !ok2 {
		return incoming
	}
	result := make(map[string]any, len(existMap)+len(incomMap))
	for k, v := range existMap {
		result[k] = v
	}
	for k, v := range incomMap {
		result[k] = v
	}
	return result
}

// ReducerSum adds incoming numeric value to existing.
// Supports int and float64.
func ReducerSum(existing, incoming any) any {
	if existing == nil {
		return incoming
	}
	switch e := existing.(type) {
	case int:
		if i, ok := incoming.(int); ok {
			return e + i
		}
	case float64:
		if i, ok := incoming.(float64); ok {
			return e + i
		}
	}
	return incoming
}

// applyUpdate merges a StateUpdate into a State using reducers.
// Returns a NEW State — the original is not modified.
func applyUpdate(state State, update StateUpdate, reducers map[string]ReducerFunc) State {
	result := state.Snapshot()
	for key, incoming := range update {
		// nil value = delete key
		if incoming == nil {
			delete(result, key)
			continue
		}
		existing, exists := result[key]
		if !exists {
			result[key] = incoming
			continue
		}
		// Apply reducer if one is set for this key
		if reducer, ok := reducers[key]; ok {
			result[key] = reducer(existing, incoming)
		} else {
			result[key] = incoming // default: overwrite
		}
	}
	return result
}
