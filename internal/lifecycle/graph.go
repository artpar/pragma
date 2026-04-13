package lifecycle

import (
	"context"
	"fmt"
)

const defaultMaxSteps = 100

// Graph defines a state machine for agent lifecycle execution.
// Immutable after Build() — safe for concurrent use.
type Graph struct {
	nodes            map[string]NodeFunc
	edges            map[string][]string         // from → [to1, to2, ...] (fan-out)
	conditionalEdges map[string]ConditionalEdge
	initialNode      string
	reducers         map[string]ReducerFunc
	maxSteps         int
}

// NodeFunc processes state and returns a partial update.
// Context carries cancellation and deadline.
type NodeFunc func(ctx context.Context, state State) (StateUpdate, error)

// RouterFunc inspects state and returns the name of the next node.
// Returns "" to signal END (termination).
type RouterFunc func(state State) string

// ConditionalEdge routes from a source node based on state inspection.
type ConditionalEdge struct {
	Router  RouterFunc
	PathMap map[string]string // router return value → node name ("" = END)
}

// Builder constructs a Graph incrementally.
type Builder struct {
	nodes            map[string]NodeFunc
	edges            map[string][]string
	conditionalEdges map[string]ConditionalEdge
	reducers         map[string]ReducerFunc
	initialNode      string
	maxSteps         int
}

// NewBuilder creates a new graph builder with default settings.
func NewBuilder() *Builder {
	return &Builder{
		nodes:            make(map[string]NodeFunc),
		edges:            make(map[string][]string),
		conditionalEdges: make(map[string]ConditionalEdge),
		reducers:         make(map[string]ReducerFunc),
		maxSteps:         defaultMaxSteps,
	}
}

// AddNode registers a named processing function.
func (b *Builder) AddNode(name string, fn NodeFunc) *Builder {
	b.nodes[name] = fn
	return b
}

// AddEdge adds a static transition from → to.
// Multiple edges from the same source create fan-out (parallel execution).
func (b *Builder) AddEdge(from, to string) *Builder {
	b.edges[from] = append(b.edges[from], to)
	return b
}

// AddConditionalEdges adds a routing function from a source node.
// The router inspects state and returns a key; pathMap maps keys to node names.
// A key mapping to "" means END (termination).
func (b *Builder) AddConditionalEdges(from string, router RouterFunc, pathMap map[string]string) *Builder {
	b.conditionalEdges[from] = ConditionalEdge{
		Router:  router,
		PathMap: pathMap,
	}
	return b
}

// SetReducer sets the merge function for a state key.
// Used when parallel branches update the same key.
func (b *Builder) SetReducer(key string, fn ReducerFunc) *Builder {
	b.reducers[key] = fn
	return b
}

// SetInitialNode sets the entry point node.
func (b *Builder) SetInitialNode(name string) *Builder {
	b.initialNode = name
	return b
}

// SetMaxSteps sets the cycle safety limit (default: 100).
func (b *Builder) SetMaxSteps(n int) *Builder {
	b.maxSteps = n
	return b
}

// Build validates the graph and returns an immutable Graph.
func (b *Builder) Build() (*Graph, error) {
	if b.initialNode == "" {
		return nil, fmt.Errorf("lifecycle: initial node not set")
	}
	if _, ok := b.nodes[b.initialNode]; !ok {
		return nil, fmt.Errorf("lifecycle: initial node %q not registered", b.initialNode)
	}
	if b.maxSteps <= 0 {
		return nil, fmt.Errorf("lifecycle: maxSteps must be > 0")
	}

	// Validate all edge targets exist
	for from, targets := range b.edges {
		if _, ok := b.nodes[from]; !ok {
			return nil, fmt.Errorf("lifecycle: edge source %q not registered", from)
		}
		for _, to := range targets {
			if _, ok := b.nodes[to]; !ok {
				return nil, fmt.Errorf("lifecycle: edge target %q not registered (from %q)", to, from)
			}
		}
	}

	// Validate conditional edge sources and targets
	for from, ce := range b.conditionalEdges {
		if _, ok := b.nodes[from]; !ok {
			return nil, fmt.Errorf("lifecycle: conditional edge source %q not registered", from)
		}
		for key, target := range ce.PathMap {
			if target == "" {
				continue // "" = END, always valid
			}
			if _, ok := b.nodes[target]; !ok {
				return nil, fmt.Errorf("lifecycle: conditional edge target %q not registered (from %q, key %q)", target, from, key)
			}
		}
	}

	// Validate no node has BOTH static edges and conditional edges
	for from := range b.conditionalEdges {
		if _, hasStatic := b.edges[from]; hasStatic {
			return nil, fmt.Errorf("lifecycle: node %q has both static and conditional edges (ambiguous)", from)
		}
	}

	// Deep copy everything for immutability
	nodes := make(map[string]NodeFunc, len(b.nodes))
	for k, v := range b.nodes {
		nodes[k] = v
	}
	edges := make(map[string][]string, len(b.edges))
	for k, v := range b.edges {
		cp := make([]string, len(v))
		copy(cp, v)
		edges[k] = cp
	}
	conds := make(map[string]ConditionalEdge, len(b.conditionalEdges))
	for k, v := range b.conditionalEdges {
		pm := make(map[string]string, len(v.PathMap))
		for pk, pv := range v.PathMap {
			pm[pk] = pv
		}
		conds[k] = ConditionalEdge{Router: v.Router, PathMap: pm}
	}
	reducers := make(map[string]ReducerFunc, len(b.reducers))
	for k, v := range b.reducers {
		reducers[k] = v
	}

	return &Graph{
		nodes:            nodes,
		edges:            edges,
		conditionalEdges: conds,
		initialNode:      b.initialNode,
		reducers:         reducers,
		maxSteps:         b.maxSteps,
	}, nil
}
