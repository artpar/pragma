package tui

import (
	"context"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artpar/gogent/internal/permission"
)

// InteractivePrompter implements permission.Prompter by bridging the
// synchronous orchestrator goroutine to the bubbletea event loop.
//
// When the orchestrator calls Prompt(), it sends a PermRequestMsg to bubbletea
// via tea.Program.Send(), then blocks on a response channel. The TUI renders
// a permission dialog, captures the user's decision, and sends the result
// back on the channel.
//
// A mutex serializes concurrent permission requests (from parallel tool
// execution) so only one dialog is shown at a time.
type InteractivePrompter struct {
	mu      sync.Mutex
	program *tea.Program
}

// NewInteractivePrompter creates a prompter. Call SetProgram before use.
func NewInteractivePrompter() *InteractivePrompter {
	return &InteractivePrompter{}
}

// SetProgram binds the bubbletea program. Must be called after tea.NewProgram
// and before any tool execution.
func (p *InteractivePrompter) SetProgram(prog *tea.Program) {
	p.program = prog
}

// Prompt asks the user for a permission decision. It blocks the calling
// goroutine until the user responds or the context is cancelled.
func (p *InteractivePrompter) Prompt(ctx context.Context, toolName, content, reason string) (permission.Decision, *permission.Rule) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.program == nil {
		return permission.DecisionDeny, nil
	}

	respCh := make(chan PermResponseMsg, 1)
	p.program.Send(PermRequestMsg{
		ToolName: toolName,
		Content:  content,
		Reason:   reason,
		Response: respCh,
	})

	select {
	case resp := <-respCh:
		return resp.Decision, resp.Rule
	case <-ctx.Done():
		return permission.DecisionDeny, nil
	}
}
