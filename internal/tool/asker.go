package tool

import "context"

// AskOption is a selectable choice for a question.
type AskOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// AskQuestion is a structured question with optional selectable options.
// When Options is empty, the question is free-text only.
type AskQuestion struct {
	Question    string      `json:"question"`
	Header      string      `json:"header,omitempty"`      // chip label (max ~12 chars)
	Options     []AskOption `json:"options,omitempty"`      // 2-4 options
	MultiSelect bool        `json:"multiSelect,omitempty"` // allow multiple selections
}

// AskRequest carries one or more questions from tool to TUI.
// Legacy: Question set, Questions empty → plain free-text input.
// Structured: Questions populated → option selection UI.
type AskRequest struct {
	Question  string        // legacy plain text (no options)
	Questions []AskQuestion // structured questions with selectable options
}

// AskResponse carries answers back from TUI to tool.
// Keys are question text, values are selected option labels or typed text.
// For multi-select questions, values are comma-separated labels.
type AskResponse struct {
	Answers map[string]string
}

// Asker allows a tool to pause and ask the user a question.
// The TUI implements this for interactive mode; non-interactive mode
// returns an error.
type Asker interface {
	Ask(ctx context.Context, req AskRequest) (AskResponse, error)
}
