package definition

import (
	"fmt"

	"github.com/artpar/pragma/internal/lifecycle"
	"github.com/artpar/pragma/internal/observe"
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	b := lifecycle.NewBuilder()

	b.SetInitialNode(def.Graph.Initial)

	if def.Graph.MaxSteps > 0 {
		observe.GlobalTrace("if: def.Graph.MaxSteps > 0")
		b.SetMaxSteps(def.Graph.MaxSteps)
	}

	for name, spec := range def.Graph.Nodes {
		observe.GlobalTrace("range def.Graph.Nodes")
		cfg := spec.ToConfig()
		fn, err := createNode(spec.Type, cfg)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"resolve node %q (type %q): %w\", name, spec.Type, err)")
			return nil, fmt.Errorf("resolve node %q (type %q): %w", name, spec.Type, err)
		}
		b.AddNode(name, fn)
	}

	hasConditional := make(map[string]bool, len(def.Graph.ConditionalEdges))
	for _, ce := range def.Graph.ConditionalEdges {
		observe.GlobalTrace("range def.Graph.ConditionalEdges (pre-pass)")
		hasConditional[ce.From] = true
	}

	for _, e := range def.Graph.Edges {
		observe.GlobalTrace("range def.Graph.Edges")
		if hasConditional[e.From] {
			observe.GlobalTrace("if: hasConditional[e.From]")
			return nil, fmt.Errorf("resolve graph: node %q has both static and conditional edges", e.From)
		}
		if e.To == "" {
			observe.GlobalTrace("if: e.To == \"\"")
			continue
		}
		b.AddEdge(e.From, e.To)
	}

	for _, ce := range def.Graph.ConditionalEdges {
		observe.GlobalTrace("range def.Graph.ConditionalEdges")
		router, err := createRouter(ce.Router)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"resolve router for node %q: %w\", ce.From, err)")
			return nil, fmt.Errorf("resolve router for node %q: %w", ce.From, err)
		}
		b.AddConditionalEdges(ce.From, router, ce.Paths)
	}

	for key, reducerName := range def.Graph.Reducers {
		observe.GlobalTrace("range def.Graph.Reducers")
		fn, err := resolveReducer(reducerName, opts)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"resolve reducer for key %q: %w\", key, err)")
			return nil, fmt.Errorf("resolve reducer for key %q: %w", key, err)
		}
		b.SetReducer(key, fn)
	}
	observe.GlobalTrace("return: b.Build()")

	return b.Build()
}

// resolveReducer maps a reducer name string to a ReducerFunc.
func resolveReducer(name string, opts *ResolveOptions) (lifecycle.ReducerFunc, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch name {
	case "overwrite":
		observe.GlobalTrace("case: \"overwrite\"")
		return lifecycle.ReducerOverwrite, nil
	case "append_list":
		observe.GlobalTrace("case: \"append_list\"")
		return lifecycle.ReducerAppendList, nil
	case "merge_map":
		observe.GlobalTrace("case: \"merge_map\"")
		return lifecycle.ReducerMergeMap, nil
	case "sum":
		observe.GlobalTrace("case: \"sum\"")
		return lifecycle.ReducerSum, nil
	default:
		observe.GlobalTrace("default")

		if opts != nil {
			if fn, ok := opts.CustomReducers[name]; ok {
				observe.GlobalTrace("if: ok")
				observe.GlobalTrace("return: fn, nil")
				return fn, nil
			}
		}
		return nil, fmt.Errorf("unknown reducer: %q (built-in: overwrite, append_list, merge_map, sum)", name)
	}
}
