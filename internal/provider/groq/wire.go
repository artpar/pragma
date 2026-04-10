// Package groq implements the provider.Provider interface for the Groq API,
// translating between internal model types and Groq's OpenAI-compatible wire format.
package groq

import "encoding/json"

// wireRequest is the Groq chat completions request body.
type wireRequest struct {
	Model               string         `json:"model"`
	Messages            []wireMessage  `json:"messages"`
	Tools               []wireTool     `json:"tools,omitempty"`
	ToolChoice          any            `json:"tool_choice,omitempty"`
	MaxCompletionTokens int            `json:"max_completion_tokens,omitempty"`
	Temperature         *float64       `json:"temperature,omitempty"`
	Stream              bool           `json:"stream"`
	StreamOptions       *wireStreamOpt `json:"stream_options,omitempty"`
	ParallelToolCalls   *bool          `json:"parallel_tool_calls,omitempty"`
	ReasoningEffort     string         `json:"reasoning_effort,omitempty"`
	ReasoningFormat     string         `json:"reasoning_format,omitempty"`
	ResponseFormat      *wireRespFmt   `json:"response_format,omitempty"`
	FrequencyPenalty    float64        `json:"frequency_penalty,omitempty"`
	PresencePenalty     float64        `json:"presence_penalty,omitempty"`
	Stop                []string       `json:"stop,omitempty"`
	Seed                *int           `json:"seed,omitempty"`
}

type wireStreamOpt struct {
	IncludeUsage bool `json:"include_usage"`
}

type wireRespFmt struct {
	Type       string          `json:"type"`
	JSONSchema json.RawMessage `json:"json_schema,omitempty"`
}

// wireMessage represents a message in the Groq API.
// Content is either a string or []wireContentPart depending on the role and content types.
type wireMessage struct {
	Role       string         `json:"role"`
	Content    any            `json:"content"`                // string or []wireContentPart
	Name       string         `json:"name,omitempty"`         // for tool role
	ToolCallID string         `json:"tool_call_id,omitempty"` // for tool role
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`   // for assistant role
	Reasoning  string         `json:"reasoning,omitempty"`    // for assistant role (reasoning models)
}

type wireContentPart struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	ImageURL *wireImageURL `json:"image_url,omitempty"`
}

type wireImageURL struct {
	URL string `json:"url"`
}

type wireToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function wireFunction `json:"function"`
}

type wireFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type wireTool struct {
	Type     string           `json:"type"`
	Function wireToolFunction `json:"function"`
}

type wireToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// wireResponse is the non-streaming chat completion response.
type wireResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []wireChoice `json:"choices"`
	Usage   wireUsage    `json:"usage"`
}

type wireChoice struct {
	Index        int         `json:"index"`
	Message      wireMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type wireUsage struct {
	PromptTokens        int              `json:"prompt_tokens"`
	CompletionTokens    int              `json:"completion_tokens"`
	TotalTokens         int              `json:"total_tokens"`
	PromptTokensDetails *wireTokenDetail `json:"prompt_tokens_details,omitempty"`
}

type wireTokenDetail struct {
	CachedTokens int `json:"cached_tokens"`
}

// wireStreamChunk is one SSE chunk in a streaming response.
type wireStreamChunk struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []wireStreamChoice `json:"choices"`
	Usage   *wireUsage         `json:"usage,omitempty"`
}

type wireStreamChoice struct {
	Index        int              `json:"index"`
	Delta        wireStreamDelta  `json:"delta"`
	FinishReason *string          `json:"finish_reason"`
}

type wireStreamDelta struct {
	Role      string              `json:"role,omitempty"`
	Content   *string             `json:"content,omitempty"`
	ToolCalls []wireToolCallDelta `json:"tool_calls,omitempty"`
	Reasoning *string             `json:"reasoning,omitempty"`
}

type wireToolCallDelta struct {
	Index    int            `json:"index"`
	ID       string         `json:"id,omitempty"`
	Type     string         `json:"type,omitempty"`
	Function *wireFuncDelta `json:"function,omitempty"`
}

type wireFuncDelta struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// wireErrorResponse is the Groq API error envelope.
type wireErrorResponse struct {
	Error wireErrorDetail `json:"error"`
}

type wireErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}
