package permission

import (
	"context"
	"encoding/json"

	"github.com/artpar/pragma/internal/observe"
)

// Prompter asks the user for a permission decision when a rule evaluates to "ask".
// Returns the user's decision and the explicit scope for any remembered rule.
type Prompter interface {
	Prompt(ctx context.Context, toolName string, toolInput json.RawMessage, content string, reason string) (Decision, RememberScope)
}

// NonInteractivePrompter denies all "ask" decisions in non-interactive mode.
type NonInteractivePrompter struct{}

// Prompt always returns DecisionDeny in non-interactive mode.
func (p *NonInteractivePrompter) Prompt(_ context.Context, _ string, _ json.RawMessage, _ string, _ string) (Decision, RememberScope) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: DecisionDeny, RememberNone")
	return DecisionDeny, RememberNone
}
