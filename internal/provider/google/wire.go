// Package google implements the provider.Provider interface for the Google Gemini API,
// translating between internal model types and Google's wire format.
package google

import "encoding/json"

// wireRequest is the Google Gemini generateContent request body.
type wireRequest struct {
	Contents          []wireContent    `json:"contents"`
	SystemInstruction *wireContent     `json:"systemInstruction,omitempty"`
	Tools             []wireTool       `json:"tools,omitempty"`
	ToolConfig        *wireToolConfig  `json:"toolConfig,omitempty"`
	GenerationConfig  *wireGenConfig   `json:"generationConfig,omitempty"`
}

// wireContent represents a message in the Gemini API.
type wireContent struct {
	Role  string     `json:"role,omitempty"`
	Parts []wirePart `json:"parts"`
}

// wirePart represents a single content part. Only one field should be set.
type wirePart struct {
	Text             string                `json:"text,omitempty"`
	InlineData       *wireInlineData       `json:"inlineData,omitempty"`
	FunctionCall     *wireFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *wireFunctionResponse `json:"functionResponse,omitempty"`
	Thought          *bool                 `json:"thought,omitempty"`
	ThoughtSignature string                `json:"thoughtSignature,omitempty"` // Gemini 3: must round-trip for function calling
}

// wireInlineData carries inline binary data (images, documents).
type wireInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"` // base64-encoded
}

// wireFunctionCall is a tool invocation from the model.
// Gemini 3+ generates a unique `id` for every function call.
type wireFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
	Id   string          `json:"id,omitempty"` // Gemini 3: unique call ID for matching responses
}

// wireFunctionResponse is the result of a tool invocation.
// The `id` must match the original FunctionCall's `id` (Gemini 3 requirement).
type wireFunctionResponse struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
	Id       string          `json:"id,omitempty"` // must match the originating FunctionCall.Id
}

// wireTool wraps function declarations.
type wireTool struct {
	FunctionDeclarations []wireFunctionDecl `json:"functionDeclarations"`
}

// wireFunctionDecl describes a tool for the model.
type wireFunctionDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// wireToolConfig controls tool usage behavior.
type wireToolConfig struct {
	FunctionCallingConfig *wireFCConfig `json:"functionCallingConfig,omitempty"`
}

// wireFCConfig sets the function calling mode.
type wireFCConfig struct {
	Mode string `json:"mode"` // "AUTO", "ANY", "NONE"
}

// wireGenConfig holds generation parameters.
type wireGenConfig struct {
	MaxOutputTokens int              `json:"maxOutputTokens,omitempty"`
	Temperature     *float64         `json:"temperature,omitempty"`
	ThinkingConfig  *wireThinkConfig `json:"thinkingConfig,omitempty"`
}

// wireThinkConfig controls extended thinking.
// Gemini 2.5 uses thinkingBudget (integer token count).
// Gemini 3 uses thinkingLevel (string: "minimal", "low", "medium", "high").
type wireThinkConfig struct {
	IncludeThoughts bool   `json:"includeThoughts,omitempty"`
	ThinkingBudget  int    `json:"thinkingBudget,omitempty"`  // Gemini 2.5: token count (0=disable, -1=dynamic)
	ThinkingLevel   string `json:"thinkingLevel,omitempty"`   // Gemini 3: "minimal", "low", "medium", "high"
}

// wireResponse is the non-streaming generateContent response.
type wireResponse struct {
	Candidates    []wireCandidate   `json:"candidates"`
	UsageMetadata wireUsageMetadata `json:"usageMetadata"`
	ModelVersion  string            `json:"modelVersion,omitempty"`
}

// wireCandidate holds one candidate response.
type wireCandidate struct {
	Content      wireContent `json:"content"`
	FinishReason string      `json:"finishReason"`
}

// wireUsageMetadata holds token usage information.
type wireUsageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	TotalTokenCount         int `json:"totalTokenCount"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount,omitempty"`
	CachedContentTokenCount int `json:"cachedContentTokenCount,omitempty"`
}

// wireStreamChunk is one chunk in a streaming response.
type wireStreamChunk struct {
	Candidates    []wireCandidate    `json:"candidates,omitempty"`
	UsageMetadata *wireUsageMetadata `json:"usageMetadata,omitempty"`
	ModelVersion  string             `json:"modelVersion,omitempty"`
}

// wireErrorResponse is the Google API error envelope.
type wireErrorResponse struct {
	Error wireErrorDetail `json:"error"`
}

// wireErrorDetail holds error information.
type wireErrorDetail struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}
