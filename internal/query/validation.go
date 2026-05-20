package query

import (
	"fmt"
	"strings"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/tool"
)

type completionValidationState struct {
	pending          bool
	mutationSeq      int
	lastMutationTool string
	lastFeedback     string
}

func (v *completionValidationState) observeToolBatch(registry *tool.Registry, calls []model.ToolCallPart, results []model.ToolResultPart, displays []string) []model.ContentPart {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var feedback []model.ContentPart
	for i, call := range calls {
		observe.GlobalTrace("range calls")
		if i >= len(results) {
			observe.GlobalTrace("if: i >= len(results)")
			continue
		}
		result := results[i]
		display := ""
		if i < len(displays) {
			observe.GlobalTrace("if: i < len(displays)")
			display = strings.TrimSpace(displays[i])
		}
		failed := result.IsError || strings.HasPrefix(display, "exit_code:") || strings.HasPrefix(display, "timeout:")
		if isFileMutationTool(registry, call.Name) {
			observe.GlobalTrace("if: isFileMutationTool(registry, call.Name)")
			if !failed {
				observe.GlobalTrace("if: !failed")
				v.mutationSeq++
				v.pending = true
				v.lastMutationTool = call.Name
			}
			continue
		}
		if call.Name != "Bash" {
			observe.GlobalTrace("if: call.Name != \"Bash\"")
			continue
		}
		command := rawStringField(call.Input, "command")
		if failed {
			observe.GlobalTrace("if: failed")
			if v.pending {
				observe.GlobalTrace("if: v.pending")
				feedback = append(feedback, model.TextPart{Text: validationFeedback("Validation command failed after file changes. Inspect the concrete command output, fix the implementation or command, then run a meaningful validation command again.")})
			}
			continue
		}
		if !looksLikeValidationCommand(command) {
			observe.GlobalTrace("if: !looksLikeValidationCommand(command)")
			continue
		}
		if isNoOpValidationOutput(result.Content) {
			observe.GlobalTrace("if: isNoOpValidationOutput(result.Content)")
			msg := "Validation command did not exercise anything meaningful after file changes; commands that report no tests to run do not satisfy completion validation."
			v.lastFeedback = msg
			feedback = append(feedback, model.TextPart{Text: validationFeedback(msg)})
			continue
		}
		v.pending = false
		v.lastFeedback = ""
	}
	observe.GlobalTrace("return: feedback")
	return feedback
}

func (v *completionValidationState) shouldBlockCompletion(parts []model.ContentPart) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !v.pending {
		observe.GlobalTrace("if: !v.pending")
		observe.GlobalTrace("return: false")
		return false
	}
	observe.GlobalTrace("return: !assistantReportedValidationImpossible(parts)")
	return !assistantReportedValidationImpossible(parts)
}

func (v *completionValidationState) completionPrompt() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if v.lastMutationTool == "" {
		observe.GlobalTrace("if: v.lastMutationTool == \"\"")
		observe.GlobalTrace("return: validationFeedback(\"Files changed during this task. Run a meaningful validati...")
		return validationFeedback("Files changed during this task. Run a meaningful validation command before claiming completion, or explicitly state why validation is impossible.")
	}
	observe.GlobalTrace("return: validationFeedback(fmt.Sprintf(\"Files changed via %s during this task. Run a ...")
	return validationFeedback(fmt.Sprintf("Files changed via %s during this task. Run a meaningful validation command after the last file change before claiming completion, or explicitly state why validation is impossible.", v.lastMutationTool))
}

func isFileMutationTool(registry *tool.Registry, name string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	desc, ok := registry.Get(name)
	if !ok || desc.Flags().ReadOnly {
		observe.GlobalTrace("if: !ok || desc.Flags().ReadOnly")
		observe.GlobalTrace("return: false")
		return false
	}
	switch name {
	case "apply_patch", "ApplyPatch", "Edit", "Write":
		observe.GlobalTrace("case: \"apply_patch\", \"ApplyPatch\", \"Edit\", \"Write\"")
		return true
	default:
		observe.GlobalTrace("default")
		return false
	}
}

func looksLikeValidationCommand(command string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	command = strings.ToLower(command)
	keywords := []string{
		"go test", "go build", "npm test", "npm run test", "npm run lint", "npm run typecheck",
		"pnpm test", "pnpm lint", "pnpm typecheck", "yarn test", "yarn lint", "yarn typecheck",
		"pytest", "cargo test", "cargo check", "cargo build", "swift test", "mvn test", "gradle test",
	}
	for _, keyword := range keywords {
		observe.GlobalTrace("range keywords")
		if strings.Contains(command, keyword) {
			observe.GlobalTrace("if: strings.Contains(command, keyword)")
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func isNoOpValidationOutput(output string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	output = strings.ToLower(output)
	noOps := []string{
		"no tests to run",
		"matched no packages",
		"no test files",
		"0 tests",
		"collected 0 items",
	}
	for _, noOp := range noOps {
		observe.GlobalTrace("range noOps")
		if strings.Contains(output, noOp) {
			observe.GlobalTrace("if: strings.Contains(output, noOp)")
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func assistantReportedValidationImpossible(parts []model.ContentPart) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	text := strings.ToLower(contentText(parts))
	phrases := []string{
		"validation was impossible",
		"validation is impossible",
		"unable to validate",
		"could not validate",
		"not able to validate",
	}
	for _, phrase := range phrases {
		observe.GlobalTrace("range phrases")
		if strings.Contains(text, phrase) {
			observe.GlobalTrace("if: strings.Contains(text, phrase)")
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func contentText(parts []model.ContentPart) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var b strings.Builder
	for _, part := range parts {
		observe.GlobalTrace("range parts")
		if text, ok := part.(model.TextPart); ok {
			observe.GlobalTrace("if: ok")
			b.WriteString(text.Text)
			b.WriteByte('\n')
		}
	}
	observe.GlobalTrace("return: b.String()")
	return b.String()
}

func validationFeedback(message string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"[system: completion validation required] \" + message")
	return "[system: completion validation required] " + message
}
