package definition_test

import (
	"testing"

	"github.com/artpar/pragma/internal/lifecycle/definition"
)

func TestFixLLMToolRouting_SingleLLMNode_NoStopRouting(t *testing.T) {
	def := &definition.GraphDef{}
	def.Graph.Initial = "actor"
	def.Graph.Nodes = map[string]definition.NodeSpec{
		"actor":     {Type: "llm"},
		"evaluator": {Type: "eval"},
	}
	def.Graph.Edges = []definition.EdgeSpec{
		{From: "actor", To: "evaluator"},
	}

	definition.FixLLMToolRouting(def)

	// Should have created a tools node and stop_reason routing.
	if _, ok := def.Graph.Nodes["actor_tools"]; !ok {
		t.Fatal("expected actor_tools node to be created")
	}
	if def.Graph.Nodes["actor_tools"].Type != "tools" {
		t.Fatalf("expected actor_tools to be type tools, got %s", def.Graph.Nodes["actor_tools"].Type)
	}

	// Should have a conditional edge from actor with stop_reason.
	found := false
	for _, ce := range def.Graph.ConditionalEdges {
		if ce.From == "actor" && ce.Router == "stop_reason" {
			found = true
			if ce.Paths["continue"] != "actor_tools" {
				t.Fatalf("expected continue path to actor_tools, got %s", ce.Paths["continue"])
			}
			if ce.Paths["end"] != "evaluator" {
				t.Fatalf("expected end path to evaluator, got %s", ce.Paths["end"])
			}
		}
	}
	if !found {
		t.Fatal("expected stop_reason conditional edge from actor")
	}

	// Should have back-edge from tools to actor.
	foundBackEdge := false
	for _, e := range def.Graph.Edges {
		if e.From == "actor_tools" && e.To == "actor" {
			foundBackEdge = true
		}
	}
	if !foundBackEdge {
		t.Fatal("expected back-edge from actor_tools to actor")
	}

	// Original unconditional edge should be removed.
	for _, e := range def.Graph.Edges {
		if e.From == "actor" && e.To == "evaluator" {
			t.Fatal("original unconditional edge should have been removed")
		}
	}
}

func TestFixLLMToolRouting_AlreadyHasStopRouting(t *testing.T) {
	def := &definition.GraphDef{}
	def.Graph.Initial = "actor"
	def.Graph.Nodes = map[string]definition.NodeSpec{
		"actor": {Type: "llm"},
		"tools": {Type: "tools"},
	}
	def.Graph.Edges = []definition.EdgeSpec{
		{From: "tools", To: "actor"},
	}
	def.Graph.ConditionalEdges = []definition.ConditionalEdgeSpec{
		{From: "actor", Router: "stop_reason", Paths: map[string]string{"continue": "tools", "end": ""}},
	}

	edgeCountBefore := len(def.Graph.Edges)
	condCountBefore := len(def.Graph.ConditionalEdges)

	definition.FixLLMToolRouting(def)

	// Should not modify anything.
	if len(def.Graph.Edges) != edgeCountBefore {
		t.Fatalf("expected %d edges, got %d", edgeCountBefore, len(def.Graph.Edges))
	}
	if len(def.Graph.ConditionalEdges) != condCountBefore {
		t.Fatalf("expected %d conditional edges, got %d", condCountBefore, len(def.Graph.ConditionalEdges))
	}
}

