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
	"github.com/artpar/pragma/internal/task"
	"github.com/artpar/pragma/internal/tool"
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
	ContextMode               string
	HandoffSchema             string
	StopAfterToolExec         bool
	Temperature               *float64
	Thinking                  *provider.ThinkingConfig
	ResponseSchema            json.RawMessage
	TaskID                    string // when set with TaskRegistry, enables task heartbeat
	ContentReplacementRecords []model.ContentReplacementRecord
	FileStateRecords          []tool.FileStateRecord
	RecordContentReplacements func([]model.ContentReplacementRecord) error
	SessionCheckpoint         func() error
	MCPServerStatuses         func() []MCPServerStatus
}

// MCPServerStatus mirrors the session-init MCP server metadata shape.
type MCPServerStatus struct {
	Name   string
	Status string
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
	fileState    *tool.FileStateCache

	// Compaction — nil means auto-compaction disabled.
	// Subagent engines pass nil (#27794: only root engine auto-compacts).
	compactor    *compact.Service
	autoTracker  *compact.AutoTracker
	windowConfig compact.WindowConfig

	// Hooks — nil means no hook manager configured.
	hookMgr *hook.Manager

	// Task registry — when set with config.TaskID, enables heartbeat for task-owned engines.
	taskRegistry *task.Registry

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
	fileState := tool.NewFileStateCache()
	fileState.Restore(cfg.FileStateRecords)
	prov = provider.WithAccounting(prov, ct, bus)
	e := &Engine{
		provider:     prov,
		registry:     reg,
		orchestrator: orch,
		store:        store,
		costTracker:  ct,
		bus:          bus,
		config:       cfg,
		fileState:    fileState,
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

// SetHookManager configures the hook manager for Stop hooks.
func (e *Engine) SetHookManager(mgr *hook.Manager) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	e.hookMgr = mgr
}

// SetTaskRegistry configures the task registry used for task heartbeat/reaping.
func (e *Engine) SetTaskRegistry(reg *task.Registry) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	e.taskRegistry = reg
}

// SetTaskID sets the task ID for task heartbeat.
func (e *Engine) SetTaskID(id string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	e.config.TaskID = id
}

// RebindProvider switches the engine to a new provider/model runtime.
func (e *Engine) RebindProvider(prov provider.Provider, modelID string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	e.provider = provider.WithAccounting(prov, e.costTracker, e.bus)
	if modelID != "" {
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

// ResetContentReplacementState rebuilds read-time replacement tracking after
// the active conversation changes, such as an in-TUI session resume.
func (e *Engine) ResetContentReplacementState(records []model.ContentReplacementRecord) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	snap := e.store.Snapshot()
	e.contentReplacementState = toolresult.ReconstructContentReplacementState(snap.Conversation.APIMessages(), records)
}

func (e *Engine) ResetFileState(records []tool.FileStateRecord) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if e.fileState == nil {
		e.fileState = tool.NewFileStateCache()
	}
	e.fileState.Restore(records)
}

func (e *Engine) FileStateRecords() []tool.FileStateRecord {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if e.fileState == nil {
		return nil
	}
	return e.fileState.Snapshot()
}

// Orchestrator returns the engine's tool orchestrator.
func (e *Engine) Orchestrator() *tool.Orchestrator {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: e.orchestrator")
	return e.orchestrator
}

// Registry returns the engine's tool registry.
func (e *Engine) Registry() *tool.Registry {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: e.registry")
	return e.registry
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
	return e.checkpointSession()
}

func (e *Engine) checkpointSession() error {
	if e.config.SessionCheckpoint == nil {
		return nil
	}
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
		return nil
	}
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
	runner := bridge.NewRunner(graph, bridge.NewRunnerConfig(
		snap.Conversation.System,
		e.config.Model,
		e.config.MaxTokens,
		e.registry.ToolDefs(),
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
			return err
		}
	}
	return nil
}
