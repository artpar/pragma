package definition

// GraphDef is the top-level YAML-parseable definition of a lifecycle graph.
type GraphDef struct {
	Graph GraphSpec `yaml:"graph"`
}

// GraphSpec defines the graph structure, nodes, edges, and reducers.
type GraphSpec struct {
	// Initial is the name of the first node to execute.
	Initial string `yaml:"initial"`
	// MaxSteps limits total supersteps before the executor returns ErrMaxStepsExceeded.
	MaxSteps int `yaml:"max_steps,omitempty"`
	// Nodes maps node names to their specifications.
	Nodes map[string]NodeSpec `yaml:"nodes"`
	// Edges defines static (unconditional) edges between nodes.
	Edges []EdgeSpec `yaml:"edges,omitempty"`
	// ConditionalEdges defines edges with router-based branching.
	ConditionalEdges []ConditionalEdgeSpec `yaml:"conditional_edges,omitempty"`
	// Reducers maps state key names to reducer type strings.
	// Built-in: "overwrite", "append_list", "merge_map", "sum".
	// Custom reducers can be registered via ResolveOptions.
	Reducers map[string]string `yaml:"reducers,omitempty"`
}

// NodeSpec defines a single node in the graph.
type NodeSpec struct {
	// Type is the node type: "llm", "tools", "eval", "reflect", etc.
	Type string `yaml:"type"`
	// Model overrides the model from state for this node (LLM nodes only).
	Model string `yaml:"model,omitempty"`
	// Prompt is the system prompt override for this node (LLM nodes only).
	Prompt string `yaml:"prompt,omitempty"`
	// Tools lists tool names available to this node (tools nodes only).
	Tools []string `yaml:"tools,omitempty"`
	// Config holds additional type-specific configuration.
	Config map[string]any `yaml:"config,omitempty"`
}

// EdgeSpec defines a static edge from one node to another.
type EdgeSpec struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

// ConditionalEdgeSpec defines an edge with router-based branching.
type ConditionalEdgeSpec struct {
	// From is the source node name.
	From string `yaml:"from"`
	// Router is the router specification string.
	// Built-in: "stop_reason", "field:<key>", "pass_fail".
	Router string `yaml:"router"`
	// Paths maps router return values to target node names.
	// Empty string ("") means END (graph termination).
	Paths map[string]string `yaml:"paths"`
}

// ToConfig merges the top-level NodeSpec fields into a flat config map
// suitable for passing to a NodeCreator.
func (ns NodeSpec) ToConfig() map[string]any {
	cfg := make(map[string]any)
	for k, v := range ns.Config {
		cfg[k] = v
	}
	if ns.Model != "" {
		cfg["model"] = ns.Model
	}
	if ns.Prompt != "" {
		cfg["prompt"] = ns.Prompt
	}
	if len(ns.Tools) > 0 {
		cfg["tools"] = ns.Tools
	}
	return cfg
}
