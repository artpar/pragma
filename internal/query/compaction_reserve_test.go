package query

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/compact"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
)

// CMP-001.4 F8 reserve-shape gates: WindowConfig.SystemPromptEst must
// reserve the request-fixed payload the ACTIVE loop mode re-sends with
// every model request — the payload auto-compaction cannot remove. The
// pre-F8 BuildCompactionDeps estimated PragmaLoopSystemPrompt for every
// mode: provider-tools requests carry the tool schemas, harness
// manifest, and patch guidance nowhere in that estimate, and pragma
// requests' custom prompt was missing from it too.

// reserveTestProvider is the minimal provider the estimate needs (name
// for the harness manifest; no model calls are made).
type reserveTestProvider struct{}

func (p *reserveTestProvider) Name() string { return "test" }

func (p *reserveTestProvider) Stream(context.Context, provider.RequestParams) (<-chan provider.StreamChunk, error) {
	return nil, errors.New("Stream is not used by this test provider")
}

func (p *reserveTestProvider) Complete(context.Context, provider.RequestParams) (model.Response, error) {
	return model.Response{}, errors.New("Complete is not used by this test provider")
}

func (p *reserveTestProvider) SupportsFeature(provider.Feature) bool { return false }
func (p *reserveTestProvider) Pricing(string) (model.Pricing, bool)  { return model.Pricing{}, false }
func (p *reserveTestProvider) ContextWindow(string) (int, bool)      { return 200_000, true }
func (p *reserveTestProvider) ListModels() []string                  { return nil }

func reserveEngine(t *testing.T, loopMode string, customPrompt string, convSystem model.SystemPrompt, cfgExtras func(*EngineConfig)) *Engine {
	t.Helper()
	conv := model.NewConversation(convSystem, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          t.TempDir(),
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	cfg := EngineConfig{
		Model:              "test-model",
		LoopMode:           loopMode,
		MaxTokens:          4096,
		CustomSystemPrompt: customPrompt,
	}
	if cfgExtras != nil {
		cfgExtras(&cfg)
	}
	return NewEngine(&reserveTestProvider{}, store, model.NewCostTracker(0), observe.NewEventBus(64), cfg)
}

// TestEstimateCompactionReserveCountsProviderToolsFixedPayload pins the
// provider-tools mode reserve: with no custom prompt, no conversation
// system, and no MCP statuses, the reserve is EXACTLY the static
// per-request payload — harness manifest + patch guidance blocks plus
// the static tool schemas (Bash, apply_patch, WebSearch, Agent). The
// pre-F8 estimate (PragmaLoopSystemPrompt tokens, ~270) counts none of
// these; a reserve below the re-injected payload defeats the
// death-spiral guard the field exists for.
func TestEstimateCompactionReserveCountsProviderToolsFixedPayload(t *testing.T) {
	engine := reserveEngine(t, LoopModeProviderTools, "", model.SystemPrompt{}, func(cfg *EngineConfig) {
		cfg.WebSearch = func(context.Context, json.RawMessage) (string, error) { return "", nil }
	})
	reserve := engine.EstimateCompactionReserve()

	// The exact toolset the provider-tools loop injects for this config.
	tools := providerToolDefs()
	tools = engine.withWebSearchTool(tools)
	tools = engine.withSubAgentTool(tools)
	toolTokens := 0
	for _, tool := range tools {
		toolTokens += compact.EstimateToolDefTokens(tool)
	}
	// The manifest and patch-guidance blocks the loop appends every
	// iteration (MCP statuses unconfigured -> no MCP block).
	system := engine.systemWithHarnessManifest(model.SystemPrompt{})
	system = engine.systemWithPatchGuidance(system, tools)
	blockTokens := compact.EstimateSystemPromptTokens(system)

	if toolTokens == 0 {
		t.Fatalf("fixture drift: static tool schemas estimated at 0 tokens")
	}
	if reserve != toolTokens+blockTokens {
		t.Fatalf("provider-tools reserve = %d, want exactly tool schemas (%d) + manifest/patch blocks (%d) = %d — the reserve must count the tool schemas and fixed system blocks the requests re-send after compaction (CMP-001.4 F8)",
			reserve, toolTokens, blockTokens, toolTokens+blockTokens)
	}
}

// TestEstimateCompactionReservePragmaModeIsLoopSystemNotConversation pins
// the mode split: the pragma loop replaces the conversation system with
// custom prompt + PragmaLoopSystemPrompt at run start, so its reserve is
// exactly that payload — NOT the provider-tools shape (no tool schemas,
// no manifest) and NOT the conversation's stored system blocks.
func TestEstimateCompactionReservePragmaModeIsLoopSystemNotConversation(t *testing.T) {
	custom := strings.Repeat("persona ", 500)                // ~500 heuristic tokens
	bigConversationSystem := strings.Repeat("system ", 1000) // ~1750 heuristic tokens, must NOT be reserved
	engine := reserveEngine(t, LoopModePragma, custom, model.SystemPrompt{
		Blocks: []model.SystemBlock{{Text: bigConversationSystem}},
	}, nil)
	reserve := engine.EstimateCompactionReserve()

	want := compact.EstimateSystemPromptTokens(engine.pragmaLoopSystemPrompt())
	if reserve != want {
		t.Fatalf("pragma reserve = %d, want exactly est(custom+PragmaLoopSystemPrompt) = %d (CMP-001.4 F8)",
			reserve, want)
	}
	convEst := compact.EstimateSystemPromptTokens(model.SystemPrompt{
		Blocks: []model.SystemBlock{{Text: bigConversationSystem}},
	})
	if reserve >= want+convEst {
		t.Fatalf("pragma reserve = %d counted the conversation system (%d tokens) the pragma loop replaces at run start — only provider-tools requests re-send it (CMP-001.4 F8)",
			reserve, convEst)
	}
}
