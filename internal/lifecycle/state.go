package lifecycle

import (
	"reflect"

	"github.com/artpar/pragma/internal/observe"
)

// State is the shared blackboard flowing through the graph.
// Keys are strings, values are any type. Nodes receive a snapshot
// (keys copied, values shared by reference). Nodes MUST NOT mutate
// values in place — they return new values via StateUpdate.
//
// The framework does NOT enforce a schema. Nodes and routers use
// type assertions on the values they expect.
type State map[string]any

// Snapshot creates a copy of State with independent map and slice containers.
// Pointer-like values are still shared, so nodes must return StateUpdate values
// instead of mutating infrastructure objects in place.
func (s State) Snapshot() State {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	cp := make(State, len(s))
	for k, v := range s {
		observe.GlobalTrace("range s")
		cp[k] = cloneStateValue(v)
	}
	observe.GlobalTrace("return: cp")
	return cp
}

func cloneStateValue(value any) any {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if value == nil {
		observe.GlobalTrace("if: value == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	cloned := cloneReflectValue(reflect.ValueOf(value))
	if !cloned.IsValid() {
		observe.GlobalTrace("if: !cloned.IsValid()")
		observe.GlobalTrace("return: nil")
		return nil
	}
	observe.GlobalTrace("return: cloned.Interface()")
	return cloned.Interface()
}

func cloneReflectValue(value reflect.Value) reflect.Value {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !value.IsValid() {
		observe.GlobalTrace("if: !value.IsValid()")
		observe.GlobalTrace("return: value")
		return value
	}
	switch value.Kind() {
	case reflect.Interface:
		observe.GlobalTrace("case: reflect.Interface")
		if value.IsNil() {
			observe.GlobalTrace("return: value")
			return value
		}
		cloned := cloneReflectValue(value.Elem())
		out := reflect.New(value.Type()).Elem()
		out.Set(cloned)
		return out
	case reflect.Slice:
		observe.GlobalTrace("case: reflect.Slice")
		if value.IsNil() {
			observe.GlobalTrace("return: value")
			return value
		}
		out := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for i := 0; i < value.Len(); i++ {
			out.Index(i).Set(cloneReflectValue(value.Index(i)))
		}
		return out
	case reflect.Map:
		observe.GlobalTrace("case: reflect.Map")
		if value.IsNil() {
			observe.GlobalTrace("return: value")
			return value
		}
		out := reflect.MakeMapWithSize(value.Type(), value.Len())
		iter := value.MapRange()
		for iter.Next() {
			out.SetMapIndex(iter.Key(), cloneReflectValue(iter.Value()))
		}
		return out
	default:
		observe.GlobalTrace("default")
		return value
	}
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: incoming")
	return incoming
}

// ReducerAppendList appends incoming slice elements to existing slice.
// Both must be []any. If existing is nil, returns incoming as-is.
func ReducerAppendList(existing, incoming any) any {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if existing == nil {
		observe.GlobalTrace("if: existing == nil")
		observe.GlobalTrace("return: incoming")
		return incoming
	}
	existSlice, ok1 := existing.([]any)
	incomSlice, ok2 := incoming.([]any)
	if !ok1 || !ok2 {
		observe.GlobalTrace("if: !ok1 || !ok2")
		observe.GlobalTrace("return: incoming")
		return incoming
	}
	result := make([]any, len(existSlice)+len(incomSlice))
	copy(result, existSlice)
	copy(result[len(existSlice):], incomSlice)
	observe.GlobalTrace("return: result")
	return result
}

// ReducerMergeMap merges incoming map into existing map.
// Incoming keys overwrite existing keys. Both must be map[string]any.
func ReducerMergeMap(existing, incoming any) any {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if existing == nil {
		observe.GlobalTrace("if: existing == nil")
		observe.GlobalTrace("return: incoming")
		return incoming
	}
	existMap, ok1 := existing.(map[string]any)
	incomMap, ok2 := incoming.(map[string]any)
	if !ok1 || !ok2 {
		observe.GlobalTrace("if: !ok1 || !ok2")
		observe.GlobalTrace("return: incoming")
		return incoming
	}
	result := make(map[string]any, len(existMap)+len(incomMap))
	for k, v := range existMap {
		observe.GlobalTrace("range existMap")
		result[k] = v
	}
	for k, v := range incomMap {
		observe.GlobalTrace("range incomMap")
		result[k] = v
	}
	observe.GlobalTrace("return: result")
	return result
}

// ReducerSum adds incoming numeric value to existing.
// Supports int and float64.
func ReducerSum(existing, incoming any) any {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if existing == nil {
		observe.GlobalTrace("if: existing == nil")
		observe.GlobalTrace("return: incoming")
		return incoming
	}
	switch e := existing.(type) {
	case int:
		observe.GlobalTrace("typecase: int")
		if i, ok := incoming.(int); ok {
			observe.GlobalTrace("return: e + i")
			return e + i
		}
	case float64:
		observe.GlobalTrace("typecase: float64")
		if i, ok := incoming.(float64); ok {
			observe.GlobalTrace("return: e + i")
			return e + i
		}
	}
	observe.GlobalTrace("return: incoming")
	return incoming
}

// applyUpdate merges a StateUpdate into a State using reducers.
// Returns a NEW State — the original is not modified.
func applyUpdate(state State, update StateUpdate, reducers map[string]ReducerFunc) State {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	result := state.Snapshot()
	for key, incoming := range update {
		observe.GlobalTrace("range update")

		if incoming == nil {
			observe.GlobalTrace("if: incoming == nil")
			delete(result, key)
			continue
		}
		existing, exists := result[key]
		if !exists {
			observe.GlobalTrace("if: !exists")
			result[key] = incoming
			continue
		}

		if reducer, ok := reducers[key]; ok {
			observe.GlobalTrace("if: ok")
			result[key] = reducer(existing, incoming)
		} else {
			observe.GlobalTrace("else: ok")
			result[key] = incoming
		}
	}
	observe.GlobalTrace("return: result")
	return result
}
