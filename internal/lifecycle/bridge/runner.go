package bridge

import (
	"context"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/lifecycle"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

// RunnerConfig is the runtime context all bridge nodes expect in lifecycle state.
type RunnerConfig struct {
	system    model.SystemPrompt
	modelID   string
	maxTokens int
	tools     []model.ToolDef
	bus       *observe.EventBus
}

func NewRunnerConfig(system model.SystemPrompt, modelID string, maxTokens int, tools []model.ToolDef, bus *observe.EventBus) RunnerConfig {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: RunnerConfig{\n\tsystem:\t\tsystem,\n\tmodelID:\tmodelID,\n\tmaxTokens:\tmaxTokens,\n\tto...")
	return RunnerConfig{
		system:    system,
		modelID:   modelID,
		maxTokens: maxTokens,
		tools:     append([]model.ToolDef(nil), tools...),
		bus:       bus,
	}
}

// RunEvent wraps executor progress and attaches a projected result to completion.
type RunEvent struct {
	Event    lifecycle.ExecutionEvent
	Progress ProgressEvent
	Result   RunResult
}

type ProgressEvent struct {
	Step     int
	Node     string
	Nodes    []string
	Status   string
	Duration time.Duration
	Err      error
	Error    string
	FromNode string
	ToNode   string
	RouteKey string
}

// RunResult is the caller-facing projection of a completed lifecycle graph.
type RunResult struct {
	State         lifecycle.State
	Err           error
	AssistantText string
	StopReason    model.StopReason
	Response      model.Response
	Usage         model.TokenUsage
}

// Runner builds lifecycle bridge state, runs the graph, and projects the result.
type Runner struct {
	graph  *lifecycle.Graph
	config RunnerConfig
}

func NewRunner(graph *lifecycle.Graph, config RunnerConfig) *Runner {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &Runner{graph: graph, config: config}")
	return &Runner{graph: graph, config: config}
}

func (r *Runner) InitialState(prompt string) lifecycle.State {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	userMsg := model.Message{
		ID:        model.NewUUID(),
		Role:      model.RoleUser,
		Content:   []model.ContentPart{model.TextPart{Text: prompt}},
		Timestamp: time.Now(),
	}
	observe.GlobalTrace("return: lifecycle.State{\n\tKeyMessages:\t[]model.Message{userMsg},\n\tKeySystem:\tr.config...")
	return lifecycle.State{
		KeyMessages:  []model.Message{userMsg},
		KeySystem:    r.config.system,
		KeyModelID:   r.config.modelID,
		KeyMaxTokens: r.config.maxTokens,
		KeyTools:     append([]model.ToolDef(nil), r.config.tools...),
	}
}

func (r *Runner) Stream(ctx context.Context, prompt string) <-chan RunEvent {
	observe.TraceCtx(ctx, "lifecycle/bridge", "Runner.Stream", "enter")
	defer observe.TraceCtx(ctx, "lifecycle/bridge", "Runner.Stream", "exit")
	ch := make(chan RunEvent, 16)
	go func() {
		defer close(ch)
		executor := lifecycle.NewExecutor(r.graph, lifecycle.WithEventBus(r.config.bus))
		for ev := range executor.Stream(ctx, r.InitialState(prompt)) {
			observe.TraceCtx(ctx, "bridge", "Runner.Stream", "range executor.Stream(ctx, r.InitialState(prompt))")
			runEv := RunEvent{Event: ev, Progress: ProjectProgress(ev)}
			if ev.Type == "completed" {
				observe.TraceCtx(ctx, "bridge", "Runner.Stream", "if: ev.Type == \"completed\"")
				runEv.Result = ProjectResult(ev.State, ev.Err)
			}
			ch <- runEv
		}
	}()
	observe.TraceCtx(ctx, "bridge", "Runner.Stream", "return: ch")
	return ch
}

func ProjectProgress(ev lifecycle.ExecutionEvent) ProgressEvent {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	progress := ProgressEvent{
		Step:     ev.Step,
		Node:     ev.Node,
		Nodes:    append([]string(nil), ev.Nodes...),
		Status:   ev.Type,
		Duration: ev.Duration,
		FromNode: ev.FromNode,
		ToNode:   ev.ToNode,
		RouteKey: ev.RouteKey,
	}
	if ev.Err != nil {
		observe.GlobalTrace("if: ev.Err != nil")
		progress.Err = ev.Err
		progress.Error = ev.Err.Error()
	}
	observe.GlobalTrace("return: progress")
	return progress
}

func ProjectResult(finalState lifecycle.State, runErr error) RunResult {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	result := RunResult{
		State:      finalState,
		Err:        runErr,
		StopReason: model.StopEndTurn,
	}
	result.AssistantText = LastAssistantText(finalState)
	if sr := StopReason(finalState); sr != "" {
		observe.GlobalTrace("if: sr != \"\"")
		result.StopReason = model.StopReason(sr)
	}
	result.Response = Response(finalState)
	result.Usage = TotalUsage(finalState)
	if result.Usage.InputTokens > 0 || result.Usage.OutputTokens > 0 {
		observe.GlobalTrace("if: result.Usage.InputTokens > 0 || result.Usage.OutputTokens > 0")
		result.Response.Usage = result.Usage
	}
	observe.GlobalTrace("return: result")
	return result
}

func LastAssistantText(state lifecycle.State) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	msgs := Messages(state)
	for i := len(msgs) - 1; i >= 0; i-- {
		observe.GlobalTrace("for: i >= 0")
		if msgs[i].Role != model.RoleAssistant {
			observe.GlobalTrace("if: msgs[i].Role != model.RoleAssistant")
			continue
		}
		var parts []string
		for _, part := range msgs[i].Content {
			observe.GlobalTrace("range msgs[i].Content")
			if tp, ok := part.(model.TextPart); ok && tp.Text != "" {
				observe.GlobalTrace("if: ok && tp.Text != \"\"")
				parts = append(parts, tp.Text)
			}
		}
		if len(parts) > 0 {
			observe.GlobalTrace("if: len(parts) > 0")
			observe.GlobalTrace("return: strings.Join(parts, \"\\n\")")
			return strings.Join(parts, "\n")
		}
	}
	observe.GlobalTrace("return: \"\"")
	return ""
}
