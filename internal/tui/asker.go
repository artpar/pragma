package tui

import (
	"context"
	"errors"
	"sync"

	"github.com/artpar/gogent/internal/observe"
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
	observe.GlobalTrace("return: &InteractiveAsker{}")
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
func (a *InteractiveAsker) Ask(ctx context.Context, question string) (string, error) {
	observe.TraceCtx(ctx, "tui", "InteractiveAsker.Ask", "enter")
	defer observe.TraceCtx(ctx, "tui", "InteractiveAsker.Ask", "exit")
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.program == nil {
		observe.TraceCtx(ctx, "tui", "InteractiveAsker.Ask", "if: a.program == nil")
		observe.TraceCtx(ctx, "tui", "InteractiveAsker.Ask", "return: \"\", errors.New(\"AskUserQuestion requires interactive mode\")")
		observe.TraceCtx(ctx, "tui", "InteractiveAsker.Ask", "return: \"\", errors.New(\"AskUserQuestion requires interactive mode\")")
		observe.TraceCtx(ctx, "tui", "InteractiveAsker.Ask", "return: \"\", errors.New(\"AskUserQuestion requires interactive mode\")")
		return "", errors.New("AskUserQuestion requires interactive mode")
	}

	respCh := make(chan string, 1)
	a.program.Send(AskRequestMsg{
		Question: question,
		Response: respCh,
	})

	select {
	case answer := <-respCh:
		observe.TraceCtx(ctx, "tui", "InteractiveAsker.Ask", "select: answer := <-respCh")
		return answer, nil
	case <-ctx.Done():
		observe.TraceCtx(ctx, "tui", "InteractiveAsker.Ask", "select: <-ctx.Done()")
		return "", ctx.Err()
	}
}

// NonInteractiveAsker returns an error for non-interactive mode.
type NonInteractiveAsker struct{}

// Ask always returns an error in non-interactive mode.
func (a *NonInteractiveAsker) Ask(_ context.Context, _ string) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"\", errors.New(\"AskUserQuestion requires interactive mode\")")
	observe.GlobalTrace("return: \"\", errors.New(\"AskUserQuestion requires interactive mode\")")
	observe.GlobalTrace("return: \"\", errors.New(\"AskUserQuestion requires interactive mode\")")
	return "", errors.New("AskUserQuestion requires interactive mode")
}
