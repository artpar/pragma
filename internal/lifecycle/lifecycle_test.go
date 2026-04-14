package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// --- Test helpers ---

func nodeReturning(update StateUpdate) NodeFunc {
	return func(_ context.Context, _ State) (StateUpdate, error) {
		return update, nil
	}
}


// --- Graph builder tests ---

func TestBuildValidation(t *testing.T) {
	tests := []struct {
		name    string
		build   func() (*Graph, error)
		wantErr string
	}{
		{
			name: "missing initial node",
			build: func() (*Graph, error) {
				return NewBuilder().AddNode("a", nodeReturning(nil)).Build()
			},
			wantErr: "initial node not set",
		},
		{
			name: "initial node not registered",
			build: func() (*Graph, error) {
				return NewBuilder().SetInitialNode("missing").Build()
			},
			wantErr: "not registered",
		},
		{
			name: "edge target not registered",
			build: func() (*Graph, error) {
				return NewBuilder().
					AddNode("a", nodeReturning(nil)).
					SetInitialNode("a").
					AddEdge("a", "missing").
					Build()
			},
			wantErr: "not registered",
		},
		{
			name: "conditional + static edges ambiguous",
			build: func() (*Graph, error) {
				return NewBuilder().
					AddNode("a", nodeReturning(nil)).
					AddNode("b", nodeReturning(nil)).
					AddNode("c", nodeReturning(nil)).
					SetInitialNode("a").
					AddEdge("a", "b").
					AddConditionalEdges("a", func(State) string { return "x" }, map[string]string{"x": "c"}).
					Build()
			},
			wantErr: "ambiguous",
		},
		{
			name: "valid simple graph",
			build: func() (*Graph, error) {
				return NewBuilder().
					AddNode("a", nodeReturning(nil)).
					SetInitialNode("a").
					Build()
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.build()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Error("expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
			}
		})
	}
}

// --- State and reducer tests ---

func TestStateSnapshot(t *testing.T) {
	s := State{"a": 1, "b": "hello"}
	cp := s.Snapshot()
	cp["a"] = 99
	if s["a"] != 1 {
		t.Error("snapshot mutation leaked to original")
	}
}

func TestApplyUpdate(t *testing.T) {
	state := State{"x": 1, "y": "keep"}
	update := StateUpdate{"x": 2, "z": "new"}
	result := applyUpdate(state, update, nil)

	if result["x"] != 2 {
		t.Errorf("expected x=2, got %v", result["x"])
	}
	if result["y"] != "keep" {
		t.Errorf("expected y=keep, got %v", result["y"])
	}
	if result["z"] != "new" {
		t.Errorf("expected z=new, got %v", result["z"])
	}
	// Original unchanged
	if state["x"] != 1 {
		t.Error("original state mutated")
	}
}

func TestApplyUpdateNilDeletes(t *testing.T) {
	state := State{"a": 1, "b": 2}
	update := StateUpdate{"a": nil}
	result := applyUpdate(state, update, nil)
	if _, ok := result["a"]; ok {
		t.Error("expected key 'a' to be deleted")
	}
	if result["b"] != 2 {
		t.Error("expected 'b' to be preserved")
	}
}

func TestReducerAppendList(t *testing.T) {
	existing := []any{"a", "b"}
	incoming := []any{"c"}
	result := ReducerAppendList(existing, incoming).([]any)
	if len(result) != 3 || result[2] != "c" {
		t.Errorf("expected [a b c], got %v", result)
	}
}

func TestReducerMergeMap(t *testing.T) {
	existing := map[string]any{"a": 1}
	incoming := map[string]any{"b": 2}
	result := ReducerMergeMap(existing, incoming).(map[string]any)
	if result["a"] != 1 || result["b"] != 2 {
		t.Errorf("expected {a:1, b:2}, got %v", result)
	}
}

func TestReducerSum(t *testing.T) {
	if ReducerSum(3, 4) != 7 {
		t.Error("int sum failed")
	}
	if ReducerSum(1.5, 2.5) != 4.0 {
		t.Error("float sum failed")
	}
}

// --- Executor tests ---

func TestSimpleLinear(t *testing.T) {
	g, err := NewBuilder().
		AddNode("a", nodeReturning(StateUpdate{"x": 1})).
		AddNode("b", nodeReturning(StateUpdate{"y": 2})).
		AddNode("c", nodeReturning(StateUpdate{"z": 3})).
		SetInitialNode("a").
		AddEdge("a", "b").
		AddEdge("b", "c").
		Build()
	if err != nil {
		t.Fatal(err)
	}

	result, err := NewExecutor(g).Run(context.Background(), State{})
	if err != nil {
		t.Fatal(err)
	}
	if result["x"] != 1 || result["y"] != 2 || result["z"] != 3 {
		t.Errorf("expected {x:1,y:2,z:3}, got %v", result)
	}
}

