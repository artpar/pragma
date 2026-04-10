package anthropic

import (
	"github.com/artpar/gogent/internal/model"
)

// normalizeMessages prepares a message sequence for the Anthropic API:
//  1. Filters out messages with no content parts
//  2. Merges consecutive same-role messages
//  3. Ensures every tool_use has a matching tool_result (and vice versa)
func normalizeMessages(msgs []model.Message) []model.Message {
	msgs = filterEmpty(msgs)
	msgs = mergeConsecutiveSameRole(msgs)
	msgs = ensureToolResultPairing(msgs)
	return msgs
}

// filterEmpty removes messages that have zero content parts.
func filterEmpty(msgs []model.Message) []model.Message {
	out := make([]model.Message, 0, len(msgs))
	for _, m := range msgs {
		if len(m.Content) > 0 {
			out = append(out, m)
		}
	}
	return out
}

// mergeConsecutiveSameRole merges adjacent messages with the same role
// by concatenating their content part slices. Anthropic requires strictly
// alternating user/assistant messages.
func mergeConsecutiveSameRole(msgs []model.Message) []model.Message {
	if len(msgs) == 0 {
		return msgs
	}
	out := make([]model.Message, 0, len(msgs))
	out = append(out, msgs[0])
	for i := 1; i < len(msgs); i++ {
		last := &out[len(out)-1]
		if msgs[i].Role == last.Role {
			last.Content = append(last.Content, msgs[i].Content...)
		} else {
			out = append(out, msgs[i])
		}
	}
	return out
}

// ensureToolResultPairing validates that every ToolCallPart in an assistant
// message has a corresponding ToolResultPart in the next user message.
// Orphaned tool calls get a synthetic error result. Orphaned tool results
// (no matching tool call) are removed.
func ensureToolResultPairing(msgs []model.Message) []model.Message {
	// Deep copy messages so appending to Content slices doesn't corrupt the caller's data
	// through shared backing arrays.
	out := make([]model.Message, len(msgs))
	for i, m := range msgs {
		content := make([]model.ContentPart, len(m.Content))
		copy(content, m.Content)
		out[i] = m
		out[i].Content = content
	}

	for i := 0; i < len(out); i++ {
		if out[i].Role != model.RoleAssistant {
			continue
		}

		// Collect tool call IDs from this assistant message
		callIDs := make(map[string]bool)
		for _, part := range out[i].Content {
			if tc, ok := part.(model.ToolCallPart); ok {
				callIDs[tc.ID] = true
			}
		}
		if len(callIDs) == 0 {
			continue
		}

		// Check the next message (should be user with tool results)
		if i+1 >= len(out) {
			// No following message — inject synthetic user message with error results
			var results []model.ContentPart
			for id := range callIDs {
				results = append(results, model.ToolResultPart{
					ToolCallID: id,
					Content:    "Tool execution was interrupted",
					IsError:    true,
				})
			}
			synth := model.Message{
				ID:      "synthetic-tool-result",
				Role:    model.RoleUser,
				Content: results,
			}
			out = append(out, synth)
			continue
		}

		nextMsg := &out[i+1]
		if nextMsg.Role != model.RoleUser {
			// Not a user message — inject one before it
			var results []model.ContentPart
			for id := range callIDs {
				results = append(results, model.ToolResultPart{
					ToolCallID: id,
					Content:    "Tool execution was interrupted",
					IsError:    true,
				})
			}
			synth := model.Message{
				ID:      "synthetic-tool-result",
				Role:    model.RoleUser,
				Content: results,
			}
			// Insert synth at position i+1
			out = append(out[:i+1], append([]model.Message{synth}, out[i+1:]...)...)
			continue
		}

		// Check which tool calls have results
		resultIDs := make(map[string]bool)
		for _, part := range nextMsg.Content {
			if tr, ok := part.(model.ToolResultPart); ok {
				resultIDs[tr.ToolCallID] = true
			}
		}

		// Add missing results
		for id := range callIDs {
			if !resultIDs[id] {
				nextMsg.Content = append(nextMsg.Content, model.ToolResultPart{
					ToolCallID: id,
					Content:    "Tool execution was interrupted",
					IsError:    true,
				})
			}
		}

		// Remove orphaned tool results (result with no matching call)
		cleaned := make([]model.ContentPart, 0, len(nextMsg.Content))
		for _, part := range nextMsg.Content {
			if tr, ok := part.(model.ToolResultPart); ok {
				if !callIDs[tr.ToolCallID] {
					continue // orphaned result — skip
				}
			}
			cleaned = append(cleaned, part)
		}
		nextMsg.Content = cleaned
	}

	return out
}
