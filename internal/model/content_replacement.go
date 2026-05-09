package model

// ContentReplacementRecord records a model-visible content replacement whose
// full content was persisted elsewhere. Records are replayed on resume so the
// same tool_use_id gets the same prompt bytes.
type ContentReplacementRecord struct {
	Kind        string `json:"kind"`
	ToolUseID   string `json:"tool_use_id"`
	Replacement string `json:"replacement"`
}

const ContentReplacementKindToolResult = "tool-result"
