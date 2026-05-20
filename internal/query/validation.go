package query

import (
	"fmt"
	"strings"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/tool"
)

type completionValidationState struct {
	pending          bool
	mutationSeq      int
	lastMutationTool string
	lastFeedback     string
}

func (v *completionValidationState) observeToolBatch(registry *tool.Registry, calls []model.ToolCallPart, results []model.ToolResultPart, displays []string) []model.ContentPart {
	var feedback []model.ContentPart
	for i, call := range calls {
		if i >= len(results) {
			continue
		}
		result := results[i]
		display := ""
		if i < len(displays) {
			display = strings.TrimSpace(displays[i])
		}
		failed := result.IsError || strings.HasPrefix(display, "exit_code:") || strings.HasPrefix(display, "timeout:")
		if isFileMutationTool(registry, call.Name) {
			if !failed {
				v.mutationSeq++
				v.pending = true
				v.lastMutationTool = call.Name
			}
			continue
		}
		if call.Name != "Bash" {
			continue
		}
		command := rawStringField(call.Input, "command")
		if failed {
			if v.pending {
				feedback = append(feedback, model.TextPart{Text: validationFeedback("Validation command failed after file changes. Inspect the concrete command output, fix the implementation or command, then run a meaningful validation command again.")})
			}
			continue
		}
		if !looksLikeValidationCommand(command) {
			continue
		}
		if isNoOpValidationOutput(result.Content) {
			msg := "Validation command did not exercise anything meaningful after file changes; commands that report no tests to run do not satisfy completion validation."
			v.lastFeedback = msg
			feedback = append(feedback, model.TextPart{Text: validationFeedback(msg)})
			continue
		}
		v.pending = false
		v.lastFeedback = ""
	}
	return feedback
}

func (v *completionValidationState) shouldBlockCompletion(parts []model.ContentPart) bool {
	if !v.pending {
		return false
	}
	return !assistantReportedValidationImpossible(parts)
}

func (v *completionValidationState) completionPrompt() string {
	if v.lastMutationTool == "" {
		return validationFeedback("Files changed during this task. Run a meaningful validation command before claiming completion, or explicitly state why validation is impossible.")
	}
	return validationFeedback(fmt.Sprintf("Files changed via %s during this task. Run a meaningful validation command after the last file change before claiming completion, or explicitly state why validation is impossible.", v.lastMutationTool))
}

func isFileMutationTool(registry *tool.Registry, name string) bool {
	desc, ok := registry.Get(name)
	if !ok || desc.Flags().ReadOnly {
		return false
	}
	switch name {
	case "apply_patch", "ApplyPatch", "Edit", "Write":
		return true
	default:
		return false
	}
}

func looksLikeValidationCommand(command string) bool {
	command = strings.ToLower(command)
	keywords := []string{
		"go test", "go build", "npm test", "npm run test", "npm run lint", "npm run typecheck",
		"pnpm test", "pnpm lint", "pnpm typecheck", "yarn test", "yarn lint", "yarn typecheck",
		"pytest", "cargo test", "cargo check", "cargo build", "swift test", "mvn test", "gradle test",
	}
	for _, keyword := range keywords {
		if strings.Contains(command, keyword) {
			return true
		}
	}
	return false
}

func isNoOpValidationOutput(output string) bool {
	output = strings.ToLower(output)
	noOps := []string{
		"no tests to run",
		"matched no packages",
		"no test files",
		"0 tests",
		"collected 0 items",
	}
	for _, noOp := range noOps {
		if strings.Contains(output, noOp) {
			return true
		}
	}
	return false
}

func assistantReportedValidationImpossible(parts []model.ContentPart) bool {
	text := strings.ToLower(contentText(parts))
	phrases := []string{
		"validation was impossible",
		"validation is impossible",
		"unable to validate",
		"could not validate",
		"not able to validate",
	}
	for _, phrase := range phrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

func contentText(parts []model.ContentPart) string {
	var b strings.Builder
	for _, part := range parts {
		if text, ok := part.(model.TextPart); ok {
			b.WriteString(text.Text)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func validationFeedback(message string) string {
	return "[system: completion validation required] " + message
}
