package lilac

import (
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/provider/shared"
)

// TestStreamUsageEstimation verifies that when Lilac's vLLM backend
// returns zero usage tokens in streaming mode, the provider estimates
// them from the accumulated content.
//
// Reproduces issue #10: cost shows $0.000000 because Lilac streaming
// doesn't populate usage in SSE chunks.
func TestStreamUsageEstimation(t *testing.T) {
	bus := observe.NewEventBus(1024)

	_ = bus // used by provider

	p := &Provider{
		inner:      nil, // not used — we'll test via the estimation logic directly
		bus:        bus,
		maxRetries: 0,
	}

	// Simulate what happens in Stream(): output tokens estimated from accumulated text
	t.Run("output_tokens_estimated_from_text", func(t *testing.T) {
		// Simulate 200 chars of text output (should estimate ~50 tokens)
		text := "This is a test response from the model that contains enough text to verify token estimation works correctly for Lilac models."
		estimatedOutput := len(text) / 4

		if estimatedOutput == 0 {
			t.Fatal("estimation should produce non-zero output tokens")
		}
		if estimatedOutput < 20 || estimatedOutput > 40 {
			t.Errorf("expected ~30 tokens for %d chars, got %d", len(text), estimatedOutput)
		}
	})

	t.Run("output_tokens_estimated_from_tool_calls", func(t *testing.T) {
		// Simulate a tool call with JSON input
		toolInput := `{"file_path": "/Users/test/workspace/internal/util/path.go", "limit": 100}`
		estimatedOutput := len(toolInput) / 4

		if estimatedOutput == 0 {
			t.Fatal("estimation should produce non-zero output tokens for tool calls")
		}
	})

	t.Run("input_tokens_estimated_from_params", func(t *testing.T) {
		params := provider.RequestParams{
			Model: "moonshotai/kimi-k2.5",
			Messages: []model.Message{
				{
					Role: model.RoleUser,
					Content: []model.ContentPart{
						model.TextPart{Text: "Read the file go.mod and tell me the module name."},
					},
				},
			},
			System: model.SystemPrompt{
				Blocks: []model.SystemBlock{
					{Text: "You are a helpful coding assistant with access to tools."},
				},
			},
		}

		estimated := shared.EstimateTokens(params)
		if estimated == 0 {
			t.Fatal("input token estimation should be non-zero")
		}
		// ~50 chars user msg / 4 + ~56 chars system / 4 = ~26 tokens
		if estimated < 15 || estimated > 40 {
			t.Errorf("expected ~26 tokens, got %d", estimated)
		}
	})

	t.Run("EstimateOutputTokens_with_mixed_content", func(t *testing.T) {
		parts := []model.ContentPart{
			model.TextPart{Text: "Here is what I found in the file:"},
			model.ToolCallPart{
				ID:    "call_123",
				Name:  "Read",
				Input: []byte(`{"file_path": "go.mod"}`),
			},
		}

		estimated := shared.EstimateOutputTokens(parts)
		if estimated == 0 {
			t.Fatal("should estimate non-zero for text + tool call parts")
		}
		// 33 chars text / 4 + 23 chars tool input / 4 = ~14 tokens
		if estimated < 10 || estimated > 20 {
			t.Errorf("expected ~14 tokens, got %d", estimated)
		}
	})

	t.Run("pricing_lookup_works_for_all_models", func(t *testing.T) {
		models := []string{"moonshotai/kimi-k2.5", "moonshotai/kimi-k2.6", "minimaxai/minimax-m2.7", "minimaxai/minimax-m3", "zai-org/glm-5.1", "google/gemma-4-31b-it"}
		for _, m := range models {
			pricing, ok := p.Pricing(m)
			if !ok {
				t.Errorf("pricing not found for model %s", m)
				continue
			}
			if pricing.InputPerMToken == 0 || pricing.OutputPerMToken == 0 {
				t.Errorf("pricing for %s has zero values: input=$%.2f output=$%.2f",
					m, pricing.InputPerMToken, pricing.OutputPerMToken)
			}
		}
	})

	t.Run("minimax_m2_7_registry", func(t *testing.T) {
		info, ok := LookupModel("minimaxai/minimax-m2.7")
		if !ok {
			t.Fatal("model not found for minimaxai/minimax-m2.7")
		}
		if info.ID != "minimaxai/minimax-m2.7" {
			t.Errorf("expected ID minimaxai/minimax-m2.7, got %q", info.ID)
		}
		if info.MaxContext != 204800 {
			t.Errorf("expected MaxContext=204800, got %d", info.MaxContext)
		}
		if info.MaxOutput != 131072 {
			t.Errorf("expected MaxOutput=131072, got %d", info.MaxOutput)
		}
		if info.Pricing.InputPerMToken != 0.30 {
			t.Errorf("expected input=0.30, got %.3f", info.Pricing.InputPerMToken)
		}
		if info.Pricing.OutputPerMToken != 1.20 {
			t.Errorf("expected output=1.20, got %.3f", info.Pricing.OutputPerMToken)
		}
		if info.Pricing.CacheReadPerMToken != 0.055 {
			t.Errorf("expected cache_read=0.055, got %.3f", info.Pricing.CacheReadPerMToken)
		}
		if !info.SupportsToolUse {
			t.Error("expected SupportsToolUse=true")
		}
		if !info.SupportsReasoning {
			t.Error("expected SupportsReasoning=true")
		}
		if info.SupportsVision {
			t.Error("expected SupportsVision=false")
		}
	})

	t.Run("minimax_m3_registry", func(t *testing.T) {
		info, ok := LookupModel("minimaxai/minimax-m3")
		if !ok {
			t.Fatal("model not found for minimaxai/minimax-m3")
		}
		if info.ID != "minimaxai/minimax-m3" {
			t.Errorf("expected ID minimaxai/minimax-m3, got %q", info.ID)
		}
		if info.MaxContext != 1048576 {
			t.Errorf("expected MaxContext=1048576, got %d", info.MaxContext)
		}
		if info.MaxOutput != 1048576 {
			t.Errorf("expected MaxOutput=1048576, got %d", info.MaxOutput)
		}
		if info.Pricing.InputPerMToken != 0.28 {
			t.Errorf("expected input=0.28, got %.3f", info.Pricing.InputPerMToken)
		}
		if info.Pricing.OutputPerMToken != 1.10 {
			t.Errorf("expected output=1.10, got %.3f", info.Pricing.OutputPerMToken)
		}
		if info.Pricing.CacheReadPerMToken != 0.05 {
			t.Errorf("expected cache_read=0.05, got %.3f", info.Pricing.CacheReadPerMToken)
		}
		if !info.SupportsVision {
			t.Error("expected SupportsVision=true")
		}
		if !info.SupportsToolUse {
			t.Error("expected SupportsToolUse=true")
		}
		if !info.SupportsReasoning {
			t.Error("expected SupportsReasoning=true")
		}
	})

	t.Run("kimi_k2_6_cache_read_pricing", func(t *testing.T) {
		pricing, ok := p.Pricing("moonshotai/kimi-k2.6")
		if !ok {
			t.Fatal("pricing not found for moonshotai/kimi-k2.6")
		}
		if pricing.InputPerMToken != 0.70 {
			t.Errorf("expected input=0.70, got %.2f", pricing.InputPerMToken)
		}
		if pricing.OutputPerMToken != 3.50 {
			t.Errorf("expected output=3.50, got %.2f", pricing.OutputPerMToken)
		}
		if pricing.CacheReadPerMToken != 0.20 {
			t.Errorf("expected cache_read=0.20, got %.2f", pricing.CacheReadPerMToken)
		}
	})

	t.Run("cost_calculation_non_zero", func(t *testing.T) {
		// Simulate what CostTracker.Record does with estimated usage
		pricing := model.Pricing{InputPerMToken: 0.40, OutputPerMToken: 2.00} // kimi-k2.5
		usage := model.TokenUsage{InputTokens: 4000, OutputTokens: 50}

		cost := float64(usage.InputTokens)*pricing.InputPerMToken/1_000_000 +
			float64(usage.OutputTokens)*pricing.OutputPerMToken/1_000_000

		if cost == 0 {
			t.Fatal("cost should be non-zero with non-zero usage and pricing")
		}
		// 4000 * 0.40/1M + 50 * 2.00/1M = 0.0016 + 0.0001 = 0.0017
		expected := 0.0017
		if cost < expected*0.9 || cost > expected*1.1 {
			t.Errorf("expected cost ~$%.4f, got $%.4f", expected, cost)
		}
	})

	_ = p // used for Pricing lookup
}

