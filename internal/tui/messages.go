package tui

import (
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/slash"
)

// LoopEventMsg wraps a query.LoopEvent for the bubbletea Update loop.
// nil Event means the event channel was closed (turn finished).
type LoopEventMsg struct {
	Event query.LoopEvent
}

// PermRequestMsg signals that the orchestrator goroutine needs a permission
// decision from the user. The TUI renders a permission dialog and sends
// the result back on the Response channel.
type PermRequestMsg struct {
	ToolName string
	Content  string
	Reason   string
	Response chan<- PermResponseMsg
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
// The TUI renders a question dialog and sends the answer back on the Response channel.
type AskRequestMsg struct {
	Question string
	Response chan<- string
}

// sessionSavedMsg signals that a session save completed.
type sessionSavedMsg struct{}

// quitTimeoutMsg signals that the 800ms Ctrl+C exit window has expired.
type quitTimeoutMsg struct{}
