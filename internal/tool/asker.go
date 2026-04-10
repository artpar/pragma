package tool

import "context"

// Asker allows a tool to pause and ask the user a question.
// The TUI implements this for interactive mode; non-interactive mode
// returns an error.
type Asker interface {
	Ask(ctx context.Context, question string) (string, error)
}
