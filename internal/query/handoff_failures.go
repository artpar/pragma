package query

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

func (e *Engine) recordHandoffToolFailures(calls []model.ToolCallPart, results []model.ToolResultPart, displays []string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !e.isStateHandoffMode() {
		observe.GlobalTrace("if: !e.isStateHandoffMode()")
		return
	}
	failures := make([]model.HandoffVerifiedFailure, 0)
	for i, call := range calls {
		observe.GlobalTrace("range calls")
		if i >= len(results) {
			observe.GlobalTrace("if: i >= len(results)")
			continue
		}
		display := ""
		if i < len(displays) {
			observe.GlobalTrace("if: i < len(displays)")
			display = displays[i]
		}
		failure, ok := handoffFailureFromToolResult(call, results[i], display)
		if ok {
			observe.GlobalTrace("if: ok")
			failures = append(failures, failure)
		}
	}
	if len(failures) == 0 {
		observe.GlobalTrace("if: len(failures) == 0")
		return
	}
	e.store.Update(func(s *app.AppState) {
		if s.HandoffState.IsZero() {
			s.HandoffState = model.NewHandoffState("")
		}
		for _, failure := range failures {
			s.HandoffState.VerifiedFailures = appendVerifiedFailure(s.HandoffState.VerifiedFailures, failure, 8)
			s.HandoffState.InvalidatedAssumptions = appendUniqueLimited(
				s.HandoffState.InvalidatedAssumptions,
				failure.InvalidatedAssumption,
				12,
			)
			s.HandoffState.RepairConstraints = appendUniqueLimited(
				s.HandoffState.RepairConstraints,
				failure.RepairConstraint,
				12,
			)
		}
		latest := failures[len(failures)-1]
		s.HandoffState.LatestToolResultInterpretation = latest.ErrorMessage
		if latest.RepairConstraint != "" {
			s.HandoffState.NextAction = latest.RepairConstraint
		}
	})
}

func handoffFailureFromToolResult(call model.ToolCallPart, result model.ToolResultPart, display string) (model.HandoffVerifiedFailure, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	errorType, failed := toolFailureType(result, display)
	if !failed {
		observe.GlobalTrace("if: !failed")
		observe.GlobalTrace("return: model.HandoffVerifiedFailure{}, false")
		return model.HandoffVerifiedFailure{}, false
	}
	errorMessage := strings.TrimSpace(result.Content)
	if errorMessage == "" {
		observe.GlobalTrace("if: errorMessage == \"\"")
		errorMessage = errorType
	}
	failure := model.HandoffVerifiedFailure{
		ID:                    "tool_failure_" + safeFailureID(call.ID),
		ToolCallID:            call.ID,
		ToolName:              call.Name,
		Command:               toolCommandSummary(call),
		ErrorType:             errorType,
		ErrorMessage:          compactOneLine(errorMessage, 500),
		OutputExcerpt:         compactOneLine(result.Content, 1200),
		InvalidatedAssumption: invalidatedAssumptionForTool(call, errorType),
		RepairConstraint:      repairConstraintForTool(call, errorType, result.Content),
	}
	observe.GlobalTrace("return: failure, true")
	return failure, true
}

func toolFailureType(result model.ToolResultPart, display string) (string, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	display = strings.TrimSpace(display)
	if result.IsError {
		observe.GlobalTrace("if: result.IsError")
		observe.GlobalTrace("return: \"tool_result_error\", true")
		return "tool_result_error", true
	}
	switch {
	case strings.HasPrefix(display, "exit_code:"):
		observe.GlobalTrace("case: strings.HasPrefix(display, \"exit_code:\")")
		return display, true
	case strings.HasPrefix(display, "timeout:"):
		observe.GlobalTrace("case: strings.HasPrefix(display, \"timeout:\")")
		return display, true
	default:
		observe.GlobalTrace("default")
		return "", false
	}
}

func toolCommandSummary(call model.ToolCallPart) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, field := range []string{"command", "script", "file_path", "path"} {
		observe.GlobalTrace("range []string{\"command\", \"script\", \"file_path\", \"path\"}")
		if value := rawStringField(call.Input, field); value != "" {
			observe.GlobalTrace("if: value != \"\"")
			if field == "file_path" || field == "path" {
				observe.GlobalTrace("if: field == \"file_path\" || field == \"path\"")
				observe.GlobalTrace("return: call.Name + \" \" + value")
				return call.Name + " " + value
			}
			observe.GlobalTrace("return: value")
			return value
		}
	}
	observe.GlobalTrace("return: call.Name")
	return call.Name
}

func rawStringField(raw json.RawMessage, field string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	value, ok := doc[field].(string)
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	observe.GlobalTrace("return: strings.TrimSpace(value)")
	return strings.TrimSpace(value)
}

