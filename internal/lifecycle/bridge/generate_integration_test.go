//go:build integration

package bridge_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/artpar/gogent/internal/lifecycle"
	"github.com/artpar/gogent/internal/lifecycle/bridge"
	"github.com/artpar/gogent/internal/lifecycle/definition"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider/google"
)

func setupGoogleProvider(t *testing.T) (*google.Provider, *observe.EventBus) {
	t.Helper()
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		t.Skip("GOOGLE_API_KEY not set")
	}
	bus := observe.NewEventBus(64)
	prov, err := google.New(apiKey, bus)
	if err != nil {
		t.Fatalf("create google provider: %v", err)
	}
	return prov, bus
}

func TestGenerateGraph_SimpleReAct(t *testing.T) {
	prov, bus := setupGoogleProvider(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	def, err := bridge.GenerateGraph(ctx, prov, bus, "gemini-2.5-flash", "call the LLM, if it wants to use tools execute them, then loop back to the LLM until done")
	if err != nil {
		t.Fatalf("GenerateGraph: %v", err)
	}

	// Verify the graph has the expected structure
	if def.Graph.Initial == "" {
		t.Error("graph has no initial node")
	}
	if len(def.Graph.Nodes) < 2 {
		t.Errorf("expected at least 2 nodes (llm + tools), got %d", len(def.Graph.Nodes))
	}

	// Verify it has an llm and tools node type
	hasLLM, hasTools := false, false
	for _, node := range def.Graph.Nodes {
		switch node.Type {
		case "llm":
			hasLLM = true
		case "tools":
			hasTools = true
		}
	}
	if !hasLLM {
		t.Error("graph has no llm node")
	}
	if !hasTools {
		t.Error("graph has no tools node")
	}

	// Verify it has a messages reducer
	if def.Graph.Reducers["messages"] != "messages" {
		t.Errorf("expected messages reducer, got reducers: %v", def.Graph.Reducers)
	}

	// Verify it resolves without error
	noopNode := func(nodeType string, config map[string]any) (lifecycle.NodeFunc, error) {
		return func(ctx context.Context, state lifecycle.State) (lifecycle.StateUpdate, error) {
			return nil, nil
		}, nil
	}
	opts := &definition.ResolveOptions{
		CustomReducers: map[string]lifecycle.ReducerFunc{
			"messages":    bridge.MessageReducer,
			"reflections": bridge.ReflectionReducer,
			"total_usage": bridge.UsageReducer,
		},
	}
	_, err = definition.Resolve(def, noopNode, definition.DefaultRouterCreator(), opts)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
}

func TestGenerateGraph_Reflexion(t *testing.T) {
	prov, bus := setupGoogleProvider(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	def, err := bridge.GenerateGraph(ctx, prov, bus, "gemini-2.5-flash", "attempt the task with tools, evaluate if it succeeded, if it failed reflect on what went wrong and try again")
	if err != nil {
		t.Fatalf("GenerateGraph: %v", err)
	}

	// Should have eval and reflect nodes
	hasEval, hasReflect := false, false
	for _, node := range def.Graph.Nodes {
		switch node.Type {
		case "eval":
			hasEval = true
		case "reflect":
			hasReflect = true
		}
	}
	if !hasEval {
		t.Error("reflexion graph has no eval node")
	}
	if !hasReflect {
		t.Error("reflexion graph has no reflect node")
	}

	// Should have reflections reducer
	if def.Graph.Reducers["reflections"] != "reflections" {
		t.Logf("warning: reflexion graph missing reflections reducer: %v", def.Graph.Reducers)
	}
}

func TestGenerateGraph_PlanExecute(t *testing.T) {
	prov, bus := setupGoogleProvider(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	def, err := bridge.GenerateGraph(ctx, prov, bus, "gemini-2.5-flash", "plan the steps first, then execute each step with tools, then verify the result is correct")
	if err != nil {
		t.Fatalf("GenerateGraph: %v", err)
	}

	// Should have multiple llm nodes (planner + verifier at minimum)
	llmCount := 0
	for _, node := range def.Graph.Nodes {
		if node.Type == "llm" {
			llmCount++
		}
	}
	if llmCount < 2 {
		t.Logf("warning: plan-execute graph has only %d llm node(s), expected at least 2 (planner + verifier)", llmCount)
	}

	if len(def.Graph.Nodes) < 2 {
		t.Errorf("expected at least 2 nodes, got %d", len(def.Graph.Nodes))
	}
}