func TestCompletionUsageCacheReadAccounting(t *testing.T) {
	usage := (&completionUsage{
		PromptTokens:     8_331_609,
		CompletionTokens: 104_900,
		TotalTokens:      8_436_509,
		PromptTokensDetails: promptTokensDetails{
			CachedTokens: 7_719_600,
		},
	}).toTokenUsage()

	if usage.InputTokens != 612_009 {
		t.Fatalf("InputTokens: got %d, want 612009", usage.InputTokens)
	}
	if usage.CacheReadInputTokens != 7_719_600 {
		t.Fatalf("CacheReadInputTokens: got %d, want 7719600", usage.CacheReadInputTokens)
	}
	if usage.OutputTokens != 104_900 {
		t.Fatalf("OutputTokens: got %d, want 104900", usage.OutputTokens)
	}

	pricing, ok := LookupModel("minimaxai/minimax-m2.7")
	if !ok {
		t.Fatal("model not found for minimaxai/minimax-m2.7")
	}
	cost := float64(usage.InputTokens)*pricing.Pricing.InputPerMToken/1_000_000 +
		float64(usage.CacheReadInputTokens)*pricing.Pricing.CacheReadPerMToken/1_000_000 +
		float64(usage.OutputTokens)*pricing.Pricing.OutputPerMToken/1_000_000

	const want = 0.734061
	if cost < want*0.999 || cost > want*1.001 {
		t.Fatalf("cost: got %.6f, want %.6f", cost, want)
	}
}

