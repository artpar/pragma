package query

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
)

func (e *Engine) recordHandoffToolFailures(calls []model.ToolCallPart, results []model.ToolResultPart, displays []string) {
	if !e.isStateHandoffMode() {
		return
	}
	failures := make([]model.HandoffVerifiedFailure, 0)
	for i, call := range calls {
		if i >= len(results) {
			continue
		}
		display := ""
		if i < len(displays) {
			display = displays[i]
		}
		failure, ok := handoffFailureFromToolResult(call, results[i], display)
		if ok {
			failures = append(failures, failure)
		}
	}
	if len(failures) == 0 {
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
	errorType, failed := toolFailureType(result, display)
	if !failed {
		return model.HandoffVerifiedFailure{}, false
	}
	errorMessage := strings.TrimSpace(result.Content)
	if errorMessage == "" {
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
		RepairConstraint:      repairConstraintForTool(call, errorType),
	}
	return failure, true
}

func toolFailureType(result model.ToolResultPart, display string) (string, bool) {
	display = strings.TrimSpace(display)
	if result.IsError {
		return "tool_result_error", true
	}
	switch {
	case strings.HasPrefix(display, "exit_code:"):
		return display, true
	case strings.HasPrefix(display, "timeout:"):
		return display, true
	default:
		return "", false
	}
}

func toolCommandSummary(call model.ToolCallPart) string {
	for _, field := range []string{"command", "script", "file_path", "path"} {
		if value := rawStringField(call.Input, field); value != "" {
			if field == "file_path" || field == "path" {
				return call.Name + " " + value
			}
			return value
		}
	}
	return call.Name
}

func rawStringField(raw json.RawMessage, field string) string {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return ""
	}
	value, ok := doc[field].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func invalidatedAssumptionForTool(call model.ToolCallPart, errorType string) string {
	switch call.Name {
	case "Edit":
		return "The Edit old_string or target region exists exactly in the current file."
	case "Bash", "PowerShell":
		if strings.HasPrefix(errorType, "exit_code:") {
			return "The shell command succeeded."
		}
		if strings.HasPrefix(errorType, "timeout:") {
			return "The shell command completed within the requested timeout."
		}
		return "The shell command executed successfully."
	default:
		return fmt.Sprintf("The %s tool call succeeded.", call.Name)
	}
}

func repairConstraintForTool(call model.ToolCallPart, errorType string) string {
	switch call.Name {
	case "Edit":
		return "Before another Edit, re-read the current file region and use exact current text from the latest Read result."
	case "Bash", "PowerShell":
		if strings.HasPrefix(errorType, "exit_code:") {
			return "Do not treat the command as passed; inspect the reported failure and change the implementation or test invocation before re-running."
		}
		if strings.HasPrefix(errorType, "timeout:") {
			return "Do not blindly retry the timed-out command; narrow the command or increase timeout only after identifying why it ran long."
		}
		return "Inspect the shell failure output and address the concrete error before issuing a similar command."
	default:
		return fmt.Sprintf("Use the verified %s failure to change the next approach before repeating a similar tool call.", call.Name)
	}
}

func appendVerifiedFailure(in []model.HandoffVerifiedFailure, failure model.HandoffVerifiedFailure, limit int) []model.HandoffVerifiedFailure {
	out := in[:0]
	for _, existing := range in {
		if existing.ID == failure.ID && failure.ID != "" {
			continue
		}
		out = append(out, existing)
	}
	out = append(out, failure)
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

func appendUniqueLimited(in []string, value string, limit int) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return in
	}
	out := in[:0]
	for _, existing := range in {
		if strings.TrimSpace(existing) == value {
			continue
		}
		out = append(out, existing)
	}
	out = append(out, value)
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

func safeFailureID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

func compactOneLine(s string, limit int) string {
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	if limit <= 0 || len(s) <= limit {
		return s
	}
	return s[:limit] + "...[truncated]"
}
