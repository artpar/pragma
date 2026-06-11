package query

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/compact"
	"github.com/artpar/pragma/internal/hook"
	"github.com/artpar/pragma/internal/lifecycle"
	"github.com/artpar/pragma/internal/lifecycle/bridge"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/toolresult"
)

// DefaultMaxTurns is the maximum number of agentic loop iterations before
// the engine stops to prevent runaway tool-calling loops.
const DefaultMaxTurns = 100

// EngineConfig holds query engine parameters derived from config + CLI flags.
type EngineConfig struct {
	Model                     string
	MaxTokens                 int
	MaxTurns                  int // 0 means use DefaultMaxTurns
	Temperature               *float64
	Thinking                  *provider.ThinkingConfig
	ResponseSchema            json.RawMessage
	ContentReplacementRecords []model.ContentReplacementRecord
	RecordContentReplacements func([]model.ContentReplacementRecord) error
	SessionCheckpoint         func() error
	MCPServerStatuses         func() []MCPServerStatus
	RefreshCapabilities       func(context.Context)
}

// MCPServerStatus mirrors the session-init MCP server metadata shape.
type MCPServerStatus struct {
	Name   string
	Status string
}

// Engine orchestrates the agentic loop: stream from provider, accumulate response,
// execute tool calls, loop until done.
type Engine struct {
	provider    provider.Provider
	store       *app.StateStore
	costTracker *model.CostTracker
	bus         *observe.EventBus
	config      EngineConfig

	// Compaction — nil means auto-compaction disabled.
	// Subagent engines pass nil (#27794: only root engine auto-compacts).
	compactor    *compact.Service
	autoTracker  *compact.AutoTracker
	windowConfig compact.WindowConfig

	// Hooks — nil means no hook manager configured.
	hookMgr *hook.Manager

	contentReplacementState *toolresult.ContentReplacementState
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
	store *app.StateStore,
	ct *model.CostTracker,
	bus *observe.EventBus,
	cfg EngineConfig,
	compDeps ...CompactionDeps,
) *Engine {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	prov = provider.WithAccounting(prov, ct, bus)
	e := &Engine{
		provider:    prov,
		store:       store,
		costTracker: ct,
		bus:         bus,
		config:      cfg,
	}
	snap := store.Snapshot()
	e.contentReplacementState = toolresult.ReconstructContentReplacementState(snap.Conversation.APIMessages(), cfg.ContentReplacementRecords)
	if len(compDeps) > 0 {
		observe.GlobalTrace("if: len(compDeps) > 0")
		e.compactor = compDeps[0].Compactor
		e.autoTracker = compDeps[0].AutoTracker
		e.windowConfig = compDeps[0].WindowConfig
	}
	observe.GlobalTrace("return: e")
	return e
}

// ForkFreshConversation creates an engine for a scoped orchestration/persona
// state. It shares runtime dependencies with the parent engine but keeps a
// separate conversation store so revisiting that state preserves only that
// state's model history.
func (e *Engine) ForkFreshConversation() (*Engine, *app.StateStore) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	snap := e.store.Snapshot()
	modelID := firstNonEmpty(snap.Model, snap.Conversation.Model, e.config.Model)
	providerName := firstNonEmpty(snap.Provider, snap.Conversation.Provider)
	workDir := firstNonEmpty(snap.CWD, snap.Conversation.WorkDir)
	conversation := model.NewConversation(model.SystemPrompt{}, modelID, providerName, workDir)
	subStore := app.NewStateStore(app.AppState{
		Conversation:      conversation,
		CWD:               workDir,
		Model:             modelID,
		Provider:          providerName,
		MaxTokens:         snap.MaxTokens,
		Temperature:       snap.Temperature,
		Thinking:          snap.Thinking,
		Worktree:          snap.Worktree,
		ArtifactSessionID: snap.SessionID(),
	})
	sub := &Engine{
		provider:     e.provider,
		store:        subStore,
		costTracker:  e.costTracker,
		bus:          e.bus,
		config:       e.config,
		compactor:    e.compactor,
		autoTracker:  e.autoTracker,
		windowConfig: e.windowConfig,
		hookMgr:      e.hookMgr,
		contentReplacementState: toolresult.ReconstructContentReplacementState(
			conversation.APIMessages(),
			e.config.ContentReplacementRecords,
		),
	}
	observe.GlobalTrace("return: sub, subStore")
	return sub, subStore
}

