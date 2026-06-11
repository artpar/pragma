package hook

import (
	"encoding/json"
	"github.com/artpar/pragma/internal/observe"
)

// Event identifies when a hook fires.
type Event string

const (
	PreToolUse       Event = "PreToolUse"
	PostToolUse      Event = "PostToolUse"
	Stop             Event = "Stop"
	UserPromptSubmit Event = "UserPromptSubmit"
)

// Command is a single hook command from settings.json.
type Command struct {
	Type    string `json:"type"`              // "command" only for now
	Command string `json:"command"`           // shell command to execute
	Timeout int    `json:"timeout,omitempty"` // seconds, default 600
}

// Entry groups a matcher with its hook commands.
// Mirrors the TS settings shape: {"matcher": "Bash", "hooks": [...]}.
type Entry struct {
	Matcher string    `json:"matcher,omitempty"` // tool name pattern (empty = match all)
	Hooks   []Command `json:"hooks"`
}

// HookInput is the JSON payload sent to hooks on stdin.
type HookInput struct {
	Event      Event           `json:"event"`
	SessionID  string          `json:"session_id,omitempty"`
	CWD        string          `json:"cwd"`
	ToolName   string          `json:"tool_name,omitempty"`
	ToolInput  json.RawMessage `json:"tool_input,omitempty"`
	Response   string          `json:"response,omitempty"`    // PostToolUse: tool output
	PromptText string          `json:"prompt_text,omitempty"` // UserPromptSubmit: user's message
}

// Result is the outcome of a single hook execution.
type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
	JSON     *JSONOutput // nil if stdout wasn't valid JSON
	Err      error       // non-nil if execution failed (timeout, spawn error)
}

// JSONOutput is the optional structured response from a hook.
type JSONOutput struct {
	Decision          string `json:"decision,omitempty"`          // "approve"/"block" (PreToolUse)
	Reason            string `json:"reason,omitempty"`            // explanation
	Continue          *bool  `json:"continue,omitempty"`          // false = stop
	StopReason        string `json:"stopReason,omitempty"`        // message when continue=false
	AdditionalContext string `json:"additionalContext,omitempty"` // shown to model
}

// Outcome classifies the hook execution result.
type Outcome string

const (
	OutcomeOK      Outcome = "ok"      // exit 0
	OutcomeBlock   Outcome = "block"   // exit 2
	OutcomeError   Outcome = "error"   // other exit codes
	OutcomeTimeout Outcome = "timeout" // timed out
)

// Outcome returns the classified outcome of a hook result.
func (r Result) Outcome() Outcome {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if r.Err != nil {
		observe.GlobalTrace("if: r.Err != nil")
		observe.GlobalTrace("return: OutcomeTimeout")
		return OutcomeTimeout
	}
	switch r.ExitCode {
	case 0:
		observe.GlobalTrace("case: 0")
		return OutcomeOK
	case 2:
		observe.GlobalTrace("case: 2")
		return OutcomeBlock
	default:
		observe.GlobalTrace("default")
		return OutcomeError
	}
}

// AggregatedResult is the combined result of all hooks for an event.
type AggregatedResult struct {
	Blocked  bool     // any hook returned exit 2
	BlockMsg string   // stderr from the blocking hook
	Feedback []string // additionalContext values from all hooks
	Stdout   string   // stdout from hooks
}
