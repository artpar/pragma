package compact

import (
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

// PendingUnansweredUserPrompts returns the operator prompts waiting for a
// model response when compaction fires: user messages that sit after the
// conversation's last assistant message, carry no tool results, and are not
// internal (CMP-001.2 F3). Tool-result messages are deliberately excluded —
// re-appending them without their tool_use assistant message would break
// tool_result pairing; they stay part of the summarized history.
//
// One definition serves BOTH compaction paths since CMP-001.2.F3: the
// auto-compaction loops and manual /compact (compact.ApplyResult) re-append
// the same pending tail after the summary, so an operator prompt left
// unanswered by an errored turn is never compacted away.
func PendingUnansweredUserPrompts(messages []model.Message) []model.Message {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	tailStart := len(messages)
	for tailStart > 0 && messages[tailStart-1].Role == model.RoleUser {
		observe.GlobalTrace("for: tailStart > 0 && messages[tailStart-1].Role == model.RoleUser")
		tailStart--
	}
	if tailStart == len(messages) {
		observe.GlobalTrace("if: tailStart == len(messages)")
		observe.GlobalTrace("return: nil")
		return nil
	}
	var pending []model.Message
	for _, msg := range messages[tailStart:] {
		observe.GlobalTrace("range messages[tailStart:]")
		if msg.Flags.IsInternal || MessageHasToolResult(msg) {
			observe.GlobalTrace("if: msg.Flags.IsInternal || MessageHasToolResult(msg)")
			continue
		}
		pending = append(pending, msg)
	}
	observe.GlobalTrace("return: pending")
	return pending
}

// MessageHasToolResult reports whether a message carries any tool-result
// content part.
func MessageHasToolResult(msg model.Message) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, part := range msg.Content {
		observe.GlobalTrace("range msg.Content")
		if _, ok := part.(model.ToolResultPart); ok {
			observe.GlobalTrace("if: ok")
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}
