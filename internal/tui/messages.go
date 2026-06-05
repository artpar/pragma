package tui

import (
	"encoding/json"

	"github.com/artpar/pragma/internal/interactive"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/slash"
	"github.com/artpar/pragma/internal/tool"
)

// LoopEventMsg wraps an interactive event for the bubbletea Update loop.
// nil Event means the event channel was closed (turn finished).
type LoopEventMsg struct {
	Event interactive.Event
}

// PermRequestMsg signals that the orchestrator goroutine needs a permission
// decision from the user. The TUI renders a permission dialog and sends
// the result back on the Response channel.
type PermRequestMsg struct {
	ToolName  string
	ToolInput json.RawMessage // raw tool input for rendering previews
	Content   string
	Reason    string
	Response  chan<- PermResponseMsg
}

// PermResponseMsg carries the user's permission decision back to the
// orchestrator goroutine via the PermRequestMsg.Response channel.
type PermResponseMsg struct {
	Decision permission.Decision
	Rule     *permission.Rule
}

// InputSubmittedMsg carries a user message from the input component.
type InputSubmittedMsg struct {
	Text string
}

// SlashResultMsg carries the result of a slash command execution.
type SlashResultMsg struct {
	Result slash.Result
	Err    error
}

// AskRequestMsg signals that a tool goroutine needs to ask the user a question.
// The TUI renders a question dialog (option selection or free-text) and sends
// the answer back on the Response channel.
type AskRequestMsg struct {
	Request  tool.AskRequest
	Response chan<- tool.AskResponse
}

// quitTimeoutMsg signals that the 800ms Ctrl+C exit window has expired.
type quitTimeoutMsg struct{}

// retryCountdownMsg is sent by tea.Tick every second during retry delay.
// Attempt is a generation counter — stale ticks from cancelled retries are discarded.
type retryCountdownMsg struct {
	SecondsLeft int
	Attempt     int
}
