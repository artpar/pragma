package model

import "strings"

// ExtractUserTextPrompts returns text-only user prompts from a message list.
// It is used by interactive presentation layers to seed input history.
func ExtractUserTextPrompts(messages []Message) []string {
	var prompts []string
	for _, msg := range messages {
		if msg.Role != RoleUser {
			continue
		}
		var b strings.Builder
		for _, part := range msg.Content {
			if tp, ok := part.(TextPart); ok {
				b.WriteString(tp.Text)
			}
		}
		if b.Len() > 0 {
			prompts = append(prompts, b.String())
		}
	}
	return prompts
}
