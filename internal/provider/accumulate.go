package provider

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artpar/gogent/internal/model"
)

// toolAccumulator collects streaming fragments for a single tool call.
type toolAccumulator struct {
	id       string
	name     string
	inputBuf strings.Builder
}

// AccumulateStream consumes all chunks from a streaming response channel
// and assembles them into a complete model.Response.
//
// Content part ordering in the returned Response:
// ThinkingPart (if any), then TextPart (if any), then ToolCallParts (in order received).
func AccumulateStream(chunks <-chan StreamChunk) (model.Response, error) {
	var textBuf strings.Builder
	var thinkBuf strings.Builder
	var thinkSigBuf strings.Builder
	toolCalls := make(map[string]*toolAccumulator)
	var toolOrder []string // track insertion order
	var stopReason model.StopReason
	var usage model.TokenUsage
	gotDone := false

	for chunk := range chunks {
		if chunk.Error != nil {
			return model.Response{}, chunk.Error
		}
		if chunk.TextDelta != "" {
			textBuf.WriteString(chunk.TextDelta)
		}
		if chunk.ThinkingDelta != "" {
			thinkBuf.WriteString(chunk.ThinkingDelta)
		}
		if chunk.ThinkingSignatureDelta != "" {
			thinkSigBuf.WriteString(chunk.ThinkingSignatureDelta)
		}
		if chunk.ToolCallStart != nil {
			tc := chunk.ToolCallStart
			acc := &toolAccumulator{
				id:   tc.ID,
				name: tc.Name,
			}
			toolCalls[tc.ID] = acc
			toolOrder = append(toolOrder, tc.ID)
		}
		if chunk.ToolCallInputDelta != nil {
			acc, ok := toolCalls[chunk.ToolCallInputDelta.ToolCallID]
			if !ok {
				return model.Response{}, fmt.Errorf("input delta for unknown tool call %q", chunk.ToolCallInputDelta.ToolCallID)
			}
			acc.inputBuf.WriteString(chunk.ToolCallInputDelta.JSONDelta)
		}
		if chunk.Done != nil {
			stopReason = chunk.Done.StopReason
			usage = chunk.Done.Usage
			gotDone = true
		}
	}

	if !gotDone {
		return model.Response{}, fmt.Errorf("stream ended without Done chunk: %w", model.ErrStreamClosed)
	}

	var parts []model.ContentPart

	if thinkBuf.Len() > 0 {
		parts = append(parts, model.ThinkingPart{
			Text:      thinkBuf.String(),
			Signature: thinkSigBuf.String(),
		})
	}
	if textBuf.Len() > 0 {
		parts = append(parts, model.TextPart{Text: textBuf.String()})
	}
	for _, id := range toolOrder {
		acc := toolCalls[id]
		parts = append(parts, model.ToolCallPart{
			ID:    acc.id,
			Name:  acc.name,
			Input: json.RawMessage(acc.inputBuf.String()),
		})
	}

	return model.Response{
		Content:    parts,
		StopReason: stopReason,
		Usage:      usage,
	}, nil
}