func firstNonEmpty(values ...string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, value := range values {
		observe.GlobalTrace("range values")
		if strings.TrimSpace(value) != "" {
			observe.GlobalTrace("if: strings.TrimSpace(value) != \"\"")
			observe.GlobalTrace("return: value")
			return value
		}
	}
	observe.GlobalTrace("return: \"\"")
	return ""
}

// SetHookManager configures the hook manager for Stop hooks.
func (e *Engine) SetHookManager(mgr *hook.Manager) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	e.hookMgr = mgr
}

// RebindProvider switches the engine to a new provider/model runtime.
func (e *Engine) RebindProvider(prov provider.Provider, modelID string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if e.provider != nil && e.provider != prov {
		observe.GlobalTrace("if: e.provider != nil && e.provider != prov")
		if err := provider.Close(context.Background(), e.provider); err != nil && e.bus != nil {
			observe.GlobalTrace("if: err != nil && e.bus != nil")
			e.bus.Emit(observe.ErrorOccurred{
				EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
				Severity:     "warn",
				Component:    "provider",
				ErrorType:    "provider_cleanup_failed",
				ErrorMessage: err.Error(),
			})
		}
	}
	e.provider = provider.WithAccounting(prov, e.costTracker, e.bus)
	e.SetModel(modelID)
}

// SetModel updates the engine's fallback model for execution paths that do not
// have an AppState model override.
func (e *Engine) SetModel(modelID string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if modelID != "" {
		observe.GlobalTrace("if: modelID != \"\"")
		e.config.Model = modelID
	}
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

func (e *Engine) SetSessionCheckpoint(checkpoint func() error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	e.config.SessionCheckpoint = checkpoint
}

// ResetSessionState rebuilds read-time replacement tracking after the active
// conversation/session changes.
func (e *Engine) ResetSessionState(contentReplacementRecords []model.ContentReplacementRecord) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	e.resetContentReplacementState(contentReplacementRecords)
}

func (e *Engine) resetContentReplacementState(records []model.ContentReplacementRecord) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	snap := e.store.Snapshot()
	e.contentReplacementState = toolresult.ReconstructContentReplacementState(snap.Conversation.APIMessages(), records)
}

// EventBus returns the engine's durable event bus.
func (e *Engine) EventBus() *observe.EventBus {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: e.bus")
	return e.bus
}

func (e *Engine) appendConversationMessage(msg model.Message, mutate func(*app.AppState)) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	e.store.Update(func(s *app.AppState) {
		s.Conversation.Append(msg)
		if mutate != nil {
			observe.GlobalTrace("if: mutate != nil")
			mutate(s)
		}
	})
	e.emitMessageAppended(msg)
	observe.GlobalTrace("return: e.checkpointSession()")
	return e.checkpointSession()
}

func (e *Engine) checkpointSession() error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if e.config.SessionCheckpoint == nil {
		observe.GlobalTrace("if: e.config.SessionCheckpoint == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	observe.GlobalTrace("return: e.config.SessionCheckpoint()")
	return e.config.SessionCheckpoint()
}

// AppendHookContext appends hook-produced context as an internal user message
// so the next provider request can see it without exposing it as assistant text.
func (e *Engine) AppendHookContext(source string, contexts []string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	text := formatHookContext(source, contexts)
	if text == "" {
		observe.GlobalTrace("if: text == \"\"")
		observe.GlobalTrace("return: nil")
		return nil
	}
	observe.GlobalTrace("return: e.appendConversationMessage(model.Message{\n\tID:\t\tmodel.NewUUID(),\n\tRole:\t\tmod...")
	return e.appendConversationMessage(model.Message{
		ID:        model.NewUUID(),
		Role:      model.RoleUser,
		Content:   []model.ContentPart{model.TextPart{Text: text}},
		Timestamp: time.Now(),
		Flags:     model.MessageFlags{IsInternal: true},
	}, nil)
}

func formatHookContext(source string, contexts []string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	cleaned := make([]string, 0, len(contexts))
	for _, ctx := range contexts {
		observe.GlobalTrace("range contexts")
		if trimmed := strings.TrimSpace(ctx); trimmed != "" {
			observe.GlobalTrace("if: trimmed != \"\"")
			cleaned = append(cleaned, trimmed)
		}
	}
	if len(cleaned) == 0 {
		observe.GlobalTrace("if: len(cleaned) == 0")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	if source == "" {
		observe.GlobalTrace("if: source == \"\"")
		source = "hook"
	}
	observe.GlobalTrace("return: \"Hook context from \" + source + \":\\n\" + strings.Join(cleaned, \"\\n\\n\")")
	return "Hook context from " + source + ":\n" + strings.Join(cleaned, "\n\n")
}

func (e *Engine) emitMessageAppended(msg model.Message) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if e.bus == nil {
		observe.GlobalTrace("if: e.bus == nil")
		return
	}
	e.bus.Emit(observe.MessageAppended{
		EventHeader:   observe.NewEventHeader("MessageAppended", "", observe.NewSpanID(), ""),
		MessageID:     msg.ID,
		Role:          string(msg.Role),
		ContentTypes:  messageContentTypes(msg.Content),
		TokenEstimate: compact.EstimateTokens(msg),
	})
}

func messageContentTypes(parts []model.ContentPart) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	types := make([]string, 0, len(parts))
	for _, part := range parts {
		observe.GlobalTrace("range parts")
		types = append(types, string(part.PartType()))
	}
	observe.GlobalTrace("return: types")
	return types
}

