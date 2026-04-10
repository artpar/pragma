package query

import (
	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
	"github.com/artpar/gogent/internal/tool"
)

// EngineConfig holds query engine parameters derived from config + CLI flags.
type EngineConfig struct {
	Model       string
	MaxTokens   int
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
) *Engine {
	return &Engine{
		provider:     prov,
		registry:     reg,
		orchestrator: orch,
		store:        store,
		costTracker:  ct,
		bus:          bus,
		config:       cfg,
	}
}
