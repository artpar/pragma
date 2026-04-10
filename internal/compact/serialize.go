package compact

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artpar/gogent/internal/model"
)

// maxInputPreview is the max bytes of tool call input to include in serialization.
const maxInputPreview = 200

// SerializeForCompaction converts messages into a text representation suitable
// for the compaction prompt. Each message is labeled by role and content type.
// Messages should be pre-processed by Microcompact() before serialization.
func SerializeForCompaction(msgs []model.Message) string {
	var b strings.Builder

	for _, msg := range msgs {
		if msg.Flags.IsInternal {
			continue
		}

		label := roleLabel(msg)
		b.WriteString(label)
		b.WriteString("\n")

		for _, part := range msg.Content {
			serializePart(&b, part)
		}

		b.WriteString("\n")
	}

	return strings.TrimSpace(b.String())
}

func roleLabel(msg model.Message) string {
	if msg.Flags.IsCompactSummary {
		return "[Previous Compaction Summary]"
	}
	switch msg.Role {
	case model.RoleUser:
		for _, part := range msg.Content {
			if _, ok := part.(model.ToolResultPart); ok {
				return "[User - Tool Results]"
			}
		}
		return "[User]"
	case model.RoleAssistant:
		return "[Assistant]"
	default:
		return fmt.Sprintf("[%s]", msg.Role)
	}
}

func serializePart(b *strings.Builder, part model.ContentPart) {
	switch p := part.(type) {
	case model.TextPart:
		if p.Text != "" {
			b.WriteString(p.Text)
			b.WriteString("\n")
		}

	case model.ToolCallPart:
		inputPreview := truncateInput(p.Input)
		b.WriteString(fmt.Sprintf("[Tool Call: %s(%s)]\n", p.Name, inputPreview))

	case model.ToolResultPart:
		if p.IsError {
			b.WriteString(fmt.Sprintf("[Tool Error]: %s\n", p.Content))
		} else {
			b.WriteString(p.Content)
			b.WriteString("\n")
		}

	// ThinkingPart, ImagePart, DocumentPart should already be handled by Microcompact,
	// but handle gracefully if called without microcompaction.
	case model.ThinkingPart:
		// Skip — thinking was already used by the model
	case model.ImagePart:
		b.WriteString(fmt.Sprintf("[image: %s]\n", p.MimeType))
	case model.DocumentPart:
		b.WriteString(fmt.Sprintf("[document: %s, %d bytes]\n", p.MimeType, len(p.Data)))
	}
}

func truncateInput(input json.RawMessage) string {
	s := string(input)
	if len(s) <= maxInputPreview {
		return s
	}
	return s[:maxInputPreview] + "..."
}
