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
	var redactedThinkingParts []model.ThinkingPart
	toolCalls := make(map[string]*toolAccumulator)
	var toolOrder []string // track insertion order
	var stopReason model.StopReason
	var usage model.TokenUsage
	var responseModel string
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
		if chunk.RedactedThinkingBlock != nil {
			redactedThinkingParts = append(redactedThinkingParts, model.ThinkingPart{
				Redacted:     true,
				RedactedData: chunk.RedactedThinkingBlock.Data,
			})
		}
		if chunk.ToolCallStart != nil {
			tc := chunk.ToolCallStart
			if _, exists := toolCalls[tc.ID]; exists {
				return model.Response{}, fmt.Errorf("duplicate tool call ID %q", tc.ID)
			}
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
			responseModel = chunk.Done.Model
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
	for _, rtp := range redactedThinkingParts {
		parts = append(parts, rtp)
	}
	if textBuf.Len() > 0 {
		parts = append(parts, model.TextPart{Text: textBuf.String()})
	}
	for _, id := range toolOrder {
		acc := toolCalls[id]
		raw := json.RawMessage(acc.inputBuf.String())
		if len(raw) > 0 && !json.Valid(raw) {
			return model.Response{}, fmt.Errorf("invalid tool input JSON for %q", acc.name)
		}
		parts = append(parts, model.ToolCallPart{
			ID:    acc.id,
			Name:  acc.name,
			Input: raw,
		})
	}

	return model.Response{
		Model:      responseModel,
		Content:    parts,
		StopReason: stopReason,
		Usage:      usage,
	}, nil
}
