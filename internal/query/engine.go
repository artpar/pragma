package query

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/compact"
	"github.com/artpar/pragma/internal/hook"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/toolresult"
)

// DefaultSubAgentMaxTurns bounds sub-agent loops when the parent runs
// uncapped (TURN-003): the interactive root loop has no default turn cap
// (operator directive 2026-09-12, after TURN-001/002's cap deaths), but a
// drifting sub-agent would block the parent's synchronous fork
// indefinitely with no one watching — the runaway guard stays for subs.
const DefaultSubAgentMaxTurns = 100

const (
	LoopModePragma        = "pragma"
	LoopModeProviderTools = "provider-tools"
)

// EngineConfig holds query engine parameters derived from config + CLI flags.
type EngineConfig struct {
	Model                     string
	LoopMode                  string
	MaxTokens                 int
	MaxTurns                  int // 0 = no turn cap (TURN-003); >0 bounds the loop
	Temperature               *float64
	Thinking                  *provider.ThinkingConfig
	ResponseSchema            json.RawMessage
	CustomSystemPrompt        string
	ContentReplacementRecords []model.ContentReplacementRecord
	RecordContentReplacements func([]model.ContentReplacementRecord) error
	SessionCheckpoint         func() error
	MCPServerStatuses         func() []MCPServerStatus
	// MCPToolDefs returns tool definitions for connected MCP servers. When
	// set, the provider-tools loop injects them into the model tool list
	// (names are "mcp__<server>__<tool>"; see internal/mcp/adapter.go).
	MCPToolDefs func(ctx context.Context) []model.ToolDef
	// MCPCallTool routes an "mcp__<server>__<tool>" invocation back to the
	// owning MCP server. Required to execute injected MCP tool calls.
	MCPCallTool func(ctx context.Context, toolName string, input json.RawMessage) (string, error)
	// WebSearch executes a WebSearch tool invocation (Brave Search API).
	// When set, the provider-tools loop offers the WebSearch tool.
	WebSearch func(ctx context.Context, input json.RawMessage) (string, error)
	// DisableSubAgents is set on sub-agent engines so the Agent tool is not
	// injected into their toolset (recursion guard, SUB-001).
	DisableSubAgents    bool
	RefreshCapabilities func(context.Context)
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

	// INT-001: operator input submitted while a turn is running. Messages
	// that cannot append immediately (conversation tail is an assistant
	// tool_use awaiting results) park here and drain at the loop's next
	// safe point; see provider_tools_loop.go AppendUserInput.
	pendingUserInputs   []model.Message
	pendingUserInputsMu sync.Mutex
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
func (engine *Engine) ForkFreshConversation() (*Engine, *app.StateStore) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	snap := engine.store.Snapshot()
	modelID := firstNonEmpty(snap.Model, snap.Conversation.Model, engine.config.Model)
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

	subCfg := engine.config
	if subCfg.MaxTurns <= 0 {
		observe.GlobalTrace("if: subCfg.MaxTurns <= 0")
		subCfg.MaxTurns = DefaultSubAgentMaxTurns
	}
	sub := &Engine{
		provider:     engine.provider,
		store:        subStore,
		costTracker:  engine.costTracker,
		bus:          engine.bus,
		config:       subCfg,
		compactor:    engine.compactor,
		autoTracker:  engine.autoTracker,
		windowConfig: engine.windowConfig,
		hookMgr:      engine.hookMgr,
		contentReplacementState: toolresult.ReconstructContentReplacementState(
			conversation.APIMessages(),
			engine.config.ContentReplacementRecords,
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
func (engine *Engine) SetHookManager(mgr *hook.Manager) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	engine.hookMgr = mgr
}

// RebindProvider switches the engine to a new provider/model runtime.
func (engine *Engine) RebindProvider(prov provider.Provider, modelID string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if engine.provider != nil && engine.provider != prov {
		observe.GlobalTrace("if: engine.provider != nil && engine.provider != prov")
		if err := provider.Close(context.Background(), engine.provider); err != nil && engine.bus != nil {
			observe.GlobalTrace("if: err != nil && engine.bus != nil")
			engine.bus.Emit(observe.ErrorOccurred{
				EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
				Severity:     "warn",
				Component:    "provider",
				ErrorType:    "provider_cleanup_failed",
				ErrorMessage: err.Error(),
			})
		}
	}
	engine.provider = provider.WithAccounting(prov, engine.costTracker, engine.bus)
	engine.SetModel(modelID)
}

// BindProvider switches the engine runtime without closing the previous
// provider. Orchestration state engines often start by sharing the root
// provider, so scoped rebinds must not clean up dependencies they do not own.
func (engine *Engine) BindProvider(prov provider.Provider, providerName string, modelID string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	engine.provider = provider.WithAccounting(prov, engine.costTracker, engine.bus)
	engine.config.Model = modelID
	engine.store.Update(func(s *app.AppState) {
		s.Provider = providerName
		s.Model = modelID
		s.Conversation.Provider = providerName
		s.Conversation.Model = modelID
	})
}

func (engine *Engine) ApplyRuntimeOptions(maxTokens int, temperature *float64, thinking *provider.ThinkingConfig, customSystemPrompt string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if maxTokens != 0 {
		observe.GlobalTrace("if: maxTokens != 0")
		engine.config.MaxTokens = maxTokens
	}
	engine.config.Temperature = temperature
	engine.config.Thinking = thinking
	engine.config.CustomSystemPrompt = customSystemPrompt
	engine.store.Update(func(s *app.AppState) {
		if maxTokens != 0 {
			s.MaxTokens = maxTokens
		}
		s.Temperature = temperature
		if thinking != nil {
			enabled := thinking.Enabled
			s.Thinking = &enabled
		} else {
			s.Thinking = nil
		}
	})
}

// SetModel updates the engine's fallback model for execution paths that do not
// have an AppState model override.
func (engine *Engine) SetModel(modelID string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if modelID != "" {
		observe.GlobalTrace("if: modelID != \"\"")
		engine.config.Model = modelID
	}
}

// SetCompaction configures auto-compaction after engine creation.
// Useful when the engine is created before compaction deps are ready.
func (engine *Engine) SetCompaction(deps CompactionDeps) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	engine.compactor = deps.Compactor
	engine.autoTracker = deps.AutoTracker
	engine.windowConfig = deps.WindowConfig
}

func (engine *Engine) SetSessionCheckpoint(checkpoint func() error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	engine.config.SessionCheckpoint = checkpoint
}

// ResetSessionState rebuilds read-time replacement tracking after the active
// conversation/session changes.
func (engine *Engine) ResetSessionState(contentReplacementRecords []model.ContentReplacementRecord) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	engine.resetContentReplacementState(contentReplacementRecords)
}

