package permission

import (
	"context"
	"github.com/artpar/pragma/internal/observe"
)

// Prompter asks the user for a permission decision when a rule evaluates to "ask".
// Returns the user's decision and an optional "remember" rule to add to the session.
type Prompter interface {
	Prompt(ctx context.Context, toolName string, content string, reason string) (Decision, *Rule)
}

// NonInteractivePrompter denies all "ask" decisions in non-interactive mode.
type NonInteractivePrompter struct{}

// Prompt always returns DecisionDeny in non-interactive mode.
func (p *NonInteractivePrompter) Prompt(_ context.Context, _ string, _ string, _ string) (Decision, *Rule) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: DecisionDeny, nil")
	return DecisionDeny, nil
}
