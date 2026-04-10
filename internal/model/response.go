package model

import (
	"encoding/json"
	"fmt"
)

// Response is the normalized output of an LLM request.
// Every provider call resolves to this — the query engine never sees wire format.
type Response struct {
	ID         string        `json:"id"`
	Model      string        `json:"model"`
	Content    []ContentPart `json:"content"`
	StopReason StopReason    `json:"stop_reason"`
	Usage      TokenUsage    `json:"usage"`
}

type responseJSON struct {
	ID         string          `json:"id"`
	Model      string          `json:"model"`
	Content    json.RawMessage `json:"content"`
	StopReason StopReason      `json:"stop_reason"`
	Usage      TokenUsage      `json:"usage"`
}

func (r Response) MarshalJSON() ([]byte, error) {
	content, err := MarshalContentParts(r.Content)
	if err != nil {
		return nil, fmt.Errorf("marshal response content: %w", err)
	}
	return json.Marshal(responseJSON{
		ID:         r.ID,
		Model:      r.Model,
		Content:    content,
		StopReason: r.StopReason,
		Usage:      r.Usage,
	})
}

func (r *Response) UnmarshalJSON(data []byte) error {
	var raw responseJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}
	parts, err := UnmarshalContentParts(raw.Content)
	if err != nil {
		return fmt.Errorf("unmarshal response content: %w", err)
	}
	switch raw.StopReason {
	case StopEndTurn, StopToolUse, StopMaxTokens, StopPauseTurn, StopError, "":
		// valid
	default:
		return fmt.Errorf("unmarshal response: invalid stop_reason %q", raw.StopReason)
	}
	r.ID = raw.ID
	r.Model = raw.Model
	r.Content = parts
	r.StopReason = raw.StopReason
	r.Usage = raw.Usage
	return nil
}
