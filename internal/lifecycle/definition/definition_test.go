package definition_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/artpar/pragma/internal/lifecycle"
	"github.com/artpar/pragma/internal/lifecycle/definition"
)

const validYAML = `
graph:
  initial: planner
  max_steps: 50
  nodes:
    planner:
      type: llm
      prompt: "Create a plan"
    executor:
      type: tools
      tools: [Bash, Read]
    reviewer:
      type: llm
      prompt: "Review execution"
  edges:
    - from: planner
      to: executor
    - from: executor
      to: reviewer
  conditional_edges:
    - from: reviewer
      router: "field:done"
      paths:
        "true": ""
        "false": executor
  reducers:
    messages: append_list
    turn_count: sum
`

func TestParse_Valid(t *testing.T) {
	def, err := definition.Parse([]byte(validYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def.Graph.Initial != "planner" {
		t.Errorf("expected initial=planner, got %s", def.Graph.Initial)
	}
	if def.Graph.MaxSteps != 50 {
		t.Errorf("expected max_steps=50, got %d", def.Graph.MaxSteps)
	}
	if len(def.Graph.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(def.Graph.Nodes))
	}
	if def.Graph.Nodes["planner"].Type != "llm" {
		t.Error("planner should be type llm")
	}
	if def.Graph.Nodes["executor"].Type != "tools" {
		t.Error("executor should be type tools")
	}
	if len(def.Graph.Nodes["executor"].Tools) != 2 {
		t.Errorf("executor should have 2 tools, got %d", len(def.Graph.Nodes["executor"].Tools))
	}
	if len(def.Graph.Edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(def.Graph.Edges))
	}
	if len(def.Graph.ConditionalEdges) != 1 {
		t.Errorf("expected 1 conditional edge, got %d", len(def.Graph.ConditionalEdges))
	}
	if def.Graph.ConditionalEdges[0].Router != "field:done" {
		t.Error("expected router=field:done")
	}
	if len(def.Graph.Reducers) != 2 {
		t.Errorf("expected 2 reducers, got %d", len(def.Graph.Reducers))
	}
}

