package tui

import (
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/query"
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

// sessionSavedMsg signals that a session save completed.
type sessionSavedMsg struct{}
