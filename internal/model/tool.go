package model

import "encoding/json"

// ToolDef describes a tool for LLM requests.
// The provider translates this to its wire format.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}