func TestConditionalRouting(t *testing.T) {
	g, err := NewBuilder().
		AddNode("start", nodeReturning(StateUpdate{"path": "left"})).
		AddNode("left", nodeReturning(StateUpdate{"result": "went left"})).
		AddNode("right", nodeReturning(StateUpdate{"result": "went right"})).
		SetInitialNode("start").
		AddConditionalEdges("start", func(s State) string {
			return s["path"].(string)
		}, map[string]string{
			"left":  "left",
			"right": "right",
		}).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	result, err := NewExecutor(g).Run(context.Background(), State{})
	if err != nil {
		t.Fatal(err)
	}
	if result["result"] != "went left" {
		t.Errorf("expected 'went left', got %v", result["result"])
	}
}

func TestCycleReAct(t *testing.T) {
	callCount := 0
	llmFn := func(_ context.Context, s State) (StateUpdate, error) {
		callCount++
		if callCount >= 3 {
			return StateUpdate{"stop_reason": "end_turn", "result": "done"}, nil
		}
		return StateUpdate{"stop_reason": "tool_use"}, nil
	}
	toolFn := func(_ context.Context, s State) (StateUpdate, error) {
		return StateUpdate{"tool_result": fmt.Sprintf("result_%d", callCount)}, nil
	}

	g := NewReActGraph(llmFn, toolFn)
	result, err := NewExecutor(g).Run(context.Background(), State{})
	if err != nil {
		t.Fatal(err)
	}
	if result["result"] != "done" {
		t.Errorf("expected 'done', got %v", result["result"])
	}
	if callCount != 3 {
		t.Errorf("expected 3 LLM calls, got %d", callCount)
	}
}