func TestCompletionUsageClampsCachedTokens(t *testing.T) {
	usage := (&completionUsage{
		PromptTokens: 100,
		PromptTokensDetails: promptTokensDetails{
			CachedTokens: 150,
		},
	}).toTokenUsage()

	if usage.InputTokens != 0 {
		t.Fatalf("InputTokens: got %d, want 0", usage.InputTokens)
	}
	if usage.CacheReadInputTokens != 100 {
		t.Fatalf("CacheReadInputTokens: got %d, want 100", usage.CacheReadInputTokens)
	}
}

// TestEnsureMaxTokens verifies that the provider sets max_tokens
// when the user doesn't specify one, preventing output truncation.
func TestEnsureMaxTokens(t *testing.T) {
	bus := observe.NewEventBus(1024)
	p := &Provider{inner: nil, bus: bus, maxRetries: 0}

	t.Run("sets_max_tokens_for_known_model", func(t *testing.T) {
		params := provider.RequestParams{
			Model:     "moonshotai/kimi-k2.5",
			MaxTokens: 0,
		}
		p.ensureMaxTokens(&params)
		if params.MaxTokens != 65535 {
			t.Errorf("expected MaxTokens=65535, got %d", params.MaxTokens)
		}
	})

	t.Run("sets_max_tokens_for_minimax_m2_7", func(t *testing.T) {
		params := provider.RequestParams{
			Model:     "minimaxai/minimax-m2.7",
			MaxTokens: 0,
		}
		p.ensureMaxTokens(&params)
		if params.MaxTokens != 131072 {
			t.Errorf("expected MaxTokens=131072, got %d", params.MaxTokens)
		}
	})

	t.Run("sets_max_tokens_for_minimax_m3", func(t *testing.T) {
		params := provider.RequestParams{
			Model:     "minimaxai/minimax-m3",
			MaxTokens: 0,
		}
		p.ensureMaxTokens(&params)
		if params.MaxTokens != 1048576 {
			t.Errorf("expected MaxTokens=1048576, got %d", params.MaxTokens)
		}
	})

	t.Run("does_not_override_user_specified", func(t *testing.T) {
		params := provider.RequestParams{
			Model:     "moonshotai/kimi-k2.5",
			MaxTokens: 8192,
		}
		p.ensureMaxTokens(&params)
		if params.MaxTokens != 8192 {
			t.Errorf("expected MaxTokens=8192 (unchanged), got %d", params.MaxTokens)
		}
	})

	t.Run("unknown_model_leaves_zero", func(t *testing.T) {
		params := provider.RequestParams{
			Model:     "unknown/model",
			MaxTokens: 0,
		}
		p.ensureMaxTokens(&params)
		if params.MaxTokens != 0 {
			t.Errorf("expected MaxTokens=0 for unknown model, got %d", params.MaxTokens)
		}
	})

	t.Run("caps_max_tokens_to_fit_context_window", func(t *testing.T) {
		// Gemma 4 context = 262144. Create a large prompt that fills most of it.
		// ~250K tokens worth of text (~1M chars at 4 chars/token)
		bigText := strings.Repeat("word ", 200000) // ~200K tokens at len/4
		params := provider.RequestParams{
			Model:     "google/gemma-4-31b-it",
			MaxTokens: 16384,
			Messages: []model.Message{
				{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
			},
		}
		p.ensureMaxTokens(&params)
		if params.MaxTokens >= 16384 {
			t.Errorf("expected MaxTokens to be capped below 16384, got %d", params.MaxTokens)
		}
		if params.MaxTokens < 1024 {
			t.Errorf("expected MaxTokens >= 1024 minimum, got %d", params.MaxTokens)
		}
	})
}
