package anthropic

import (
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
)

// normalizeMessages prepares a message sequence for the Anthropic API:
//  1. Filters out messages with no content parts
//  2. Merges consecutive same-role messages
//  3. Ensures every tool_use has a matching tool_result (and vice versa)
func normalizeMessages(msgs []model.Message) []model.Message {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	msgs = filterEmpty(msgs)
	msgs = mergeConsecutiveSameRole(msgs)
	msgs = ensureToolResultPairing(msgs)
	observe.GlobalTrace("return: msgs")
	return msgs
}

// filterEmpty removes messages that have zero content parts.
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

// mergeConsecutiveSameRole merges adjacent messages with the same role
// by concatenating their content part slices. Anthropic requires strictly
// alternating user/assistant messages.
func mergeConsecutiveSameRole(msgs []model.Message) []model.Message {
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
		last := &out[len(out)-1]
		if msgs[i].Role == last.Role {
			observe.GlobalTrace("if: msgs[i].Role == last.Role")
			last.Content = append(last.Content, msgs[i].Content...)
		} else {
			observe.GlobalTrace("else: msgs[i].Role == last.Role")
			out = append(out, msgs[i])
		}
	}
	observe.GlobalTrace("return: out")
	return out
}

// ensureToolResultPairing validates that every ToolCallPart in an assistant
// message has a corresponding ToolResultPart in the next user message.
// Orphaned tool calls get a synthetic error result. Orphaned tool results
// (no matching tool call) are removed.
func ensureToolResultPairing(msgs []model.Message) []model.Message {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	out := make([]model.Message, len(msgs))
	for i, m := range msgs {
		observe.GlobalTrace("range msgs")
		content := make([]model.ContentPart, len(m.Content))
		copy(content, m.Content)
		out[i] = m
		out[i].Content = content
	}

	for i := 0; i < len(out); i++ {
		observe.GlobalTrace("for: i < len(out)")
		if out[i].Role != model.RoleAssistant {
			observe.GlobalTrace("if: out[i].Role != model.RoleAssistant")
			continue
		}

		callIDs := make(map[string]bool)
		for _, part := range out[i].Content {
			observe.GlobalTrace("range out[i].Content")
			if tc, ok := part.(model.ToolCallPart); ok {
				observe.GlobalTrace("if: ok")
				callIDs[tc.ID] = true
			}
		}
		if len(callIDs) == 0 {
			observe.GlobalTrace("if: len(callIDs) == 0")
			continue
		}

		if i+1 >= len(out) {
			observe.GlobalTrace("if: i+1 >= len(out)")
			// No following message — inject synthetic user message with error results
			var results []model.ContentPart
			for id := range callIDs {
				observe.GlobalTrace("range callIDs")
				results = append(results, model.ToolResultPart{
					ToolCallID: id,
					Content:    "Tool execution was interrupted",
					IsError:    true,
				})
			}
			synth := model.Message{
				ID:      model.NewUUID(),
				Role:    model.RoleUser,
				Content: results,
			}
			out = append(out, synth)
			continue
		}

		nextMsg := &out[i+1]
		if nextMsg.Role != model.RoleUser {
			observe.GlobalTrace("if: nextMsg.Role != model.RoleUser")
			// Not a user message — inject one before it
			var results []model.ContentPart
			for id := range callIDs {
				observe.GlobalTrace("range callIDs")
				results = append(results, model.ToolResultPart{
					ToolCallID: id,
					Content:    "Tool execution was interrupted",
					IsError:    true,
				})
			}
			synth := model.Message{
				ID:      model.NewUUID(),
				Role:    model.RoleUser,
				Content: results,
			}

			out = append(out[:i+1], append([]model.Message{synth}, out[i+1:]...)...)
			continue
		}

		resultIDs := make(map[string]bool)
		for _, part := range nextMsg.Content {
			observe.GlobalTrace("range nextMsg.Content")
			if tr, ok := part.(model.ToolResultPart); ok {
				observe.GlobalTrace("if: ok")
				resultIDs[tr.ToolCallID] = true
			}
		}

		for id := range callIDs {
			observe.GlobalTrace("range callIDs")
			if !resultIDs[id] {
				observe.GlobalTrace("if: !resultIDs[id]")
				nextMsg.Content = append(nextMsg.Content, model.ToolResultPart{
					ToolCallID: id,
					Content:    "Tool execution was interrupted",
					IsError:    true,
				})
			}
		}

		cleaned := make([]model.ContentPart, 0, len(nextMsg.Content))
		for _, part := range nextMsg.Content {
			observe.GlobalTrace("range nextMsg.Content")
			if tr, ok := part.(model.ToolResultPart); ok {
				observe.GlobalTrace("if: ok")
				if !callIDs[tr.ToolCallID] {
					observe.GlobalTrace("if: !callIDs[tr.ToolCallID]")
					continue
				}
			}
			cleaned = append(cleaned, part)
		}
		nextMsg.Content = cleaned
	}
	observe.GlobalTrace("return: out")

	return out
}
