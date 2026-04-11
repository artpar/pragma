package openai

import "github.com/artpar/gogent/internal/model"

// normalizeMessages prepares internal messages for the OpenAI API.
func normalizeMessages(msgs []model.Message) []model.Message {
	msgs = filterEmpty(msgs)
	msgs = stripEmptyTextParts(msgs)
	msgs = ensureToolResultPairing(msgs)
	msgs = mergeConsecutiveUser(msgs)
	return msgs
}

// filterEmpty removes messages with zero content parts.
func filterEmpty(msgs []model.Message) []model.Message {
	out := make([]model.Message, 0, len(msgs))
	for _, m := range msgs {
		if len(m.Content) > 0 {
			out = append(out, m)
		}
	}
	return out
}

// stripEmptyTextParts removes TextPart{Text: ""} from all messages (ADR-018 defense).
func stripEmptyTextParts(msgs []model.Message) []model.Message {
	out := make([]model.Message, 0, len(msgs))
	for _, m := range msgs {
		var filtered []model.ContentPart
		for _, p := range m.Content {
			if tp, ok := p.(model.TextPart); ok && tp.Text == "" {
				continue
			}
			filtered = append(filtered, p)
		}
		if len(filtered) > 0 {
			m.Content = filtered
			out = append(out, m)
		}
	}
	return out
}

// ensureToolResultPairing ensures every ToolCallPart has a matching ToolResultPart
// and vice versa. Injects synthetic error results for orphan calls, removes orphan results.
// If conversation ends with an assistant message containing tool calls, injects a
// synthetic user message with error results.
func ensureToolResultPairing(msgs []model.Message) []model.Message {
	out := make([]model.Message, 0, len(msgs))
	pendingCalls := make(map[string]bool) // toolCallID -> has result

	for i, m := range msgs {
		if m.Role == model.RoleAssistant {
			// Reset pending for this assistant turn
			pendingCalls = make(map[string]bool)
			for _, p := range m.Content {
				if tc, ok := p.(model.ToolCallPart); ok {
					pendingCalls[tc.ID] = false
				}
			}
			out = append(out, m)

			// If no following message or next message isn't user, inject synthetic user message
			if len(pendingCalls) > 0 {
				hasFollowingUser := i+1 < len(msgs) && msgs[i+1].Role == model.RoleUser
				if !hasFollowingUser {
					var synthetic []model.ContentPart
					for callID := range pendingCalls {
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
			// Mark results that have matching calls, remove orphans
			var filtered []model.ContentPart
			for _, p := range m.Content {
				if tr, ok := p.(model.ToolResultPart); ok {
					if _, exists := pendingCalls[tr.ToolCallID]; exists {
						pendingCalls[tr.ToolCallID] = true
						filtered = append(filtered, p)
					}
					// Orphaned results (no matching call) are dropped
				} else {
					filtered = append(filtered, p)
				}
			}

			// Inject synthetic error results for calls that have no result
			var synthetic []model.ContentPart
			for callID, hasResult := range pendingCalls {
				if !hasResult {
					synthetic = append(synthetic, model.ToolResultPart{
						ToolCallID: callID,
						Content:    "Tool execution was interrupted.",
						IsError:    true,
					})
				}
			}
			if len(synthetic) > 0 {
				filtered = append(synthetic, filtered...)
			}

			// Reset pending calls for the next turn
			pendingCalls = make(map[string]bool)

			if len(filtered) > 0 {
				m.Content = filtered
				out = append(out, m)
			}
			continue
		}

		out = append(out, m)
	}

	return out
}

// mergeConsecutiveUser merges adjacent user messages into one.
func mergeConsecutiveUser(msgs []model.Message) []model.Message {
	if len(msgs) == 0 {
		return msgs
	}
	out := make([]model.Message, 0, len(msgs))
	out = append(out, msgs[0])

	for i := 1; i < len(msgs); i++ {
		prev := &out[len(out)-1]
		cur := msgs[i]
		if prev.Role == model.RoleUser && cur.Role == model.RoleUser {
			prev.Content = append(prev.Content, cur.Content...)
		} else {
			out = append(out, cur)
		}
	}
	return out
}
