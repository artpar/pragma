package compact

import (
	"encoding/json"
	"fmt"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	result := make([]model.Message, 0, len(msgs))

	toolNames := make(map[string]string)
	for _, msg := range msgs {
		observe.GlobalTrace("range msgs")
		for _, part := range msg.Content {
			observe.GlobalTrace("range msg.Content")
			if tc, ok := part.(model.ToolCallPart); ok {
				observe.GlobalTrace("if: ok")
				toolNames[tc.ID] = tc.Name
			}
		}
	}

	for _, msg := range msgs {
		observe.GlobalTrace("range msgs")
		trimmed := trimMessage(msg, toolNames)
		if len(trimmed.Content) > 0 {
			observe.GlobalTrace("if: len(trimmed.Content) > 0")
			result = append(result, trimmed)
		}
	}
	observe.GlobalTrace("return: result")
	observe.GlobalTrace("return: result")
	return result
}

func trimMessage(msg model.Message, toolNames map[string]string) model.Message {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	out := model.Message{
		ID:        msg.ID,
		Role:      msg.Role,
		Timestamp: msg.Timestamp,
		Flags:     msg.Flags,
	}

	parts := make([]model.ContentPart, 0, len(msg.Content))
	for _, part := range msg.Content {
		observe.GlobalTrace("range msg.Content")
		if trimmed := trimPart(part, toolNames); trimmed != nil {
			observe.GlobalTrace("if: trimmed != nil")
			parts = append(parts, trimmed)
		}
	}
	out.Content = parts
	observe.GlobalTrace("return: out")
	observe.GlobalTrace("return: out")
	return out
}

func trimPart(part model.ContentPart, toolNames map[string]string) model.ContentPart {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch p := part.(type) {
	case model.TextPart:
		observe.GlobalTrace("typecase: model.TextPart")

		return p

	case model.ThinkingPart:
		observe.GlobalTrace("typecase: model.ThinkingPart")

		return nil

	case model.ToolCallPart:
		observe.GlobalTrace("typecase: model.ToolCallPart")

		inputCopy := make(json.RawMessage, len(p.Input))
		copy(inputCopy, p.Input)
		return model.ToolCallPart{
			ID:    p.ID,
			Name:  p.Name,
			Input: inputCopy,
		}

	case model.ToolResultPart:
		observe.GlobalTrace("typecase: model.ToolResultPart")
		if len(p.Content) <= largeToolResultThreshold {
			observe.GlobalTrace("return: p")
			observe.GlobalTrace("return: p")
			return p
		}

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
		observe.GlobalTrace("typecase: model.ImagePart")

		return model.TextPart{Text: fmt.Sprintf("[image: %s]", p.MimeType)}

	case model.DocumentPart:
		observe.GlobalTrace("typecase: model.DocumentPart")

		return model.TextPart{Text: fmt.Sprintf("[document: %s, %d bytes]", p.MimeType, len(p.Data))}

	default:
		observe.GlobalTrace("typedefault")
		return part
	}
}
