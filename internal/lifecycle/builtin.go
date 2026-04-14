package lifecycle

import (
	"context"
	"fmt"
	"github.com/artpar/gogent/internal/observe"
)

// NamedNode pairs a name with a NodeFunc for pipeline construction.
type NamedNode struct {
	Name string
	Fn   NodeFunc
}

// mustBuild calls Build() and panics on error.
// Used by builtin pattern constructors whose graph structure is known-correct.
func mustBuild(b *Builder) *Graph {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	g, err := b.Build()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		panic(fmt.Sprintf("lifecycle: builtin graph build failed: %v", err))
	}
	observe.GlobalTrace("return: g")
	return g
}

// NewReActGraph creates a ReAct (think-act-observe) loop.
// llmFn calls the LLM; toolFn executes tool calls.
// State must contain "stop_reason" after llmFn: "tool_use" continues, anything else ends.
func NewReActGraph(llmFn, toolFn NodeFunc) *Graph {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: mustBuild(NewBuilder().\n\tAddNode(\"llm\", llmFn).\n\tAddNode(\"tools\", toolFn).\n\tS...")
	return mustBuild(NewBuilder().
		AddNode("llm", llmFn).
		AddNode("tools", toolFn).
		SetInitialNode("llm").
		AddConditionalEdges("llm", func(s State) string {
			if reason, _ := s["stop_reason"].(string); reason == "tool_use" {
				return "continue"
			}
			return "end"
		}, map[string]string{
			"continue": "tools",
			"end":      "",
		}).
		AddEdge("tools", "llm"))
}

// NewMADGraph creates a Multi-Agent Debate graph.
// Agents debate in parallel for `rounds` rounds, then synthesize.
// State keys: "round" (int), "max_rounds" (int), "positions" (map[string]any).
func NewMADGraph(agents map[string]NodeFunc, rounds int, synthesizeFn NodeFunc) *Graph {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	b := NewBuilder().
		SetMaxSteps(rounds*2+10).
		SetReducer("positions", ReducerMergeMap)

	agentNames := make([]string, 0, len(agents))
	for name, fn := range agents {
		observe.GlobalTrace("range agents")
		b.AddNode(name, fn)
		agentNames = append(agentNames, name)
	}

	b.AddNode("_round_inc", func(_ context.Context, s State) (StateUpdate, error) {
		round, _ := s["round"].(int)
		return StateUpdate{"round": round + 1}, nil
	})

	b.AddNode("synthesize", synthesizeFn)

	b.AddNode("_dispatch", func(_ context.Context, _ State) (StateUpdate, error) {
		return nil, nil
	})
	b.SetInitialNode("_dispatch")

	for _, name := range agentNames {
		observe.GlobalTrace("range agentNames")
		b.AddEdge("_dispatch", name)
	}

	for _, name := range agentNames {
		observe.GlobalTrace("range agentNames")
		b.AddEdge(name, "_round_inc")
	}

	b.AddConditionalEdges("_round_inc", func(s State) string {
		round, _ := s["round"].(int)
		maxRounds := rounds
		if round >= maxRounds {
			return "synthesize"
		}
		return "dispatch"
	}, map[string]string{
		"synthesize": "synthesize",
		"dispatch":   "_dispatch",
	})
	observe.GlobalTrace("return: mustBuild(b)")

	return mustBuild(b)
}

// NewPlanExecuteGraph creates a Plan-and-Execute graph.
// planFn generates plan, executeFn executes one step, replanFn reviews.
// State keys: "plan" ([]string), "current_step" (int), "past_steps" ([]any), "done" (bool).
func NewPlanExecuteGraph(planFn, executeFn, replanFn NodeFunc) *Graph {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: mustBuild(NewBuilder().\n\tAddNode(\"planner\", planFn).\n\tAddNode(\"executor\", exe...")
	return mustBuild(NewBuilder().
		AddNode("planner", planFn).
		AddNode("executor", executeFn).
		AddNode("replanner", replanFn).
		SetInitialNode("planner").
		AddEdge("planner", "executor").
		AddEdge("executor", "replanner").
		AddConditionalEdges("replanner", func(s State) string {
			if done, _ := s["done"].(bool); done {
				return "end"
			}
			return "continue"
		}, map[string]string{
			"end":      "",
			"continue": "executor",
		}).
		SetReducer("past_steps", ReducerAppendList))
}

// NewReflexionGraph creates a Reflexion (trial-reflect-retry) graph.
// State keys: "trial" (int), "passed" (bool), "reflections" ([]any).
func NewReflexionGraph(actorFn, evaluatorFn, reflectFn NodeFunc, maxTrials int) *Graph {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: mustBuild(NewBuilder().\n\tAddNode(\"actor\", actorFn).\n\tAddNode(\"evaluator\", eva...")
	return mustBuild(NewBuilder().
		AddNode("actor", actorFn).
		AddNode("evaluator", evaluatorFn).
		AddNode("reflector", reflectFn).
		SetInitialNode("actor").
		AddEdge("actor", "evaluator").
		AddConditionalEdges("evaluator", func(s State) string {
			if passed, _ := s["passed"].(bool); passed {
				return "end"
			}
			trial, _ := s["trial"].(int)
			if trial >= maxTrials {
				return "end"
			}
			return "reflect"
		}, map[string]string{
			"end":     "",
			"reflect": "reflector",
		}).
		AddEdge("reflector", "actor").
		SetReducer("reflections", ReducerAppendList))
}

// NewPipelineGraph creates a sequential pipeline.
// Steps execute in order: steps[0] → steps[1] → ... → steps[N-1] → END.
func NewPipelineGraph(steps []NamedNode) *Graph {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	b := NewBuilder()
	for _, step := range steps {
		observe.GlobalTrace("range steps")
		b.AddNode(step.Name, step.Fn)
	}
	if len(steps) > 0 {
		observe.GlobalTrace("if: len(steps) > 0")
		b.SetInitialNode(steps[0].Name)
	}
	for i := 0; i < len(steps)-1; i++ {
		observe.GlobalTrace("for: i < len(steps)-1")
		b.AddEdge(steps[i].Name, steps[i+1].Name)
	}
	observe.GlobalTrace("return: mustBuild(b)")
	return mustBuild(b)
}

// NewFanOutFanInGraph creates a parallel execution graph.
// All parallel nodes execute concurrently, then mergeFn aggregates.
// Callers should set appropriate reducers on keys updated by parallel nodes.
func NewFanOutFanInGraph(parallel map[string]NodeFunc, mergeFn NodeFunc) *Graph {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	b := NewBuilder()

	b.AddNode("_dispatch", func(_ context.Context, _ State) (StateUpdate, error) {
		return nil, nil
	})
	b.SetInitialNode("_dispatch")
	b.AddNode("merge", mergeFn)

	for name, fn := range parallel {
		observe.GlobalTrace("range parallel")
		b.AddNode(name, fn)
		b.AddEdge("_dispatch", name)
		b.AddEdge(name, "merge")
	}
	observe.GlobalTrace("return: mustBuild(b)")

	return mustBuild(b)
}

// NewMDAP creates an MDAP/MAKER graph (Meyerson et al., 2025).
// Sequential pipeline with voting error correction at each step.
//
// sampleFn: calls LLM to generate (action, next_state) from current task_state.
//
//	Should set "candidate_action", "candidate_state", "valid" (bool) in state.
//
// voteFn: accumulates candidate into votes, checks margin.
//
//	Should set "margin_reached" (bool), "winning_action", "winning_state".
//
// advanceFn: applies winning action, advances step counter.
//
//	Should update "task_state", increment "step", append to "action_list", reset "votes".
//
// State keys: "task_state" (any), "step" (int), "total_steps" (int),
//
//	"k" (int), "action_list" ([]any), "votes" (map[string]any),
//	"valid" (bool), "margin_reached" (bool).
func NewMDAP(sampleFn, voteFn, advanceFn NodeFunc) *Graph {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: mustBuild(NewBuilder().\n\tAddNode(\"sample\", sampleFn).\n\tAddNode(\"vote\", voteFn...")
	return mustBuild(NewBuilder().
		AddNode("sample", sampleFn).
		AddNode("vote", voteFn).
		AddNode("advance", advanceFn).
		SetInitialNode("sample").
		SetMaxSteps(1_000_000).
		AddConditionalEdges("sample", func(s State) string {
			if valid, _ := s["valid"].(bool); valid {
				return "vote"
			}
			return "resample"
		}, map[string]string{
			"vote":     "vote",
			"resample": "sample",
		}).
		AddConditionalEdges("vote", func(s State) string {
			if reached, _ := s["margin_reached"].(bool); reached {
				return "advance"
			}
			return "more"
		}, map[string]string{
			"advance": "advance",
			"more":    "sample",
		}).
		AddConditionalEdges("advance", func(s State) string {
			step, _ := s["step"].(int)
			total, _ := s["total_steps"].(int)
			if step >= total {
				return "done"
			}
			return "next"
		}, map[string]string{
			"done": "",
			"next": "sample",
		}))
}
