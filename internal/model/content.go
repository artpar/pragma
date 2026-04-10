package model

import (
	"encoding/json"
	"fmt"
)

// ContentType discriminates ContentPart variants in JSON.
type ContentType string

const (
	ContentText       ContentType = "text"
	ContentImage      ContentType = "image"
	ContentToolCall   ContentType = "tool_call"
	ContentToolResult ContentType = "tool_result"
	ContentThinking   ContentType = "thinking"
)

// ContentPart is a sealed interface representing one block of message content.
// Only the 5 variants in this file implement it.
type ContentPart interface {
	contentPartSealed()
	PartType() ContentType
}

// TextPart holds plain text content.
type TextPart struct {
	Text string `json:"text"`
}

func (TextPart) contentPartSealed() {}
func (TextPart) PartType() ContentType { return ContentText }

// ImagePart holds raw image bytes. Provider adapters encode to base64 or
// whatever wire format is needed.
type ImagePart struct {
	MimeType string `json:"mime_type"`
	Data     []byte `json:"data"`
}

func (ImagePart) contentPartSealed() {}
func (ImagePart) PartType() ContentType { return ContentImage }

// ToolCallPart represents the LLM requesting a tool invocation.
// ID is an internal UUID; provider adapters map to/from wire IDs.
// Input is raw JSON — Anthropic sends objects, OpenAI sends strings,
// the provider parses before emitting.
type ToolCallPart struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

func (ToolCallPart) contentPartSealed() {}
func (ToolCallPart) PartType() ContentType { return ContentToolCall }

// ToolResultPart holds the result of a tool invocation.
// ToolCallID correlates to ToolCallPart.ID.
type ToolResultPart struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error,omitempty"`
}

func (ToolResultPart) contentPartSealed() {}
func (ToolResultPart) PartType() ContentType { return ContentToolResult }

// ThinkingPart holds the LLM's reasoning trace (if the provider supports it).
// Signature is a provider attestation (e.g., Anthropic's cryptographic proof)
// that must be sent back in conversation history. Empty for providers that
// don't use signatures.
type ThinkingPart struct {
	Text      string `json:"text"`
	Signature string `json:"signature,omitempty"`
}

func (ThinkingPart) contentPartSealed() {}
func (ThinkingPart) PartType() ContentType { return ContentThinking }

// contentEnvelope wraps a ContentPart with a type discriminator for JSON.
type contentEnvelope struct {
	Type ContentType     `json:"type"`
	Data json.RawMessage `json:"data"`
}

// MarshalContentPart serializes a ContentPart with a "type" discriminator.
func MarshalContentPart(part ContentPart) ([]byte, error) {
	data, err := json.Marshal(part)
	if err != nil {
		return nil, fmt.Errorf("marshal content part: %w", err)
	}
	env := contentEnvelope{
		Type: part.PartType(),
		Data: data,
	}
	return json.Marshal(env)
}

// UnmarshalContentPart deserializes a ContentPart, dispatching on the "type" field.
func UnmarshalContentPart(data []byte) (ContentPart, error) {
	var env contentEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("unmarshal content envelope: %w", err)
	}
	switch env.Type {
	case ContentText:
		var p TextPart
		if err := json.Unmarshal(env.Data, &p); err != nil {
			return nil, fmt.Errorf("unmarshal text part: %w", err)
		}
		return p, nil
	case ContentImage:
		var p ImagePart
		if err := json.Unmarshal(env.Data, &p); err != nil {
			return nil, fmt.Errorf("unmarshal image part: %w", err)
		}
		return p, nil
	case ContentToolCall:
		var p ToolCallPart
		if err := json.Unmarshal(env.Data, &p); err != nil {
			return nil, fmt.Errorf("unmarshal tool call part: %w", err)
		}
		return p, nil
	case ContentToolResult:
		var p ToolResultPart
		if err := json.Unmarshal(env.Data, &p); err != nil {
			return nil, fmt.Errorf("unmarshal tool result part: %w", err)
		}
		return p, nil
	case ContentThinking:
		var p ThinkingPart
		if err := json.Unmarshal(env.Data, &p); err != nil {
			return nil, fmt.Errorf("unmarshal thinking part: %w", err)
		}
		return p, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrInvalidContentType, env.Type)
	}
}

// MarshalContentParts serializes a slice of ContentPart.
func MarshalContentParts(parts []ContentPart) ([]byte, error) {
	envs := make([]contentEnvelope, len(parts))
	for i, part := range parts {
		data, err := json.Marshal(part)
		if err != nil {
			return nil, fmt.Errorf("marshal content part [%d]: %w", i, err)
		}
		envs[i] = contentEnvelope{
			Type: part.PartType(),
			Data: data,
		}
	}
	return json.Marshal(envs)
}

// UnmarshalContentParts deserializes a JSON array of ContentPart envelopes.
func UnmarshalContentParts(data []byte) ([]ContentPart, error) {
	var envs []contentEnvelope
	if err := json.Unmarshal(data, &envs); err != nil {
		return nil, fmt.Errorf("unmarshal content parts array: %w", err)
	}
	parts := make([]ContentPart, len(envs))
	for i, env := range envs {
		raw, err := json.Marshal(env)
		if err != nil {
			return nil, fmt.Errorf("re-marshal envelope [%d]: %w", i, err)
		}
		part, err := UnmarshalContentPart(raw)
		if err != nil {
			return nil, fmt.Errorf("unmarshal content part [%d]: %w", i, err)
		}
		parts[i] = part
	}
	return parts, nil
}