func TestFanOutFanIn(t *testing.T) {
	var counterA, counterB int32

	// Build manually with reducer so parallel results merge correctly
	b := NewBuilder()
	b.AddNode("_dispatch", func(_ context.Context, _ State) (StateUpdate, error) {
		return nil, nil
	})
	b.AddNode("worker_a", func(_ context.Context, _ State) (StateUpdate, error) {
		atomic.AddInt32(&counterA, 1)
		return StateUpdate{"results": []any{"from_a"}}, nil
	})
	b.AddNode("worker_b", func(_ context.Context, _ State) (StateUpdate, error) {
		atomic.AddInt32(&counterB, 1)
		return StateUpdate{"results": []any{"from_b"}}, nil
	})
	b.AddNode("merge", func(_ context.Context, s State) (StateUpdate, error) {
		return StateUpdate{"merged": s["results"]}, nil
	})
	b.SetInitialNode("_dispatch")
	b.AddEdge("_dispatch", "worker_a")
	b.AddEdge("_dispatch", "worker_b")
	b.AddEdge("worker_a", "merge")
	b.AddEdge("worker_b", "merge")
	b.SetReducer("results", ReducerAppendList) // critical: merge parallel results
	g, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}

	result, err := NewExecutor(g).Run(context.Background(), State{
		"results": []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if counterA != 1 || counterB != 1 {
		t.Errorf("expected both workers called once, got a=%d b=%d", counterA, counterB)
	}
	// Verify BOTH results are present after parallel merge
	merged, ok := result["merged"].([]any)
	if !ok {
		t.Fatalf("expected merged to be []any, got %T", result["merged"])
	}
	if len(merged) != 2 {
		t.Errorf("expected 2 merged results, got %d: %v", len(merged), merged)
	}
	// Both "from_a" and "from_b" should be present (order may vary)
	has := map[string]bool{}
	for _, v := range merged {
		has[v.(string)] = true
	}
	if !has["from_a"] || !has["from_b"] {
		t.Errorf("expected both from_a and from_b in merged, got %v", merged)
	}
}

func TestMADDebate(t *testing.T) {
	var mu sync.Mutex
	agentCallCounts := map[string]int{}
	agents := map[string]NodeFunc{
		"bull": func(_ context.Context, s State) (StateUpdate, error) {
			mu.Lock()
			agentCallCounts["bull"]++
			count := agentCallCounts["bull"]
			mu.Unlock()
			return StateUpdate{
				"positions": map[string]any{"bull": fmt.Sprintf("bullish_round_%d", count)},
			}, nil
		},
		"bear": func(_ context.Context, s State) (StateUpdate, error) {
			mu.Lock()
			agentCallCounts["bear"]++
			count := agentCallCounts["bear"]
			mu.Unlock()
			return StateUpdate{
				"positions": map[string]any{"bear": fmt.Sprintf("bearish_round_%d", count)},
			}, nil
		},
	}
	synthesizeFn := func(_ context.Context, s State) (StateUpdate, error) {
		positions := s["positions"]
		return StateUpdate{"result": fmt.Sprintf("synthesis of %v", positions)}, nil
	}

	g := NewMADGraph(agents, 2, synthesizeFn)
	result, err := NewExecutor(g).Run(context.Background(), State{
		"round":     0,
		"positions": map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result["result"] == nil {
		t.Error("expected synthesis result")
	}
	// Each agent should be called once per round = 2 times
	if agentCallCounts["bull"] != 2 || agentCallCounts["bear"] != 2 {
		t.Errorf("expected 2 calls each, got bull=%d bear=%d",
			agentCallCounts["bull"], agentCallCounts["bear"])
	}
}

func TestPlanAndExecute(t *testing.T) {
	planFn := func(_ context.Context, _ State) (StateUpdate, error) {
		return StateUpdate{
			"plan":         []string{"step1", "step2"},
			"current_step": 0,
		}, nil
	}
	executeFn := func(_ context.Context, s State) (StateUpdate, error) {
		step := s["current_step"].(int)
		plan := s["plan"].([]string)
		result := fmt.Sprintf("executed_%s", plan[step])
		return StateUpdate{
			"past_steps":   []any{result},
			"current_step": step + 1,
		}, nil
	}
	replanFn := func(_ context.Context, s State) (StateUpdate, error) {
		step := s["current_step"].(int)
		plan := s["plan"].([]string)
		if step >= len(plan) {
			return StateUpdate{"done": true, "result": "all done"}, nil
		}
		return StateUpdate{"done": false}, nil
	}

	g := NewPlanExecuteGraph(planFn, executeFn, replanFn)
	result, err := NewExecutor(g).Run(context.Background(), State{
		"past_steps": []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result["result"] != "all done" {
		t.Errorf("expected 'all done', got %v", result["result"])
	}
}

func TestReflexion(t *testing.T) {
	trial := 0
	actorFn := func(_ context.Context, s State) (StateUpdate, error) {
		trial++
		return StateUpdate{"trial_result": fmt.Sprintf("attempt_%d", trial), "trial": trial}, nil
	}
	evaluatorFn := func(_ context.Context, s State) (StateUpdate, error) {
		t := s["trial"].(int)
		passed := t >= 3 // pass on third attempt
		return StateUpdate{"passed": passed}, nil
	}
	reflectFn := func(_ context.Context, s State) (StateUpdate, error) {
		t := s["trial"].(int)
		return StateUpdate{
			"reflections": []any{fmt.Sprintf("reflection_after_trial_%d", t)},
		}, nil
	}

	g := NewReflexionGraph(actorFn, evaluatorFn, reflectFn, 5)
	result, err := NewExecutor(g).Run(context.Background(), State{
		"reflections": []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result["passed"] != true {
		t.Error("expected passed=true")
	}
	if trial != 3 {
		t.Errorf("expected 3 trials, got %d", trial)
	}
}

func TestMDAP(t *testing.T) {
	sampleCount := 0
	sampleFn := func(_ context.Context, s State) (StateUpdate, error) {
		sampleCount++
		step, _ := s["step"].(int)
		return StateUpdate{
			"candidate_action": fmt.Sprintf("move_%d", step),
			"candidate_state":  fmt.Sprintf("state_%d", step+1),
			"valid":            true,
		}, nil
	}
	voteFn := func(_ context.Context, s State) (StateUpdate, error) {
		// Simplified: always reach margin immediately
		return StateUpdate{
			"margin_reached": true,
			"winning_action": s["candidate_action"],
			"winning_state":  s["candidate_state"],
		}, nil
	}
	advanceFn := func(_ context.Context, s State) (StateUpdate, error) {
		step, _ := s["step"].(int)
		actions, _ := s["action_list"].([]string)
		actions = append(actions, s["winning_action"].(string))
		return StateUpdate{
			"task_state":   s["winning_state"],
			"step":         step + 1,
			"action_list":  actions,
			"votes":        map[string]any{}, // reset
			"valid":        false,
			"margin_reached": false,
		}, nil
	}

	g := NewMDAP(sampleFn, voteFn, advanceFn)
	result, err := NewExecutor(g).Run(context.Background(), State{
		"task_state":   "initial",
		"step":         0,
		"total_steps":  3,
		"k":            1,
		"action_list":  []string{},
		"votes":        map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	step, _ := result["step"].(int)
	if step != 3 {
		t.Errorf("expected step=3, got %d", step)
	}
	actions, _ := result["action_list"].([]string)
	if len(actions) != 3 {
		t.Errorf("expected 3 actions, got %d: %v", len(actions), actions)
	}
}

func TestMaxStepsExceeded(t *testing.T) {
	g, _ := NewBuilder().
		AddNode("loop", nodeReturning(StateUpdate{"x": 1})).
		SetInitialNode("loop").
		AddEdge("loop", "loop"). // infinite self-loop
		SetMaxSteps(5).
		Build()

	_, err := NewExecutor(g).Run(context.Background(), State{})
	if !errors.Is(err, ErrMaxStepsExceeded) {
		t.Errorf("expected ErrMaxStepsExceeded, got %v", err)
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	g, _ := NewBuilder().
		AddNode("a", nodeReturning(StateUpdate{"x": 1})).
		SetInitialNode("a").
		Build()

	_, err := NewExecutor(g).Run(ctx, State{})
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestReducerMergeInParallel(t *testing.T) {
	g, _ := NewBuilder().
		AddNode("dispatch", nodeReturning(nil)).
		AddNode("a", nodeReturning(StateUpdate{"items": []any{"from_a"}})).
		AddNode("b", nodeReturning(StateUpdate{"items": []any{"from_b"}})).
		AddNode("collect", func(_ context.Context, s State) (StateUpdate, error) {
			return StateUpdate{"count": len(s["items"].([]any))}, nil
		}).
		SetInitialNode("dispatch").
		AddEdge("dispatch", "a").
		AddEdge("dispatch", "b").
		AddEdge("a", "collect").
		AddEdge("b", "collect").
		SetReducer("items", ReducerAppendList).
		Build()

	result, err := NewExecutor(g).Run(context.Background(), State{"items": []any{}})
	if err != nil {
		t.Fatal(err)
	}
	if result["count"] != 2 {
		t.Errorf("expected count=2, got %v", result["count"])
	}
}

func TestCheckpointing(t *testing.T) {
	cp := NewMemoryCheckpointer()
	g, _ := NewBuilder().
		AddNode("a", nodeReturning(StateUpdate{"step": "a_done"})).
		AddNode("b", nodeReturning(StateUpdate{"step": "b_done"})).
		SetInitialNode("a").
		AddEdge("a", "b").
		Build()

	_, err := NewExecutor(g, WithCheckpointer(cp)).Run(context.Background(), State{})
	if err != nil {
		t.Fatal(err)
	}

	step, state, err := cp.Latest()
	if err != nil {
		t.Fatal(err)
	}
	if step != 2 { // 2 supersteps (a, then b)
		t.Errorf("expected 2 checkpoints, latest step=%d", step)
	}
	if state["step"] != "b_done" {
		t.Errorf("expected step=b_done, got %v", state["step"])
	}

	// Load specific checkpoint
	state1, err := cp.Load(1)
	if err != nil {
		t.Fatal(err)
	}
	if state1["step"] != "a_done" {
		t.Errorf("expected step=a_done at checkpoint 1, got %v", state1["step"])
	}
}

func TestPipeline(t *testing.T) {
	g := NewPipelineGraph([]NamedNode{
		{"step1", nodeReturning(StateUpdate{"v": 1})},
		{"step2", func(_ context.Context, s State) (StateUpdate, error) {
			return StateUpdate{"v": s["v"].(int) + 10}, nil
		}},
		{"step3", func(_ context.Context, s State) (StateUpdate, error) {
			return StateUpdate{"v": s["v"].(int) * 2}, nil
		}},
	})

	result, err := NewExecutor(g).Run(context.Background(), State{})
	if err != nil {
		t.Fatal(err)
	}
	if result["v"] != 22 { // (1 + 10) * 2
		t.Errorf("expected v=22, got %v", result["v"])
	}
}

func TestStream(t *testing.T) {
	g, _ := NewBuilder().
		AddNode("a", nodeReturning(StateUpdate{"x": 1})).
		AddNode("b", nodeReturning(StateUpdate{"y": 2})).
		SetInitialNode("a").
		AddEdge("a", "b").
		Build()

	events := NewExecutor(g).Stream(context.Background(), State{})
	var completed bool
	eventCount := 0
	for ev := range events {
		eventCount++
		if ev.Type == "completed" {
			completed = true
			if ev.Err != nil {
				t.Errorf("unexpected error in stream: %v", ev.Err)
			}
		}
	}
	if !completed {
		t.Error("expected completed event")
	}
	if eventCount < 3 { // at least: step_started, step_started, completed
		t.Errorf("expected at least 3 events, got %d", eventCount)
	}
}

