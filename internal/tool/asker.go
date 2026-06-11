package tool

import (
	"context"
	"errors"
	"github.com/artpar/pragma/internal/observe"
)

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
	Options     []AskOption `json:"options,omitempty"`     // 2-4 options
	MultiSelect bool        `json:"multiSelect,omitempty"` // allow multiple selections
}

// AskRequest carries one or more questions from a tool to the interactive UI.
// Legacy: Question set, Questions empty → plain free-text input.
// Structured: Questions populated → option selection UI.
type AskRequest struct {
	Question  string        // legacy plain text (no options)
	Questions []AskQuestion // structured questions with selectable options
}

// AskResponse carries answers back from the interactive UI to the tool.
// Keys are question text, values are selected option labels or typed text.
// For multi-select questions, values are comma-separated labels.
type AskResponse struct {
	Answers map[string]string
}

// Asker allows a tool to pause and ask the user a question.
// Interactive presentations implement this; non-interactive mode returns an error.
type Asker interface {
	Ask(ctx context.Context, req AskRequest) (AskResponse, error)
}

// NonInteractiveAsker rejects AskUserQuestion in non-interactive command paths.
type NonInteractiveAsker struct{}

// Ask always returns an error because there is no interactive UI to answer it.
func (a *NonInteractiveAsker) Ask(_ context.Context, _ AskRequest) (AskResponse, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: AskResponse{}, errors.New(\"AskUserQuestion requires interactive mode\")")
	return AskResponse{}, errors.New("AskUserQuestion requires interactive mode")
}
