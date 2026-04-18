package render

import (
	"fmt"
	"github.com/artpar/pragma/internal/observe"
	"strings"
)

// maxAPIErrorChars is the truncation threshold for error messages in non-verbose mode.
// Matches TS MAX_API_ERROR_CHARS = 1000.
const maxAPIErrorChars = 1000

// ErrorData holds the data needed to render an error segment.
// Uses string for Kind to avoid importing query/ (render package has no internal deps).
type ErrorData struct {
	Kind        string
	ErrorMsg    string
	Guidance    string
	Attempt     int
	MaxAttempts int
	SecondsLeft int
	Retrying    bool
}

// errorKindLabel maps ErrorKind strings to human-readable labels.
var errorKindLabel = map[string]string{
	"rate_limit":       "Rate limit exceeded",
	"overloaded":       "Server overloaded",
	"authentication":   "Authentication failed",
	"context_overflow": "Context too long",
	"connection":       "Connection error",
	"server_error":     "Server error",
	"unknown":          "Error",
}

// RenderError renders a classified error with optional retry state.
// Matches TS SystemAPIErrorMessage layout:
//   - Error kind label in red
//   - Error message (truncated at 1000 chars in non-verbose with ctrl+o hint)
//   - Retry countdown if retrying
//   - Guidance line if present
func RenderError(data ErrorData, verbose bool, width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	label := errorKindLabel[data.Kind]
	if label == "" {
		observe.GlobalTrace("if: label == \"\"")
		label = "Error"
	}

	if data.Retrying {
		observe.GlobalTrace("if: data.Retrying")
		observe.GlobalTrace("return: renderRetrying(data, label)")
		return renderRetrying(data, label)
	}
	var b strings.Builder
	observe.GlobalTrace("return: renderTerminal(&b, data, label, verbose)")
	return renderTerminal(&b, data, label, verbose)
}

// renderRetrying renders a retry-in-progress error with countdown.
// Layout:
//
//	⎿  Rate limit exceeded
//	   Retrying in 8 seconds… (attempt 5/11)
func renderRetrying(data ErrorData, label string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var b strings.Builder

	b.WriteString(bracketErr.Render(BracketPrefix))
	b.WriteString(errBold.Render(label))
	b.WriteString("\n")

	unit := "seconds"
	if data.SecondsLeft == 1 {
		observe.GlobalTrace("if: data.SecondsLeft == 1")
		unit = "second"
	}
	countdown := fmt.Sprintf("Retrying in %d %s… (attempt %d/%d)",
		max(0, data.SecondsLeft), unit, data.Attempt, data.MaxAttempts)
	b.WriteString(bracketDim.Render(BracketPrefix))
	b.WriteString(bracketDim.Render(countdown))
	observe.GlobalTrace("return: b.String()")

	return b.String()
}

// renderTerminal renders a terminal (non-retrying) error.
// Layout:
//
//	⎿  Authentication failed
//	   API Error: 401 Unauthorized…
//	   (ctrl+o to expand)
//	   Check your API key or run /doctor
func renderTerminal(b *strings.Builder, data ErrorData, label string, verbose bool) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	b.WriteString(bracketErr.Render(BracketPrefix))
	b.WriteString(errBold.Render(label))
	b.WriteString("\n")

	msg := data.ErrorMsg
	truncated := false
	if !verbose && len(msg) > maxAPIErrorChars {
		observe.GlobalTrace("if: !verbose && len(msg) > maxAPIErrorChars")
		msg = msg[:maxAPIErrorChars] + "…"
		truncated = true
	}

	if msg != "" {
		observe.GlobalTrace("if: msg != \"\"")
		lines := strings.Split(msg, "\n")
		maxLines := len(lines)
		if !verbose && maxLines > 10 {
			observe.GlobalTrace("if: !verbose && maxLines > 10")
			maxLines = 10
			truncated = true
		}
		for i := 0; i < maxLines; i++ {
			observe.GlobalTrace("for: i < maxLines")
			b.WriteString(bracketErr.Render(BracketPrefix))
			b.WriteString(bracketErr.Render(lines[i]))
			b.WriteString("\n")
		}
		if !verbose && len(lines) > 10 {
			observe.GlobalTrace("if: !verbose && len(lines) > 10")
			b.WriteString(bracketDim.Render(BracketPrefix))
			b.WriteString(bracketDim.Render(fmt.Sprintf("(+%d more lines)", len(lines)-10)))
			b.WriteString("\n")
		}
	}

	if truncated {
		observe.GlobalTrace("if: truncated")
		appendExpandHint(b)
		b.WriteString("\n")
	}

	if data.Guidance != "" {
		observe.GlobalTrace("if: data.Guidance != \"\"")
		b.WriteString(bracketDim.Render(BracketPrefix))
		b.WriteString(bracketDim.Render(data.Guidance))
	}
	observe.GlobalTrace("return: b.String()")

	return b.String()
}
