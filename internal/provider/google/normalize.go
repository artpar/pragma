package google

import "github.com/artpar/gogent/internal/model"

// normalizeMessages prepares internal messages for the Google Gemini API.
func normalizeMessages(msgs []model.Message) []model.Message {
	msgs = filterEmpty(msgs)
	msgs = stripEmptyTextParts(msgs)
	msgs = ensureToolResultPairing(msgs)
	msgs = mergeConsecutiveRoles(msgs)
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
func ensureToolResultPairing(msgs []model.Message) []model.Message {
	out := make([]model.Message, 0, len(msgs))
	pendingCalls := make(map[string]bool)

	for i, m := range msgs {
		if m.Role == model.RoleAssistant {
			pendingCalls = make(map[string]bool)
			for _, p := range m.Content {
				if tc, ok := p.(model.ToolCallPart); ok {
					pendingCalls[tc.ID] = false
				}
			}
			out = append(out, m)

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
			var filtered []model.ContentPart
			for _, p := range m.Content {
				if tr, ok := p.(model.ToolResultPart); ok {
					if _, exists := pendingCalls[tr.ToolCallID]; exists {
						pendingCalls[tr.ToolCallID] = true
						filtered = append(filtered, p)
					}
				} else {
					filtered = append(filtered, p)
				}
			}

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

// mergeConsecutiveRoles merges adjacent messages with the same role.
// Google requires strictly alternating user/model messages.
func mergeConsecutiveRoles(msgs []model.Message) []model.Message {
	if len(msgs) == 0 {
		return msgs
	}
	out := make([]model.Message, 0, len(msgs))
	out = append(out, msgs[0])

	for i := 1; i < len(msgs); i++ {
		prev := &out[len(out)-1]
		cur := msgs[i]
		if prev.Role == cur.Role {
			prev.Content = append(prev.Content, cur.Content...)
		} else {
			out = append(out, cur)
		}
	}
	return out
}
