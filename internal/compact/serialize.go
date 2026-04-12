package compact

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
)

// maxInputPreview is the max bytes of tool call input to include in serialization.
const maxInputPreview = 200

// SerializeForCompaction converts messages into a text representation suitable
// for the compaction prompt. Each message is labeled by role and content type.
// Messages should be pre-processed by Microcompact() before serialization.
func SerializeForCompaction(msgs []model.Message) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var b strings.Builder

	for _, msg := range msgs {
		observe.GlobalTrace("range msgs")
		if msg.Flags.IsInternal {
			observe.GlobalTrace("if: msg.Flags.IsInternal")
			continue
		}

		label := roleLabel(msg)
		b.WriteString(label)
		b.WriteString("\n")

		for _, part := range msg.Content {
			observe.GlobalTrace("range msg.Content")
			serializePart(&b, part)
		}

		b.WriteString("\n")
	}
	observe.GlobalTrace("return: strings.TrimSpace(b.String())")
	observe.GlobalTrace("return: strings.TrimSpace(b.String())")
	observe.GlobalTrace("return: strings.TrimSpace(b.String())")

	return strings.TrimSpace(b.String())
}

func roleLabel(msg model.Message) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if msg.Flags.IsCompactSummary {
		observe.GlobalTrace("if: msg.Flags.IsCompactSummary")
		observe.GlobalTrace("return: \"[Previous Compaction Summary]\"")
		observe.GlobalTrace("return: \"[Previous Compaction Summary]\"")
		observe.GlobalTrace("return: \"[Previous Compaction Summary]\"")
		return "[Previous Compaction Summary]"
	}
	switch msg.Role {
	case model.RoleUser:
		observe.GlobalTrace("case: model.RoleUser")
		for _, part := range msg.Content {
			if _, ok := part.(model.ToolResultPart); ok {
				observe.GlobalTrace("if: ok")
				observe.GlobalTrace("return: \"[User - Tool Results]\"")
				observe.GlobalTrace("return: \"[User - Tool Results]\"")
				observe.GlobalTrace("return: \"[User - Tool Results]\"")
				return "[User - Tool Results]"
			}
		}
		return "[User]"
	case model.RoleAssistant:
		observe.GlobalTrace("case: model.RoleAssistant")
		return "[Assistant]"
	default:
		observe.GlobalTrace("default")
		return fmt.Sprintf("[%s]", msg.Role)
	}
}

func serializePart(b *strings.Builder, part model.ContentPart) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch p := part.(type) {
	case model.TextPart:
		observe.GlobalTrace("typecase: model.TextPart")
		if p.Text != "" {
			b.WriteString(p.Text)
			b.WriteString("\n")
		}

	case model.ToolCallPart:
		observe.GlobalTrace("typecase: model.ToolCallPart")
		inputPreview := truncateInput(p.Input)
		b.WriteString(fmt.Sprintf("[Tool Call: %s(%s)]\n", p.Name, inputPreview))

	case model.ToolResultPart:
		observe.GlobalTrace("typecase: model.ToolResultPart")
		if p.IsError {
			b.WriteString(fmt.Sprintf("[Tool Error]: %s\n", p.Content))
		} else {
			b.WriteString(p.Content)
			b.WriteString("\n")
		}

	case model.ThinkingPart:
		observe.GlobalTrace("typecase: model.ThinkingPart")

	case model.ImagePart:
		observe.GlobalTrace("typecase: model.ImagePart")
		b.WriteString(fmt.Sprintf("[image: %s]\n", p.MimeType))
	case model.DocumentPart:
		observe.GlobalTrace("typecase: model.DocumentPart")
		b.WriteString(fmt.Sprintf("[document: %s, %d bytes]\n", p.MimeType, len(p.Data)))
	}
}

func truncateInput(input json.RawMessage) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	s := string(input)
	if len(s) <= maxInputPreview {
		observe.GlobalTrace("if: len(s) <= maxInputPreview")
		observe.GlobalTrace("return: s")
		observe.GlobalTrace("return: s")
		observe.GlobalTrace("return: s")
		return s
	}
	observe.GlobalTrace("return: s[:maxInputPreview] + \"...\"")
	observe.GlobalTrace("return: s[:maxInputPreview] + \"...\"")
	observe.GlobalTrace("return: s[:maxInputPreview] + \"...\"")
	return s[:maxInputPreview] + "..."
}