// RunGraph executes a lifecycle graph using the engine's own provider. Returns
// a channel of LoopEvents, same as Run().
func (e *Engine) RunGraph(ctx context.Context, graph *lifecycle.Graph, prompt string) <-chan LoopEvent {
	observe.TraceCtx(ctx, "query", "Engine.RunGraph", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.RunGraph", "exit")
	ch := make(chan LoopEvent, 16)
	go func() {
		defer close(ch)
		defer func() {
			if r := recover(); r != nil {
				observe.TraceCtx(ctx, "query", "Engine.RunGraph", "if: r != nil")
				ch <- ErrorEvent{Err: fmt.Errorf("graph execution panic: %v", r)}
			}
		}()
		e.runGraph(ctx, graph, prompt, ch)
	}()
	observe.TraceCtx(ctx, "query", "Engine.RunGraph", "return: ch")
	return ch
}

func (e *Engine) runGraph(ctx context.Context, graph *lifecycle.Graph, prompt string, ch chan<- LoopEvent) {
	observe.TraceCtx(ctx, "query", "Engine.runGraph", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.runGraph", "exit")

	snap := e.store.Snapshot()
	resolvedModel := e.config.Model
	if snap.Model != "" {
		observe.TraceCtx(ctx, "query", "Engine.runGraph", "if: snap.Model != \"\"")
		resolvedModel = snap.Model
	}
	runner := bridge.NewRunner(graph, bridge.NewRunnerConfig(
		snap.Conversation.System,
		resolvedModel,
		e.config.MaxTokens,
		nil,
		e.bus,
	))

	var result bridge.RunResult
	for runEv := range runner.Stream(ctx, prompt) {
		observe.TraceCtx(ctx, "query", "Engine.runGraph", "range events")
		progress := runEv.Progress
		ch <- lifecycleProgressEvent(progress)
		if progress.Status == "completed" {
			observe.TraceCtx(ctx, "query", "Engine.runGraph", "if: progress.Status == \"completed\"")
			result = runEv.Result
		}
	}

	if err := e.appendLifecycleMessages(result.State); err != nil {
		observe.TraceCtx(ctx, "query", "Engine.runGraph", "if: err != nil")
		ch <- ErrorEvent{Err: err}
		return
	}

	if result.Err != nil {
		observe.TraceCtx(ctx, "query", "Engine.runGraph", "if: result.Err != nil")
		ch <- ErrorEvent{Err: fmt.Errorf("lifecycle graph: %w", result.Err)}
		return
	}

	if result.AssistantText != "" {
		observe.TraceCtx(ctx, "query", "Engine.runGraph", "if: result.AssistantText != \"\"")
		ch <- TextEvent{Text: result.AssistantText}
	}
	ch <- TurnCompleteEvent{Response: result.Response, StopReason: result.StopReason}
}

func lifecycleProgressEvent(progress bridge.ProgressEvent) LifecycleProgressEvent {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: LifecycleProgressEvent{\n\tStep:\t\tprogress.Step,\n\tNode:\t\tprogress.Node,\n\tNodes:...")
	return LifecycleProgressEvent{
		Step:     progress.Step,
		Node:     progress.Node,
		Nodes:    progress.Nodes,
		Status:   progress.Status,
		Duration: progress.Duration,
		Error:    progress.Error,
		FromNode: progress.FromNode,
		ToNode:   progress.ToNode,
		RouteKey: progress.RouteKey,
	}
}

func (e *Engine) appendLifecycleMessages(state lifecycle.State) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, msg := range bridge.Messages(state) {
		observe.GlobalTrace("range bridge.Messages(state)")
		if err := e.appendConversationMessage(msg, nil); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: err")
			return err
		}
	}
	observe.GlobalTrace("return: nil")
	return nil
}
