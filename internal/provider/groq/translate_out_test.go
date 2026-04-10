package groq

import (
	"encoding/json"
	"testing"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/provider"
)

func TestBuildWireRequest_Basic(t *testing.T) {
	params := provider.RequestParams{
		Model:     "llama-3.3-70b-versatile",
		MaxTokens: 1024,
		Messages: []model.Message{
			{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "hello"}}},
		},
		System: model.SystemPrompt{
			Blocks: []model.SystemBlock{{Text: "You are helpful."}},
		},
	}
	mapper := NewIDMapper()
	req := buildWireRequest(params, mapper, false)

	if req.Model != "llama-3.3-70b-versatile" {
		t.Errorf("model=%q", req.Model)
	}
	if req.MaxCompletionTokens != 1024 {
		t.Errorf("max_completion_tokens=%d, want 1024", req.MaxCompletionTokens)
	}
	if req.Stream {
		t.Error("stream should be false")
	}
	// System message + user message
	if len(req.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(req.Messages))
	}
	if req.Messages[0].Role != "system" {
		t.Errorf("first message role=%q, want system", req.Messages[0].Role)
	}
}

func TestBuildWireRequest_Stream(t *testing.T) {
	params := provider.RequestParams{
		Model:     "llama-3.1-8b-instant",
		MaxTokens: 100,
		Messages:  []model.Message{{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "hi"}}}},
	}
	mapper := NewIDMapper()
	req := buildWireRequest(params, mapper, true)

	if !req.Stream {
		t.Error("stream should be true")
	}
	if req.StreamOptions == nil || !req.StreamOptions.IncludeUsage {
		t.Error("stream_options.include_usage should be true")
	}
}

func TestBuildWireRequest_MaxTokensCapped(t *testing.T) {
	params := provider.RequestParams{
		Model:     "llama-3.3-70b-versatile", // MaxOutput: 32768
		MaxTokens: 100000,
		Messages:  []model.Message{{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "hi"}}}},
	}
	mapper := NewIDMapper()
	req := buildWireRequest(params, mapper, false)

	if req.MaxCompletionTokens != 32768 {
		t.Errorf("max_completion_tokens=%d, want 32768 (capped)", req.MaxCompletionTokens)
	}
}

func TestBuildWireRequest_WithTools(t *testing.T) {
	params := provider.RequestParams{
		Model:     "llama-3.3-70b-versatile",
		MaxTokens: 100,
		Messages:  []model.Message{{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "hi"}}}},
		Tools: []model.ToolDef{
			{Name: "bash", Description: "Run commands", InputSchema: json.RawMessage(`{"type":"object","properties":{"cmd":{"type":"string"}}}`)},
		},
	}
	mapper := NewIDMapper()
	req := buildWireRequest(params, mapper, false)

	if len(req.Tools) != 1 {
		t.Fatalf("got %d tools, want 1", len(req.Tools))
	}
	if req.Tools[0].Type != "function" {
		t.Errorf("tool type=%q, want function", req.Tools[0].Type)
	}
	if req.Tools[0].Function.Name != "bash" {
		t.Errorf("tool name=%q, want bash", req.Tools[0].Function.Name)
	}
	if req.ToolChoice != "auto" {
		t.Errorf("tool_choice=%v, want auto", req.ToolChoice)
	}
	if req.ParallelToolCalls == nil || !*req.ParallelToolCalls {
		t.Error("parallel_tool_calls should be true for llama-3.3")
	}
}

func TestBuildWireRequest_Reasoning(t *testing.T) {
	thinking := &provider.ThinkingConfig{Enabled: true, BudgetTokens: 5000}
	params := provider.RequestParams{
		Model:     "openai/gpt-oss-20b",
		MaxTokens: 1000,
		Messages:  []model.Message{{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "think"}}}},
		Thinking:  thinking,
	}
	mapper := NewIDMapper()
	req := buildWireRequest(params, mapper, false)

	if req.ReasoningFormat != "parsed" {
		t.Errorf("reasoning_format=%q, want parsed", req.ReasoningFormat)
	}
	if req.ReasoningEffort != "high" {
		t.Errorf("reasoning_effort=%q, want high", req.ReasoningEffort)
	}
}

