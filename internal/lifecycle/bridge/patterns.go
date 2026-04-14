package bridge

import (
	"fmt"

	"github.com/artpar/gogent/internal/lifecycle"
	"github.com/artpar/gogent/internal/observe"
)

// mustBuild calls Build() and panics on error. Used by pattern constructors
// whose graph structure is known-correct at compile time.
func mustBuild(b *lifecycle.Builder) *lifecycle.Graph {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	g, err := b.Build()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		panic(fmt.Sprintf("lifecycle/bridge: pattern build failed: %v", err))
	}
	observe.GlobalTrace("return: g")
	return g
}

// NewReAct builds a ReAct graph wired to real infrastructure.
// LLMNode calls the provider, ToolNode executes tool calls, loop until end_turn.
func NewReAct(infra Infra) *lifecycle.Graph {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: mustBuild(lifecycle.NewBuilder().\n\tAddNode(\"llm\", LLMNode(infra.Provider, inf...")
	return mustBuild(lifecycle.NewBuilder().
		AddNode("llm", LLMNode(infra.Provider, infra.Bus, LLMNodeConfig{})).
		AddNode("tools", ToolNode(infra.Orchestrator, infra.Cwd)).
		SetInitialNode("llm").
		SetReducer(KeyMessages, MessageReducer).
		SetReducer(KeyTurnCount, lifecycle.ReducerSum).
		SetReducer(KeyTotalUsage, UsageReducer).
		AddConditionalEdges("llm", StopReasonRouter(), map[string]string{
			"continue": "tools",
			"end":      "",
		}).
		AddEdge("tools", "llm"))
}

// NewPlanExecute builds a Plan-and-Execute graph wired to real infrastructure.
// Planner generates a plan, executor runs tools, replanner evaluates and may loop.
func NewPlanExecute(infra Infra) *lifecycle.Graph {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	planFn := LLMNode(infra.Provider, infra.Bus, LLMNodeConfig{
		SystemOverride: "You are a planning agent. Given the task, create a detailed step-by-step plan. " +
			"Output the plan as a numbered list.",
	})
	executeFn := ToolNode(infra.Orchestrator, infra.Cwd)
	replanFn := LLMNode(infra.Provider, infra.Bus, LLMNodeConfig{
		SystemOverride: "You are a replanning agent. Review the execution results so far. " +
			"If the task is complete, respond with a summary (no tool calls). " +
			"If not, call tools to continue execution.",
	})
	observe.GlobalTrace("return: mustBuild(lifecycle.NewBuilder().\n\tAddNode(\"planner\", planFn).\n\tAddNode(\"exec...")

	return mustBuild(lifecycle.NewBuilder().
		AddNode("planner", planFn).
		AddNode("executor", executeFn).
		AddNode("replanner", replanFn).
		SetInitialNode("planner").
		SetReducer(KeyMessages, MessageReducer).
		SetReducer(KeyTurnCount, lifecycle.ReducerSum).
		SetReducer(KeyTotalUsage, UsageReducer).
		AddEdge("planner", "executor").
		AddEdge("executor", "replanner").
		AddConditionalEdges("replanner", StopReasonRouter(), map[string]string{
			"continue": "executor",
			"end":      "",
		}))
}

// NewReflexion builds a Reflexion graph wired to real infrastructure.
// Actor attempts the task, evaluator judges, reflector critiques on failure.
func NewReflexion(infra Infra, maxTrials int) *lifecycle.Graph {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	actorFn := LLMNode(infra.Provider, infra.Bus, LLMNodeConfig{})
	evaluatorFn := EvalNode(infra.Provider, infra.Bus, EvalNodeConfig{
		Criteria: "Did the actor successfully complete the requested task?",
	})
	reflectFn := ReflectNode(infra.Provider, infra.Bus)
	observe.GlobalTrace("return: mustBuild(lifecycle.NewBuilder().\n\tAddNode(\"actor\", actorFn).\n\tAddNode(\"evalu...")

	return mustBuild(lifecycle.NewBuilder().
		AddNode("actor", actorFn).
		AddNode("evaluator", evaluatorFn).
		AddNode("reflector", reflectFn).
		SetInitialNode("actor").
		SetReducer(KeyMessages, MessageReducer).
		SetReducer(KeyReflections, ReflectionReducer).
		SetReducer(KeyTurnCount, lifecycle.ReducerSum).
		SetReducer(KeyTotalUsage, UsageReducer).
		SetMaxSteps(maxTrials*4+5).
		AddEdge("actor", "evaluator").
		AddConditionalEdges("evaluator", func(s lifecycle.State) string {
			if Passed(s) {
				return "pass"
			}
			return "fail"
		}, map[string]string{
			"pass": "",
			"fail": "reflector",
		}).
		AddEdge("reflector", "actor"))
}

// PatternMap returns all available named patterns backed by real infrastructure.
func PatternMap(infra Infra) map[string]*lifecycle.Graph {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: map[string]*lifecycle.Graph{\n\t\"react\":\tNewReAct(infra),\n\t\"plan-execute\":\tNewP...")
	return map[string]*lifecycle.Graph{
		"react":        NewReAct(infra),
		"plan-execute": NewPlanExecute(infra),
		"reflexion":    NewReflexion(infra, 3),
	}
}

// PatternDescriptions returns human-readable descriptions for built-in patterns.
func PatternDescriptions() map[string]string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: map[string]string{\n\t\"react\":\t\"ReAct (Think-Act-Observe) — LLM calls tools i...")
	return map[string]string{
		"react":        "ReAct (Think-Act-Observe) — LLM calls tools in a loop until done",
		"plan-execute": "Plan-and-Execute — plan steps, execute with tools, replan if needed",
		"reflexion":    "Reflexion — attempt, evaluate, reflect on failures, retry",
	}
}
