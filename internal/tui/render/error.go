package render

import (
	"fmt"
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
	label := errorKindLabel[data.Kind]
	if label == "" {
		label = "Error"
	}

	if data.Retrying {
		return renderRetrying(data, label)
	}
	var b strings.Builder
	return renderTerminal(&b, data, label, verbose)
}

// renderRetrying renders a retry-in-progress error with countdown.
// Layout:
//
//	⎿  Rate limit exceeded
//	   Retrying in 8 seconds… (attempt 5/11)
func renderRetrying(data ErrorData, label string) string {
	var b strings.Builder

	// Error kind label in red
	b.WriteString(bracketErr.Render(BracketPrefix))
	b.WriteString(errBold.Render(label))
	b.WriteString("\n")

	// Countdown line in dim
	unit := "seconds"
	if data.SecondsLeft == 1 {
		unit = "second"
	}
	countdown := fmt.Sprintf("Retrying in %d %s… (attempt %d/%d)",
		max(0, data.SecondsLeft), unit, data.Attempt, data.MaxAttempts)
	b.WriteString(bracketDim.Render(BracketPrefix))
	b.WriteString(bracketDim.Render(countdown))

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
	// Error kind label in red
	b.WriteString(bracketErr.Render(BracketPrefix))
	b.WriteString(errBold.Render(label))
	b.WriteString("\n")

	// Error message
	msg := data.ErrorMsg
	truncated := false
	if !verbose && len(msg) > maxAPIErrorChars {
		msg = msg[:maxAPIErrorChars] + "…"
		truncated = true
	}

	if msg != "" {
		lines := strings.Split(msg, "\n")
		maxLines := len(lines)
		if !verbose && maxLines > 10 {
			maxLines = 10
			truncated = true
		}
		for i := 0; i < maxLines; i++ {
			b.WriteString(bracketErr.Render(BracketPrefix))
			b.WriteString(bracketErr.Render(lines[i]))
			b.WriteString("\n")
		}
		if !verbose && len(lines) > 10 {
			b.WriteString(bracketDim.Render(BracketPrefix))
			b.WriteString(bracketDim.Render(fmt.Sprintf("(+%d more lines)", len(lines)-10)))
			b.WriteString("\n")
		}
	}

	// Expand hint
	if truncated {
		appendExpandHint(b)
		b.WriteString("\n")
	}

	// Guidance
	if data.Guidance != "" {
		b.WriteString(bracketDim.Render(BracketPrefix))
		b.WriteString(bracketDim.Render(data.Guidance))
	}

	return b.String()
}