func TestParse_Invalid(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{"missing initial", `graph: {nodes: {a: {type: llm}}}`},
		{"missing nodes", `graph: {initial: a}`},
		{"initial not in nodes", `graph: {initial: missing, nodes: {a: {type: llm}}}`},
		{"edge from unknown", `graph: {initial: a, nodes: {a: {type: llm}}, edges: [{from: unknown, to: a}]}`},
		{"edge to unknown", `graph: {initial: a, nodes: {a: {type: llm}}, edges: [{from: a, to: unknown}]}`},
		{"conditional edge from unknown", `graph: {initial: a, nodes: {a: {type: llm}}, conditional_edges: [{from: unknown, router: stop_reason, paths: {end: ""}}]}`},
		{"conditional edge path to unknown", `graph: {initial: a, nodes: {a: {type: llm}}, conditional_edges: [{from: a, router: stop_reason, paths: {end: unknown}}]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := definition.Parse([]byte(tt.yaml))
			if err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestParse_StaticEdgeToEmptyIsRejected(t *testing.T) {
	yamlDef := `
graph:
  initial: agent
  nodes:
    agent:
      type: llm
  edges:
    - from: agent
      to: ""
  reducers:
    messages: overwrite
`
	def, err := definition.Parse([]byte(yamlDef))
	if err == nil {
		t.Fatalf("expected static to: \"\" to be rejected, got def %#v", def)
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	_, err := definition.Parse([]byte(`{invalid yaml [[[`))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestNodeSpec_ToConfig(t *testing.T) {
	spec := definition.NodeSpec{
		Type:   "llm",
		Model:  "claude-sonnet-4",
		Prompt: "Be helpful",
		Tools:  []string{"Bash"},
		Config: map[string]any{"temperature": 0.5},
	}
	cfg := spec.ToConfig()
	if cfg["model"] != "claude-sonnet-4" {
		t.Errorf("expected model in config, got %v", cfg["model"])
	}
	if cfg["prompt"] != "Be helpful" {
		t.Errorf("expected prompt in config, got %v", cfg["prompt"])
	}
	if cfg["temperature"] != 0.5 {
		t.Errorf("expected temperature in config, got %v", cfg["temperature"])
	}
	tools, ok := cfg["tools"].([]string)
	if !ok || len(tools) != 1 {
		t.Errorf("expected tools in config, got %v", cfg["tools"])
	}
}

// noopCreator creates a noop NodeFunc for testing resolution without real infrastructure.
func noopCreator(nodeType string, _ map[string]any) (lifecycle.NodeFunc, error) {
	return func(_ context.Context, _ lifecycle.State) (lifecycle.StateUpdate, error) {
		return nil, nil
	}, nil
}

func TestResolve_ValidGraph(t *testing.T) {
	def, err := definition.Parse([]byte(validYAML))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	graph, err := definition.Resolve(def, noopCreator, definition.DefaultRouterCreator(), nil)
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	if graph == nil {
		t.Fatal("expected non-nil graph")
	}
}

func TestResolve_UnknownReducer(t *testing.T) {
	yaml := `
graph:
  initial: a
  nodes:
    a: {type: llm}
  reducers:
    messages: unknown_reducer
`
	def, err := definition.Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	_, err = definition.Resolve(def, noopCreator, definition.DefaultRouterCreator(), nil)
	if err == nil {
		t.Error("expected error for unknown reducer")
	}
}

func TestResolve_CustomReducer(t *testing.T) {
	yaml := `
graph:
  initial: a
  nodes:
    a: {type: llm}
  reducers:
    messages: my_custom
`
	def, err := definition.Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	opts := &definition.ResolveOptions{
		CustomReducers: map[string]lifecycle.ReducerFunc{
			"my_custom": func(existing, incoming any) any { return incoming },
		},
	}
	graph, err := definition.Resolve(def, noopCreator, definition.DefaultRouterCreator(), opts)
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	if graph == nil {
		t.Fatal("expected non-nil graph")
	}
}

func TestResolve_UnknownRouter(t *testing.T) {
	yaml := `
graph:
  initial: a
  nodes:
    a: {type: llm}
  conditional_edges:
    - from: a
      router: "unknown_router_type"
      paths: {"x": ""}
`
	def, err := definition.Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	_, err = definition.Resolve(def, noopCreator, definition.DefaultRouterCreator(), nil)
	if err == nil {
		t.Error("expected error for unknown router")
	}
}

func TestResolve_RejectsMixedStaticAndConditionalEdges(t *testing.T) {
	yaml := `
graph:
  initial: a
  nodes:
    a: {type: llm}
    b: {type: llm}
    c: {type: llm}
  edges:
    - from: a
      to: b
  conditional_edges:
    - from: a
      router: "field:route"
      paths: {"c": "c"}
`
	def, err := definition.Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	_, err = definition.Resolve(def, noopCreator, definition.DefaultRouterCreator(), nil)
	if err == nil {
		t.Fatal("expected mixed static and conditional edges to be rejected")
	}
}

func TestDefaultRouterCreator(t *testing.T) {
	creator := definition.DefaultRouterCreator()

	tests := []struct {
		spec   string
		state  lifecycle.State
		expect string
	}{
		{"stop_reason", lifecycle.State{"stop_reason": "tool_use"}, "continue"},
		{"stop_reason", lifecycle.State{"stop_reason": "end_turn"}, "end"},
		{"field:done", lifecycle.State{"done": true}, "true"},
		{"field:done", lifecycle.State{"done": false}, "false"},
		{"field:status", lifecycle.State{"status": "complete"}, "complete"},
		{"pass_fail", lifecycle.State{"passed": true}, "pass"},
		{"pass_fail", lifecycle.State{"passed": false}, "fail"},
	}

	for _, tt := range tests {
		t.Run(tt.spec, func(t *testing.T) {
			router, err := creator(tt.spec)
			if err != nil {
				t.Fatalf("unexpected error creating router %q: %v", tt.spec, err)
			}
			got := router(tt.state)
			if got != tt.expect {
				t.Errorf("router(%q) with state = %q, want %q", tt.spec, got, tt.expect)
			}
		})
	}

	// Error cases
	_, err := creator("unknown")
	if err == nil {
		t.Error("expected error for unknown router spec")
	}
	_, err = creator("field:")
	if err == nil {
		t.Error("expected error for empty field key")
	}
}

func TestResolve_NodeCreatorError(t *testing.T) {
	yaml := `
graph:
  initial: a
  nodes:
    a: {type: unknown_type}
`
	def, err := definition.Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	failCreator := func(nodeType string, _ map[string]any) (lifecycle.NodeFunc, error) {
		return nil, fmt.Errorf("unsupported type: %s", nodeType)
	}

	_, err = definition.Resolve(def, failCreator, definition.DefaultRouterCreator(), nil)
	if err == nil {
		t.Error("expected error from node creator")
	}
}
