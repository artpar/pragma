package tui

import (
	"context"
	"errors"
	"sync"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/tool"
	tea "github.com/charmbracelet/bubbletea"
)

// InteractiveAsker implements tool.Asker by bridging the synchronous tool
// goroutine to the bubbletea event loop. Same pattern as InteractivePrompter.
type InteractiveAsker struct {
	mu      sync.Mutex
	program *tea.Program
}

// NewInteractiveAsker creates an asker. Call SetProgram before use.
func NewInteractiveAsker() *InteractiveAsker {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &InteractiveAsker{}")
	return &InteractiveAsker{}
}

// SetProgram binds the bubbletea program. Must be called after tea.NewProgram.
func (a *InteractiveAsker) SetProgram(prog *tea.Program) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	a.program = prog
}

// Ask sends a question to the TUI and blocks until the user answers.
func (a *InteractiveAsker) Ask(ctx context.Context, req tool.AskRequest) (tool.AskResponse, error) {
	observe.TraceCtx(ctx, "tui", "InteractiveAsker.Ask", "enter")
	defer observe.TraceCtx(ctx, "tui", "InteractiveAsker.Ask", "exit")
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.program == nil {
		observe.TraceCtx(ctx, "tui", "InteractiveAsker.Ask", "if: a.program == nil")
		observe.TraceCtx(ctx, "tui", "InteractiveAsker.Ask", "return: tool.AskResponse{}, errors.New(\"AskUserQuestion requires interactive mode\")")
		return tool.AskResponse{}, errors.New("AskUserQuestion requires interactive mode")
	}

	respCh := make(chan tool.AskResponse, 1)
	a.program.Send(AskRequestMsg{
		Request:  req,
		Response: respCh,
	})

	select {
	case answer := <-respCh:
		observe.TraceCtx(ctx, "tui", "InteractiveAsker.Ask", "select: answer := <-respCh")
		return answer, nil
	case <-ctx.Done():
		observe.TraceCtx(ctx, "tui", "InteractiveAsker.Ask", "select: <-ctx.Done()")
		return tool.AskResponse{}, ctx.Err()
	}
}

// NonInteractiveAsker returns an error for non-interactive mode.
type NonInteractiveAsker struct{}

// Ask always returns an error in non-interactive mode.
func (a *NonInteractiveAsker) Ask(_ context.Context, _ tool.AskRequest) (tool.AskResponse, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.AskResponse{}, errors.New(\"AskUserQuestion requires interactive mode\")")
	return tool.AskResponse{}, errors.New("AskUserQuestion requires interactive mode")
}
