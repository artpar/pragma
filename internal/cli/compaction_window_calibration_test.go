package cli

import (
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/query"
)

// TestBuildCompactionDepsCalibratesSystemPromptEstToLoopMode guards
// CMP-001.4 F8: BuildCompactionDeps estimated SystemPromptEst from
// PragmaLoopSystemPrompt for every mode — a prompt provider-tools
// requests never carry at all — while the fixed request payload they DO
// carry (the custom prompt, the conversation's system blocks, harness
// manifest, MCP status, patch guidance, tool schemas) was counted
// nowhere. The reserve must at least cover the provable per-request
// payload: the custom prompt (prepended by both loops) and the
// conversation's system blocks (carried verbatim by provider-tools
// requests, replaced by custom+PragmaLoopSystemPrompt in pragma mode).
func TestBuildCompactionDepsCalibratesSystemPromptEstToLoopMode(t *testing.T) {
	cases := []struct {
		name     string
		loopMode string
	}{
		{"provider-tools", query.LoopModeProviderTools},
		{"pragma", query.LoopModePragma},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prov := &compactSessionTestProvider{}
			bigSystem := strings.Repeat("word ", 2500)      // ~3,125 heuristic tokens
			customPrompt := strings.Repeat("persona ", 500) // ~1,000 heuristic tokens
			conv := model.NewConversation(
				model.SystemPrompt{Blocks: []model.SystemBlock{{Text: bigSystem}}},
				"test-model", "test", t.TempDir())
			conv.Messages = []model.Message{
				{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "hi"}}},
				{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: "hello"}}},
			}
			store := app.NewStateStore(app.AppState{
				Conversation: conv,
				CWD:          t.TempDir(),
				Model:        "test-model",
				Provider:     "test",
			})
			engineCfg := query.EngineConfig{
				Model:              "test-model",
				LoopMode:           tc.loopMode,
				MaxTokens:          4096,
				CustomSystemPrompt: customPrompt,
			}
			engine := query.NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(64), engineCfg)
			d := &Deps{
				Prov:        prov,
				Store:       store,
				Bus:         observe.NewEventBus(64),
				CostTracker: model.NewCostTracker(0),
				Cfg:         config.Config{Provider: "test", Model: "test-model", MaxTokens: 4096},
				EngineCfg:   engineCfg,
				Engine:      engine,
			}

			compDeps, _ := BuildCompactionDeps(d)

			// Conservative floors for the reserve: both loops re-send
			// the custom prompt (~1,000 tokens here) with every request;
			// provider-tools requests also carry the conversation's
			// system blocks verbatim (~3,125 tokens here).
			var want int
			if tc.loopMode == query.LoopModeProviderTools {
				want = 3_000 // custom + conversation system both re-sent
			} else {
				want = 1_000 // pragma mode: custom prompt re-sent; the loop replaces the conversation system
			}
			if compDeps.WindowConfig.SystemPromptEst < want {
				t.Fatalf("%s mode: SystemPromptEst = %d, want >= %d — the reserve ignored the custom prompt (%d tokens) and the conversation system (%d tokens) the requests actually re-send (CMP-001.4 F8)",
					tc.name, compDeps.WindowConfig.SystemPromptEst, want, len(customPrompt)/4, len(bigSystem)/4)
			}
		})
	}
}