func TestFixLLMToolRouting_MultipleLLMNodes_DedicatedTools(t *testing.T) {
	def := &definition.GraphDef{}
	def.Graph.Initial = "planner"
	def.Graph.Nodes = map[string]definition.NodeSpec{
		"planner":  {Type: "llm"},
		"auditor":  {Type: "llm"},
		"verifier": {Type: "llm"},
	}
	def.Graph.Edges = []definition.EdgeSpec{
		{From: "planner", To: "auditor"},
		{From: "auditor", To: "verifier"},
	}

	definition.FixLLMToolRouting(def)

	// Each LLM node with an unconditional edge should get its own tools node.
	if _, ok := def.Graph.Nodes["planner_tools"]; !ok {
		t.Fatal("expected planner_tools node")
	}
	if _, ok := def.Graph.Nodes["auditor_tools"]; !ok {
		t.Fatal("expected auditor_tools node")
	}

	// Verifier had no unconditional edge, so no fix needed.
	// (It was the target of auditor, not a source.)

	// planner should route: continue→planner_tools, end→auditor.
	for _, ce := range def.Graph.ConditionalEdges {
		if ce.From == "planner" {
			if ce.Paths["continue"] != "planner_tools" {
				t.Fatalf("planner continue: expected planner_tools, got %s", ce.Paths["continue"])
			}
			if ce.Paths["end"] != "auditor" {
				t.Fatalf("planner end: expected auditor, got %s", ce.Paths["end"])
			}
		}
	}
}

func TestFixLLMToolRouting_NilDef(t *testing.T) {
	// Should not panic.
	definition.FixLLMToolRouting(nil)
}

func TestFixLLMToolRouting_NoLLMNodes(t *testing.T) {
	def := &definition.GraphDef{}
	def.Graph.Initial = "tools"
	def.Graph.Nodes = map[string]definition.NodeSpec{
		"tools": {Type: "tools"},
	}

	definition.FixLLMToolRouting(def)

	// Should not modify anything.
	if len(def.Graph.ConditionalEdges) != 0 {
		t.Fatal("expected no conditional edges added")
	}
}

func TestFixLLMToolRouting_NoReuseWhenBackEdgeToDifferentNode(t *testing.T) {
	// Existing tools node has back-edge to "reviewer", but we're fixing "planner".
	// Reusing would create tools→reviewer AND tools→planner (parallel execution bug).
	def := &definition.GraphDef{}
	def.Graph.Initial = "planner"
	def.Graph.Nodes = map[string]definition.NodeSpec{
		"planner":  {Type: "llm"},
		"tools":    {Type: "tools"},
		"reviewer": {Type: "llm"},
	}
	def.Graph.Edges = []definition.EdgeSpec{
		{From: "planner", To: "reviewer"},
		{From: "tools", To: "reviewer"}, // existing back-edge to reviewer, NOT planner
	}
	def.Graph.ConditionalEdges = []definition.ConditionalEdgeSpec{
		{From: "reviewer", Router: "stop_reason", Paths: map[string]string{"continue": "tools", "end": ""}},
	}

	definition.FixLLMToolRouting(def)

	// Should NOT reuse existing tools node — must create planner_tools.
	if _, ok := def.Graph.Nodes["planner_tools"]; !ok {
		t.Fatal("expected planner_tools (should not reuse tools node with back-edge to different node)")
	}

	// Existing tools node should still only have its original back-edge.
	toolsEdges := 0
	for _, e := range def.Graph.Edges {
		if e.From == "tools" {
			toolsEdges++
		}
	}
	if toolsEdges != 1 {
		t.Fatalf("existing tools node should have 1 back-edge, got %d", toolsEdges)
	}
}

func TestFixLLMToolRouting_ReusesExistingToolsNode(t *testing.T) {
	def := &definition.GraphDef{}
	def.Graph.Initial = "actor"
	def.Graph.Nodes = map[string]definition.NodeSpec{
		"actor": {Type: "llm"},
		"tools": {Type: "tools"},
		"eval":  {Type: "eval"},
	}
	def.Graph.Edges = []definition.EdgeSpec{
		{From: "actor", To: "eval"},
		{From: "tools", To: "actor"},
	}

	definition.FixLLMToolRouting(def)

	// Single fix + existing tools node with 1 back-edge → should reuse.
	if _, ok := def.Graph.Nodes["actor_tools"]; ok {
		t.Fatal("should reuse existing tools node, not create actor_tools")
	}

	found := false
	for _, ce := range def.Graph.ConditionalEdges {
		if ce.From == "actor" && ce.Router == "stop_reason" {
			found = true
			if ce.Paths["continue"] != "tools" {
				t.Fatalf("expected continue→tools, got %s", ce.Paths["continue"])
			}
		}
	}
	if !found {
		t.Fatal("expected stop_reason routing from actor")
	}
}
