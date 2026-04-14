package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/compact"
	"github.com/artpar/gogent/internal/hook"
	"github.com/artpar/gogent/internal/lifecycle"
	"github.com/artpar/gogent/internal/lifecycle/bridge"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
	"github.com/artpar/gogent/internal/tool"
)

// DefaultMaxTurns is the maximum number of agentic loop iterations before
// the engine stops to prevent runaway tool-calling loops.
const DefaultMaxTurns = 100

// EngineConfig holds query engine parameters derived from config + CLI flags.
type EngineConfig struct {
	Model       string
	MaxTokens   int
	MaxTurns    int // 0 means use DefaultMaxTurns
	Temperature *float64
	Thinking    *provider.ThinkingConfig
}

// Engine orchestrates the agentic loop: stream from provider, accumulate response,
// execute tool calls, loop until done.
type Engine struct {
	provider     provider.Provider
	registry     *tool.Registry
	orchestrator *tool.Orchestrator
	store        *app.StateStore
	costTracker  *model.CostTracker
	bus          *observe.EventBus
	config       EngineConfig

	// Compaction — nil means auto-compaction disabled.
	// Subagent engines pass nil (#27794: only root engine auto-compacts).
	compactor    *compact.Service
	autoTracker  *compact.AutoTracker
	windowConfig compact.WindowConfig

	// Hooks — nil means no hook manager configured.
	hookMgr *hook.Manager
}

// CompactionDeps holds optional compaction dependencies.
// Pass nil/zero values to disable auto-compaction (e.g., for subagent engines).
type CompactionDeps struct {
	Compactor    *compact.Service
	AutoTracker  *compact.AutoTracker
	WindowConfig compact.WindowConfig
}

// NewEngine creates an Engine with all dependencies injected.
func NewEngine(
	prov provider.Provider,
	reg *tool.Registry,
	orch *tool.Orchestrator,
	store *app.StateStore,
	ct *model.CostTracker,
	bus *observe.EventBus,
	cfg EngineConfig,
	compDeps ...CompactionDeps,
) *Engine {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	e := &Engine{
		provider:     prov,
		registry:     reg,
		orchestrator: orch,
		store:        store,
		costTracker:  ct,
		bus:          bus,
		config:       cfg,
	}
	if len(compDeps) > 0 {
		observe.GlobalTrace("if: len(compDeps) > 0")
		e.compactor = compDeps[0].Compactor
		e.autoTracker = compDeps[0].AutoTracker
		e.windowConfig = compDeps[0].WindowConfig
	}
	observe.GlobalTrace("return: e")
	return e
}

// SetHookManager configures the hook manager for Stop hooks.
func (e *Engine) SetHookManager(mgr *hook.Manager) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	e.hookMgr = mgr
}

// SetCompaction configures auto-compaction after engine creation.
// Useful when the engine is created before compaction deps are ready.
func (e *Engine) SetCompaction(deps CompactionDeps) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	e.compactor = deps.Compactor
	e.autoTracker = deps.AutoTracker
	e.windowConfig = deps.WindowConfig
}

// Orchestrator returns the engine's tool orchestrator.
func (e *Engine) Orchestrator() *tool.Orchestrator { return e.orchestrator }

// Registry returns the engine's tool registry.
func (e *Engine) Registry() *tool.Registry { return e.registry }

// RunGraph executes a lifecycle graph using the engine's own provider, orchestrator,
// and registry. Returns a channel of LoopEvents, same as Run().
func (e *Engine) RunGraph(ctx context.Context, graph *lifecycle.Graph, prompt string) <-chan LoopEvent {
	observe.TraceCtx(ctx, "query", "Engine.RunGraph", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.RunGraph", "exit")
	ch := make(chan LoopEvent, 16)
	go func() {
		defer close(ch)
		defer func() {
			if r := recover(); r != nil {
				ch <- ErrorEvent{Err: fmt.Errorf("graph execution panic: %v", r)}
			}
		}()
		e.runGraph(ctx, graph, prompt, ch)
	}()
	return ch
}

func (e *Engine) runGraph(ctx context.Context, graph *lifecycle.Graph, prompt string, ch chan<- LoopEvent) {
	observe.TraceCtx(ctx, "query", "Engine.runGraph", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.runGraph", "exit")

	snap := e.store.Snapshot()

	userMsg := model.Message{
		ID:   model.NewUUID(),
		Role: model.RoleUser,
		Content: []model.ContentPart{model.TextPart{Text: prompt}},
	}

	initialState := lifecycle.State{
		bridge.KeyMessages:  []model.Message{userMsg},
		bridge.KeySystem:    snap.Conversation.System,
		bridge.KeyModelID:   e.config.Model,
		bridge.KeyMaxTokens: e.config.MaxTokens,
		bridge.KeyTools:     e.registry.ToolDefs(),
	}

	executor := lifecycle.NewExecutor(graph, lifecycle.WithEventBus(e.bus))
	finalState, err := executor.Run(ctx, initialState)

	if err != nil {
		ch <- ErrorEvent{Err: fmt.Errorf("lifecycle graph: %w", err)}
		return
	}

	// Extract final assistant text
	var resultText strings.Builder
	msgs := bridge.Messages(finalState)
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == model.RoleAssistant {
			for _, part := range msgs[i].Content {
				if tp, ok := part.(model.TextPart); ok && tp.Text != "" {
					resultText.WriteString(tp.Text)
				}
			}
			if resultText.Len() > 0 {
				break
			}
		}
	}

	if resultText.Len() > 0 {
		ch <- TextEvent{Text: resultText.String()}
	}

	stopReason := model.StopEndTurn
	if sr := bridge.StopReason(finalState); sr != "" {
		stopReason = model.StopReason(sr)
	}
	var resp model.Response
	if r, ok := finalState[bridge.KeyResponse].(model.Response); ok {
		resp = r
	}
	// Use accumulated total usage from all LLM calls, not just the last response
	totalUsage := bridge.TotalUsage(finalState)
	if totalUsage.InputTokens > 0 || totalUsage.OutputTokens > 0 {
		resp.Usage = totalUsage
	}
	ch <- TurnCompleteEvent{Response: resp, StopReason: stopReason}
}
