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
	System    model.SystemPrompt
	ModelID   string
	MaxTokens int
	Tools     []model.ToolDef
	Bus       *observe.EventBus
}

// RunEvent wraps executor progress and attaches a projected result to completion.
type RunEvent struct {
	Event  lifecycle.ExecutionEvent
	Result RunResult
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
	return lifecycle.State{
		KeyMessages:  []model.Message{userMsg},
		KeySystem:    r.config.System,
		KeyModelID:   r.config.ModelID,
		KeyMaxTokens: r.config.MaxTokens,
		KeyTools:     append([]model.ToolDef(nil), r.config.Tools...),
	}
}

func (r *Runner) Stream(ctx context.Context, prompt string) <-chan RunEvent {
	observe.TraceCtx(ctx, "lifecycle/bridge", "Runner.Stream", "enter")
	defer observe.TraceCtx(ctx, "lifecycle/bridge", "Runner.Stream", "exit")
	ch := make(chan RunEvent, 16)
	go func() {
		defer close(ch)
		executor := lifecycle.NewExecutor(r.graph, lifecycle.WithEventBus(r.config.Bus))
		for ev := range executor.Stream(ctx, r.InitialState(prompt)) {
			runEv := RunEvent{Event: ev}
			if ev.Type == "completed" {
				runEv.Result = ProjectResult(ev.State, ev.Err)
			}
			ch <- runEv
		}
	}()
	return ch
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
		result.StopReason = model.StopReason(sr)
	}
	result.Response = Response(finalState)
	result.Usage = TotalUsage(finalState)
	if result.Usage.InputTokens > 0 || result.Usage.OutputTokens > 0 {
		result.Response.Usage = result.Usage
	}
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
			return strings.Join(parts, "\n")
		}
	}
	observe.GlobalTrace("return: \"\"")
	return ""
}
