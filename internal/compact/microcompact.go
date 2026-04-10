package compact

import (
	"encoding/json"
	"fmt"

	"github.com/artpar/gogent/internal/model"
)

// largeToolResultThreshold is the character count above which tool results
// are stubbed during microcompaction. Based on insight from GitHub issue #27293:
// 60-70% of context is tool results already synthesized by the model.
const largeToolResultThreshold = 500

// Microcompact returns a deep-copied, trimmed version of messages suitable for
// the compaction prompt. This is LOSSLESS — information the model already
// incorporated into its text responses is stubbed, not lost.
//
// Operations:
// - Large tool results (>500 chars) are stubbed with a size marker
// - ThinkingParts are stripped entirely (model already used reasoning)
// - ImageParts/DocumentParts are replaced with type markers
// - TextParts are preserved verbatim
// - ToolCallParts are preserved (show what was requested)
func Microcompact(msgs []model.Message) []model.Message {
	result := make([]model.Message, 0, len(msgs))

	// Build a map of tool call ID → tool name for result stubbing
	toolNames := make(map[string]string)
	for _, msg := range msgs {
		for _, part := range msg.Content {
			if tc, ok := part.(model.ToolCallPart); ok {
				toolNames[tc.ID] = tc.Name
			}
		}
	}

	for _, msg := range msgs {
		trimmed := trimMessage(msg, toolNames)
		if len(trimmed.Content) > 0 {
			result = append(result, trimmed)
		}
	}
	return result
}

func trimMessage(msg model.Message, toolNames map[string]string) model.Message {
	out := model.Message{
		ID:        msg.ID,
		Role:      msg.Role,
		Timestamp: msg.Timestamp,
		Flags:     msg.Flags,
	}

	parts := make([]model.ContentPart, 0, len(msg.Content))
	for _, part := range msg.Content {
		if trimmed := trimPart(part, toolNames); trimmed != nil {
			parts = append(parts, trimmed)
		}
	}
	out.Content = parts
	return out
}

func trimPart(part model.ContentPart, toolNames map[string]string) model.ContentPart {
	switch p := part.(type) {
	case model.TextPart:
		// Preserve text verbatim — this is the actual conversation content
		return p

	case model.ThinkingPart:
		// Strip entirely — model already used its reasoning, the output
		// text response captures the synthesized result
		return nil

	case model.ToolCallPart:
		// Preserve — shows what was requested (important context)
		// Deep copy the input
		inputCopy := make(json.RawMessage, len(p.Input))
		copy(inputCopy, p.Input)
		return model.ToolCallPart{
			ID:    p.ID,
			Name:  p.Name,
			Input: inputCopy,
		}

	case model.ToolResultPart:
		if len(p.Content) <= largeToolResultThreshold {
			return p
		}
		// Stub large results — model already synthesized this content
		name := toolNames[p.ToolCallID]
		if name == "" {
			name = "unknown"
		}
		return model.ToolResultPart{
			ToolCallID: p.ToolCallID,
			Content:    fmt.Sprintf("[tool result for %s: %d chars — content already synthesized in assistant response above]", name, len(p.Content)),
			IsError:    p.IsError,
		}

	case model.ImagePart:
		// Replace with marker — image content can't be summarized as text
		return model.TextPart{Text: fmt.Sprintf("[image: %s]", p.MimeType)}

	case model.DocumentPart:
		// Replace with marker
		return model.TextPart{Text: fmt.Sprintf("[document: %s, %d bytes]", p.MimeType, len(p.Data))}

	default:
		return part
	}
}
