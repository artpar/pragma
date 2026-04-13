package definition

import (
	"fmt"

	"github.com/artpar/gogent/internal/lifecycle"
)

// NodeCreator is called by Resolve for each node spec to produce a NodeFunc.
// nodeType is the spec's Type field, config is the merged configuration.
type NodeCreator func(nodeType string, config map[string]any) (lifecycle.NodeFunc, error)

// ResolveOptions configures the resolution process.
type ResolveOptions struct {
	// CustomReducers maps reducer name strings to ReducerFunc values.
	// These supplement the built-in reducers ("overwrite", "append_list", "merge_map", "sum").
	CustomReducers map[string]lifecycle.ReducerFunc
}

// Resolve converts a GraphDef into an executable lifecycle.Graph.
// The createNode function maps node types to NodeFunc implementations.
// The createRouter function maps router spec strings to RouterFunc implementations.
func Resolve(def *GraphDef, createNode NodeCreator, createRouter RouterCreator, opts *ResolveOptions) (*lifecycle.Graph, error) {
	b := lifecycle.NewBuilder()

	// Set initial node
	b.SetInitialNode(def.Graph.Initial)

	// Set max steps if specified
	if def.Graph.MaxSteps > 0 {
		b.SetMaxSteps(def.Graph.MaxSteps)
	}

	// Resolve and add nodes
	for name, spec := range def.Graph.Nodes {
		cfg := spec.ToConfig()
		fn, err := createNode(spec.Type, cfg)
		if err != nil {
			return nil, fmt.Errorf("resolve node %q (type %q): %w", name, spec.Type, err)
		}
		b.AddNode(name, fn)
	}

	// Add static edges
	for _, e := range def.Graph.Edges {
		b.AddEdge(e.From, e.To)
	}

	// Resolve and add conditional edges
	for _, ce := range def.Graph.ConditionalEdges {
		router, err := createRouter(ce.Router)
		if err != nil {
			return nil, fmt.Errorf("resolve router for node %q: %w", ce.From, err)
		}
		b.AddConditionalEdges(ce.From, router, ce.Paths)
	}

	// Resolve reducers
	for key, reducerName := range def.Graph.Reducers {
		fn, err := resolveReducer(reducerName, opts)
		if err != nil {
			return nil, fmt.Errorf("resolve reducer for key %q: %w", key, err)
		}
		b.SetReducer(key, fn)
	}

	return b.Build()
}

// resolveReducer maps a reducer name string to a ReducerFunc.
func resolveReducer(name string, opts *ResolveOptions) (lifecycle.ReducerFunc, error) {
	switch name {
	case "overwrite":
		return lifecycle.ReducerOverwrite, nil
	case "append_list":
		return lifecycle.ReducerAppendList, nil
	case "merge_map":
		return lifecycle.ReducerMergeMap, nil
	case "sum":
		return lifecycle.ReducerSum, nil
	default:
		// Check custom reducers
		if opts != nil {
			if fn, ok := opts.CustomReducers[name]; ok {
				return fn, nil
			}
		}
		return nil, fmt.Errorf("unknown reducer: %q (built-in: overwrite, append_list, merge_map, sum)", name)
	}
}