func (engine *Engine) resetContentReplacementState(records []model.ContentReplacementRecord) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	snap := engine.store.Snapshot()
	engine.contentReplacementState = toolresult.ReconstructContentReplacementState(snap.Conversation.APIMessages(), records)
}

// EventBus returns the engine's durable event bus.
func (engine *Engine) EventBus() *observe.EventBus {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: engine.bus")
	return engine.bus
}

func (engine *Engine) setConversationSystemPrompt(system model.SystemPrompt) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	engine.store.Update(func(s *app.AppState) {
		s.Conversation.System = system
	})
}

func (engine *Engine) appendConversationMessage(msg model.Message) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	engine.store.Update(func(s *app.AppState) {
		s.Conversation.Append(msg)
	})
	engine.emitMessageAppended(msg)
	observe.GlobalTrace("return: engine.checkpointSession()")
	return engine.checkpointSession()
}

func (engine *Engine) checkpointSession() error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if engine.config.SessionCheckpoint == nil {
		observe.GlobalTrace("if: engine.config.SessionCheckpoint == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	observe.GlobalTrace("return: engine.config.SessionCheckpoint()")
	return engine.config.SessionCheckpoint()
}

// AppendHookContext appends hook-produced context as an internal user message
// so the next provider request can see it without exposing it as assistant text.
func (engine *Engine) AppendHookContext(source string, contexts []string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	text := formatHookContext(source, contexts)
	if text == "" {
		observe.GlobalTrace("if: text == \"\"")
		observe.GlobalTrace("return: nil")
		return nil
	}
	observe.GlobalTrace("return: engine.appendConversationMessage(model.Message{...})")
	observe.GlobalTrace("return: engine.appendConversationMessage(model.Message{\n\tID:\t\tmodel.NewUUID(),\n\tRole:...")
	return engine.appendConversationMessage(model.Message{
		ID:        model.NewUUID(),
		Role:      model.RoleUser,
		Content:   []model.ContentPart{model.TextPart{Text: text}},
		Timestamp: time.Now(),
		Flags:     model.MessageFlags{IsInternal: true},
	})
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

func (engine *Engine) emitMessageAppended(msg model.Message) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if engine.bus == nil {
		observe.GlobalTrace("if: engine.bus == nil")
		return
	}
	engine.bus.Emit(observe.MessageAppended{
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
