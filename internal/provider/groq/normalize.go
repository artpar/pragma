package groq

import (
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
)

// normalizeMessages prepares internal messages for the Groq API.
func normalizeMessages(msgs []model.Message) []model.Message {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	msgs = filterEmpty(msgs)
	msgs = stripEmptyTextParts(msgs)
	msgs = ensureToolResultPairing(msgs)
	msgs = mergeConsecutiveUser(msgs)
	observe.GlobalTrace("return: msgs")
	return msgs
}

// filterEmpty removes messages with zero content parts.
func filterEmpty(msgs []model.Message) []model.Message {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	out := make([]model.Message, 0, len(msgs))
	for _, m := range msgs {
		observe.GlobalTrace("range msgs")
		if len(m.Content) > 0 {
			observe.GlobalTrace("if: len(m.Content) > 0")
			out = append(out, m)
		}
	}
	observe.GlobalTrace("return: out")
	return out
}

// stripEmptyTextParts removes TextPart{Text: ""} from all messages (ADR-018 defense).
func stripEmptyTextParts(msgs []model.Message) []model.Message {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	out := make([]model.Message, 0, len(msgs))
	for _, m := range msgs {
		observe.GlobalTrace("range msgs")
		var filtered []model.ContentPart
		for _, p := range m.Content {
			observe.GlobalTrace("range m.Content")
			if tp, ok := p.(model.TextPart); ok && tp.Text == "" {
				observe.GlobalTrace("if: ok && tp.Text == \"\"")
				continue
			}
			filtered = append(filtered, p)
		}
		if len(filtered) > 0 {
			observe.GlobalTrace("if: len(filtered) > 0")
			m.Content = filtered
			out = append(out, m)
		}
	}
	observe.GlobalTrace("return: out")
	return out
}

// ensureToolResultPairing ensures every ToolCallPart has a matching ToolResultPart
// and vice versa. Injects synthetic error results for orphan calls, removes orphan results.
// If conversation ends with an assistant message containing tool calls, injects a
// synthetic user message with error results.
func ensureToolResultPairing(msgs []model.Message) []model.Message {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	out := make([]model.Message, 0, len(msgs))
	pendingCalls := make(map[string]bool)

	for i, m := range msgs {
		observe.GlobalTrace("range msgs")
		if m.Role == model.RoleAssistant {
			observe.GlobalTrace("if: m.Role == model.RoleAssistant")

			pendingCalls = make(map[string]bool)
			for _, p := range m.Content {
				observe.GlobalTrace("range m.Content")
				if tc, ok := p.(model.ToolCallPart); ok {
					observe.GlobalTrace("if: ok")
					pendingCalls[tc.ID] = false
				}
			}
			out = append(out, m)

			if len(pendingCalls) > 0 {
				observe.GlobalTrace("if: len(pendingCalls) > 0")
				hasFollowingUser := i+1 < len(msgs) && msgs[i+1].Role == model.RoleUser
				if !hasFollowingUser {
					observe.GlobalTrace("if: !hasFollowingUser")
					var synthetic []model.ContentPart
					for callID := range pendingCalls {
						observe.GlobalTrace("range pendingCalls")
						synthetic = append(synthetic, model.ToolResultPart{
							ToolCallID: callID,
							Content:    "Tool execution was interrupted.",
							IsError:    true,
						})
					}
					out = append(out, model.Message{
						Role:    model.RoleUser,
						Content: synthetic,
					})
					pendingCalls = make(map[string]bool)
				}
			}
			continue
		}

		if m.Role == model.RoleUser {
			observe.GlobalTrace("if: m.Role == model.RoleUser")
			// Mark results that have matching calls, remove orphans
			var filtered []model.ContentPart
			for _, p := range m.Content {
				observe.GlobalTrace("range m.Content")
				if tr, ok := p.(model.ToolResultPart); ok {
					observe.GlobalTrace("if: ok")
					if _, exists := pendingCalls[tr.ToolCallID]; exists {
						observe.GlobalTrace("if: exists")
						pendingCalls[tr.ToolCallID] = true
						filtered = append(filtered, p)
					}

				} else {
					observe.GlobalTrace("else: ok")
					filtered = append(filtered, p)
				}
			}

			// Inject synthetic error results for calls that have no result
			var synthetic []model.ContentPart
			for callID, hasResult := range pendingCalls {
				observe.GlobalTrace("range pendingCalls")
				if !hasResult {
					observe.GlobalTrace("if: !hasResult")
					synthetic = append(synthetic, model.ToolResultPart{
						ToolCallID: callID,
						Content:    "Tool execution was interrupted.",
						IsError:    true,
					})
				}
			}
			if len(synthetic) > 0 {
				observe.GlobalTrace("if: len(synthetic) > 0")
				filtered = append(synthetic, filtered...)
			}

			pendingCalls = make(map[string]bool)

			if len(filtered) > 0 {
				observe.GlobalTrace("if: len(filtered) > 0")
				m.Content = filtered
				out = append(out, m)
			}
			continue
		}

		out = append(out, m)
	}
	observe.GlobalTrace("return: out")

	return out
}

// mergeConsecutiveUser merges adjacent user messages into one.
func mergeConsecutiveUser(msgs []model.Message) []model.Message {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(msgs) == 0 {
		observe.GlobalTrace("if: len(msgs) == 0")
		observe.GlobalTrace("return: msgs")
		return msgs
	}
	out := make([]model.Message, 0, len(msgs))
	out = append(out, msgs[0])

	for i := 1; i < len(msgs); i++ {
		observe.GlobalTrace("for: i < len(msgs)")
		prev := &out[len(out)-1]
		cur := msgs[i]
		if prev.Role == model.RoleUser && cur.Role == model.RoleUser {
			observe.GlobalTrace("if: prev.Role == model.RoleUser && cur.Role == model.RoleUser")
			prev.Content = append(prev.Content, cur.Content...)
		} else {
			observe.GlobalTrace("else: prev.Role == model.RoleUser && cur.Role == model.RoleUser")
			out = append(out, cur)
		}
	}
	observe.GlobalTrace("return: out")
	return out
}
