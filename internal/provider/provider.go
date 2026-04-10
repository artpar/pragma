package provider

import (
	"context"

	"github.com/artpar/gogent/internal/model"
)

// Provider is the translation boundary between internal model types and
// LLM wire formats. Only provider adapters know about wire formats.
type Provider interface {
	Name() string
	Stream(ctx context.Context, params RequestParams) (<-chan StreamChunk, error)
	Complete(ctx context.Context, params RequestParams) (model.Response, error)
	SupportsFeature(feature Feature) bool
	Pricing(modelID string) model.Pricing
}

// Feature flags that providers may or may not support.
type Feature string

const (
	FeaturePrefixCaching Feature = "prefix_caching"
	FeatureThinking      Feature = "thinking"
	FeatureImages        Feature = "images"
	FeatureToolUse       Feature = "tool_use"
	FeatureStreaming      Feature = "streaming"
)

// RequestParams carries all data needed for an LLM request, in internal types.
type RequestParams struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	Messages    []model.Message    `json:"messages"`
	System      model.SystemPrompt `json:"system"`
	Tools       []model.ToolDef    `json:"tools,omitempty"`
	Temperature *float64           `json:"temperature,omitempty"`
	Thinking    *ThinkingConfig    `json:"thinking,omitempty"`
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
	Done                   *StreamDone
	Error                  error
}

// ToolCallDelta carries incremental JSON for an in-progress tool call.
type ToolCallDelta struct {
	ToolCallID string
	JSONDelta  string
}

// StreamDone signals the end of a stream with final metadata.
type StreamDone struct {
	StopReason model.StopReason
	Usage      model.TokenUsage
}