func invalidatedAssumptionForTool(call model.ToolCallPart, errorType string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch call.Name {
	case "Edit":
		observe.GlobalTrace("case: \"Edit\"")
		return "The Edit old_string or target region exists exactly in the current file."
	case "Bash", "PowerShell":
		observe.GlobalTrace("case: \"Bash\", \"PowerShell\"")
		if strings.HasPrefix(errorType, "exit_code:") {
			observe.GlobalTrace("return: \"The shell command succeeded.\"")
			return "The shell command succeeded."
		}
		if strings.HasPrefix(errorType, "timeout:") {
			observe.GlobalTrace("return: \"The shell command completed within the requested timeout.\"")
			return "The shell command completed within the requested timeout."
		}
		return "The shell command executed successfully."
	default:
		observe.GlobalTrace("default")
		return fmt.Sprintf("The %s tool call succeeded.", call.Name)
	}
}

func repairConstraintForTool(call model.ToolCallPart, errorType, output string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch call.Name {
	case "Edit":
		observe.GlobalTrace("case: \"Edit\"")
		if retry := extractEditRetryCandidate(output); retry != "" {
			return "Retry the Edit using this exact old_string from the previous Edit error:\n```\n" + retry + "\n```"
		}
		return "Before another Edit, re-read the current file region and use exact current text from the latest Read result."
	case "Bash", "PowerShell":
		observe.GlobalTrace("case: \"Bash\", \"PowerShell\"")
		if strings.HasPrefix(errorType, "exit_code:") {
			observe.GlobalTrace("return: \"Do not treat the command as passed; inspect the reported failure and change ...")
			return "Do not treat the command as passed; inspect the reported failure and change the implementation or test invocation before re-running."
		}
		if strings.HasPrefix(errorType, "timeout:") {
			observe.GlobalTrace("return: \"Do not blindly retry the timed-out command; narrow the command or increase t...")
			return "Do not blindly retry the timed-out command; narrow the command or increase timeout only after identifying why it ran long."
		}
		return "Inspect the shell failure output and address the concrete error before issuing a similar command."
	default:
		observe.GlobalTrace("default")
		return fmt.Sprintf("Use the verified %s failure to change the next approach before repeating a similar tool call.", call.Name)
	}
}

func extractEditRetryCandidate(output string) string {
	const marker = "Retry with this exact old_string:\n```\n"
	start := strings.Index(output, marker)
	if start == -1 {
		return ""
	}
	start += len(marker)
	end := strings.Index(output[start:], "\n```")
	if end == -1 {
		return ""
	}
	return strings.Trim(output[start:start+end], "\r\n")
}

func appendVerifiedFailure(in []model.HandoffVerifiedFailure, failure model.HandoffVerifiedFailure, limit int) []model.HandoffVerifiedFailure {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	out := in[:0]
	for _, existing := range in {
		observe.GlobalTrace("range in")
		if existing.ID == failure.ID && failure.ID != "" {
			observe.GlobalTrace("if: existing.ID == failure.ID && failure.ID != \"\"")
			continue
		}
		out = append(out, existing)
	}
	out = append(out, failure)
	if limit > 0 && len(out) > limit {
		observe.GlobalTrace("if: limit > 0 && len(out) > limit")
		out = out[len(out)-limit:]
	}
	observe.GlobalTrace("return: out")
	return out
}

func appendUniqueLimited(in []string, value string, limit int) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	value = strings.TrimSpace(value)
	if value == "" {
		observe.GlobalTrace("if: value == \"\"")
		observe.GlobalTrace("return: in")
		return in
	}
	out := in[:0]
	for _, existing := range in {
		observe.GlobalTrace("range in")
		if strings.TrimSpace(existing) == value {
			observe.GlobalTrace("if: strings.TrimSpace(existing) == value")
			continue
		}
		out = append(out, existing)
	}
	out = append(out, value)
	if limit > 0 && len(out) > limit {
		observe.GlobalTrace("if: limit > 0 && len(out) > limit")
		out = out[len(out)-limit:]
	}
	observe.GlobalTrace("return: out")
	return out
}

func safeFailureID(id string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	id = strings.TrimSpace(id)
	if id == "" {
		observe.GlobalTrace("if: id == \"\"")
		observe.GlobalTrace("return: \"unknown\"")
		return "unknown"
	}
	var b strings.Builder
	for _, r := range id {
		observe.GlobalTrace("range id")
		switch {
		case r >= 'a' && r <= 'z':
			observe.GlobalTrace("case: r >= 'a' && r <= 'z'")
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			observe.GlobalTrace("case: r >= 'A' && r <= 'Z'")
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			observe.GlobalTrace("case: r >= '0' && r <= '9'")
			b.WriteRune(r)
		case r == '-' || r == '_':
			observe.GlobalTrace("case: r == '-' || r == '_'")
			b.WriteRune(r)
		default:
			observe.GlobalTrace("default")
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		observe.GlobalTrace("if: b.Len() == 0")
		observe.GlobalTrace("return: \"unknown\"")
		return "unknown"
	}
	observe.GlobalTrace("return: b.String()")
	return b.String()
}

func compactOneLine(s string, limit int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	if limit <= 0 || len(s) <= limit {
		observe.GlobalTrace("if: limit <= 0 || len(s) <= limit")
		observe.GlobalTrace("return: s")
		return s
	}
	observe.GlobalTrace("return: s[:limit] + \"...[truncated]\"")
	return s[:limit] + "...[truncated]"
}