func TestAssistantToWire_ToolCalls(t *testing.T) {
	mapper := NewIDMapper()
	mapper.RegisterPair("int-1", "call_abc")

	msg := model.Message{
		Role: model.RoleAssistant,
		Content: []model.ContentPart{
			model.TextPart{Text: "Let me run that."},
			model.ToolCallPart{ID: "int-1", Name: "bash", Input: json.RawMessage(`{"cmd":"ls"}`)},
		},
	}
	wire := assistantToWire(msg, mapper)

	if wire.Content != "Let me run that." {
		t.Errorf("content=%v", wire.Content)
	}
	if len(wire.ToolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1", len(wire.ToolCalls))
	}
	if wire.ToolCalls[0].ID != "call_abc" {
		t.Errorf("tool call id=%q, want call_abc", wire.ToolCalls[0].ID)
	}
	if wire.ToolCalls[0].Function.Arguments != `{"cmd":"ls"}` {
		t.Errorf("arguments=%q", wire.ToolCalls[0].Function.Arguments)
	}
}

func TestUserToWire_ToolResults(t *testing.T) {
	mapper := NewIDMapper()
	mapper.RegisterPair("int-1", "call_abc")

	msg := model.Message{
		Role: model.RoleUser,
		Content: []model.ContentPart{
			model.ToolResultPart{ToolCallID: "int-1", Content: "file.txt", IsError: false},
			model.TextPart{Text: "What next?"},
		},
	}
	toolNameMap := map[string]string{"int-1": "bash"}
	wires := userToWire(msg, mapper, toolNameMap)

	// Should produce 2 wire messages: tool result + user text
	if len(wires) != 2 {
		t.Fatalf("got %d wire messages, want 2", len(wires))
	}

	// First: tool result
	if wires[0].Role != "tool" {
		t.Errorf("first message role=%q, want tool", wires[0].Role)
	}
	if wires[0].ToolCallID != "call_abc" {
		t.Errorf("tool_call_id=%q, want call_abc", wires[0].ToolCallID)
	}
	if wires[0].Name != "bash" {
		t.Errorf("name=%q, want bash", wires[0].Name)
	}

	// Second: user text
	if wires[1].Role != "user" {
		t.Errorf("second message role=%q, want user", wires[1].Role)
	}
}

func TestUserToWire_ImagePart(t *testing.T) {
	mapper := NewIDMapper()
	msg := model.Message{
		Role: model.RoleUser,
		Content: []model.ContentPart{
			model.TextPart{Text: "What's in this image?"},
			model.ImagePart{MimeType: "image/png", Data: []byte{0x89, 0x50}},
		},
	}
	wires := userToWire(msg, mapper, nil)
	if len(wires) != 1 {
		t.Fatalf("got %d messages, want 1", len(wires))
	}
	// Should use content array (multipart)
	parts, ok := wires[0].Content.([]wireContentPart)
	if !ok {
		t.Fatalf("content should be []wireContentPart, got %T", wires[0].Content)
	}
	if len(parts) != 2 {
		t.Fatalf("got %d parts, want 2", len(parts))
	}
	if parts[1].Type != "image_url" {
		t.Errorf("part type=%q, want image_url", parts[1].Type)
	}
	if parts[1].ImageURL == nil {
		t.Fatal("image_url should not be nil")
	}
}

func TestUserToWire_SimpleText(t *testing.T) {
	mapper := NewIDMapper()
	msg := model.Message{
		Role:    model.RoleUser,
		Content: []model.ContentPart{model.TextPart{Text: "hello"}},
	}
	wires := userToWire(msg, mapper, nil)
	if len(wires) != 1 {
		t.Fatalf("got %d messages, want 1", len(wires))
	}
	// Simple text should be string content, not array
	if s, ok := wires[0].Content.(string); !ok || s != "hello" {
		t.Errorf("content=%v (type %T), want string 'hello'", wires[0].Content, wires[0].Content)
	}
}

func TestPrePopulateMapper(t *testing.T) {
	mapper := NewIDMapper()
	msgs := []model.Message{
		{
			Role: model.RoleAssistant,
			Content: []model.ContentPart{
				model.ToolCallPart{ID: "uuid-1", Name: "bash"},
				model.ToolCallPart{ID: "uuid-2", Name: "grep"},
			},
		},
	}
	prePopulateMapper(msgs, mapper)

	wire1 := mapper.ToWire("uuid-1")
	wire2 := mapper.ToWire("uuid-2")
	if wire1 == "" || wire2 == "" {
		t.Error("pre-populated IDs should not be empty")
	}
	if wire1 == wire2 {
		t.Error("different internal IDs should produce different wire IDs")
	}
}
