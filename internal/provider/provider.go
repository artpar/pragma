package provider

import (
	"context"
	"encoding/json"

	"github.com/artpar/pragma/internal/model"
)

// Provider is the translation boundary between internal model types and
// LLM wire formats. Only provider adapters know about wire formats.
type Provider interface {
	Name() string
	Stream(ctx context.Context, params RequestParams) (<-chan StreamChunk, error)
	Complete(ctx context.Context, params RequestParams) (model.Response, error)
	SupportsFeature(feature Feature) bool
	Pricing(modelID string) (model.Pricing, bool)
	// ContextWindow returns the context window size in tokens for the given model.
	// Must parse model variant suffixes like [1m] (GitHub issue #41984, #39467).
	// Returns (0, false) if the model is unknown.
	ContextWindow(modelID string) (int, bool)
}

// Feature flags that providers may or may not support.
type Feature string

const (
	FeaturePrefixCaching  Feature = "prefix_caching"
	FeatureThinking       Feature = "thinking"
	FeatureImages         Feature = "images"
	FeatureToolUse        Feature = "tool_use"
	FeatureStreaming       Feature = "streaming"
	FeatureStructuredOutput Feature = "structured_output"
)

// TokenCounter is an optional interface providers can implement for precise token counting.
// Providers that don't implement this fall back to heuristic estimation.
type TokenCounter interface {
	CountTokens(ctx context.Context, params RequestParams) (int, error)
}

// RequestParams carries all data needed for an LLM request, in internal types.
type RequestParams struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	Messages    []model.Message    `json:"messages"`
	System      model.SystemPrompt `json:"system"`
	Tools       []model.ToolDef    `json:"tools,omitempty"`
	Temperature    *float64           `json:"temperature,omitempty"`
	Thinking       *ThinkingConfig    `json:"thinking,omitempty"`
	ResponseSchema json.RawMessage    `json:"response_schema,omitempty"`
}

// ThinkingConfig controls extended thinking / reasoning.
type ThinkingConfig struct {
	Enabled      bool `json:"enabled"`
	BudgetTokens int  `json:"budget_tokens,omitempty"`
}

// StreamChunk is one piece of a streaming response.
// Flat struct — exactly one field is non-zero per chunk.
// Cheaper than interface boxing for high-frequency channel values.
type StreamChunk struct {
	TextDelta              string
	ThinkingDelta          string
	ThinkingSignatureDelta string
	ToolCallStart          *model.ToolCallPart
	ToolCallInputDelta     *ToolCallDelta
	RedactedThinkingBlock  *RedactedThinking
	Done                   *StreamDone
	Error                  error
}

// ToolCallDelta carries incremental JSON for an in-progress tool call.
type ToolCallDelta struct {
	ToolCallID string
	JSONDelta  string
}

// RedactedThinking carries opaque data for a provider-redacted thinking block.
// Arrives as a single content_block_start event (not accumulated from deltas).
type RedactedThinking struct {
	Data string
}

// StreamDone signals the end of a stream with final metadata.
type StreamDone struct {
	StopReason model.StopReason
	Usage      model.TokenUsage
	Model      string
}
