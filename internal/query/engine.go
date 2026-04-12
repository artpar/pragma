package query

import (
	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/compact"
	"github.com/artpar/gogent/internal/hook"
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
	observe.GlobalTrace("return: e")
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
