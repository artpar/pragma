package google

import (
	"encoding/json"
	"testing"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
)

func TestBuildWireRequest_BasicText(t *testing.T) {
	bus := observe.NewEventBus(16)
	params := provider.RequestParams{
		Model:     "gemini-2.5-flash",
		MaxTokens: 4096,
		Messages: []model.Message{
			{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "hello"}}},
		},
		System: model.SystemPrompt{
			Blocks: []model.SystemBlock{{Text: "You are helpful."}},
		},
	}

	mapper := NewIDMapper()
	req := buildWireRequest(params, mapper, bus)

	if req.SystemInstruction == nil {
		t.Fatal("SystemInstruction should be set")
	}
	if req.SystemInstruction.Parts[0].Text != "You are helpful." {
		t.Errorf("system text=%q", req.SystemInstruction.Parts[0].Text)
	}

	if len(req.Contents) != 1 {
		t.Fatalf("contents len=%d, want 1", len(req.Contents))
	}
	if req.Contents[0].Role != "user" {
		t.Errorf("role=%q, want user", req.Contents[0].Role)
	}
	if req.Contents[0].Parts[0].Text != "hello" {
		t.Errorf("text=%q", req.Contents[0].Parts[0].Text)
	}

	if req.GenerationConfig == nil || req.GenerationConfig.MaxOutputTokens != 4096 {
		t.Error("MaxOutputTokens not set correctly")
	}
}

func TestBuildWireRequest_WithTools(t *testing.T) {
	bus := observe.NewEventBus(16)
	params := provider.RequestParams{
		Model: "gemini-2.5-flash",
		Tools: []model.ToolDef{
			{Name: "search", Description: "Search", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
	}

	mapper := NewIDMapper()
	req := buildWireRequest(params, mapper, bus)

	if len(req.Tools) != 1 {
		t.Fatalf("tools len=%d, want 1", len(req.Tools))
	}
	if len(req.Tools[0].FunctionDeclarations) != 1 {
		t.Fatal("expected 1 function declaration")
	}
	decl := req.Tools[0].FunctionDeclarations[0]
	if decl.Name != "search" {
		t.Errorf("name=%q", decl.Name)
	}
	if req.ToolConfig == nil || req.ToolConfig.FunctionCallingConfig.Mode != "AUTO" {
		t.Error("tool config mode should be AUTO")
	}
}

func TestAssistantToWire_ToolCallPreservesId(t *testing.T) {
	mapper := NewIDMapper()
	// Simulate: Google returned a function call with id "g3-id-abc"
	mapper.RegisterPair("uuid-123", "g3-id-abc")

	msg := model.Message{
		Role: model.RoleAssistant,
		Content: []model.ContentPart{
			model.TextPart{Text: "Let me search."},
			model.ToolCallPart{
				ID:    "uuid-123",
				Name:  "search",
				Input: json.RawMessage(`{"query":"test"}`),
			},
		},
	}

	content := assistantToWire(msg, mapper)

	if content.Role != "model" {
		t.Errorf("role=%q, want model", content.Role)
	}
	if len(content.Parts) != 2 {
		t.Fatalf("parts len=%d, want 2", len(content.Parts))
	}
	fc := content.Parts[1].FunctionCall
	if fc == nil {
		t.Fatal("expected FunctionCall")
	}
	if fc.Name != "search" {
		t.Errorf("fc name=%q", fc.Name)
	}
	// The Google-provided id must be preserved on the wire
	if fc.Id != "g3-id-abc" {
		t.Errorf("fc id=%q, want g3-id-abc", fc.Id)
	}
}

func TestAssistantToWire_ThinkingPreservesSignature(t *testing.T) {
	mapper := NewIDMapper()

	msg := model.Message{
		Role: model.RoleAssistant,
		Content: []model.ContentPart{
			model.ThinkingPart{Text: "reasoning...", Signature: "sig_xyz"},
			model.TextPart{Text: "answer"},
		},
	}

	content := assistantToWire(msg, mapper)

	if len(content.Parts) != 2 {
		t.Fatalf("parts len=%d, want 2", len(content.Parts))
	}

	// First part: thinking with signature
	wp := content.Parts[0]
	if wp.Text != "reasoning..." {
		t.Errorf("thinking text=%q", wp.Text)
	}
	if wp.Thought == nil || !*wp.Thought {
		t.Error("thought should be true")
	}
	if wp.ThoughtSignature != "sig_xyz" {
		t.Errorf("thoughtSignature=%q, want sig_xyz", wp.ThoughtSignature)
	}
}

func TestUserToWire_ToolResultWithId(t *testing.T) {
	mapper := NewIDMapper()
	// Simulate: the FunctionCall had Google id "g3-id-abc"
	mapper.RegisterPair("uuid-123", "g3-id-abc")
	toolNameMap := map[string]string{"uuid-123": "search"}
	bus := observe.NewEventBus(16)

	msg := model.Message{
		Role: model.RoleUser,
		Content: []model.ContentPart{
			model.ToolResultPart{
				ToolCallID: "uuid-123",
				Content:    "found 3 results",
			},
		},
	}

	contents := userToWire(msg, mapper, toolNameMap, bus)

	fr := contents[0].Parts[0].FunctionResponse
	if fr == nil {
		t.Fatal("expected FunctionResponse")
	}
	if fr.Name != "search" {
		t.Errorf("fr name=%q", fr.Name)
	}
	// The id must match the original FunctionCall's id
	if fr.Id != "g3-id-abc" {
		t.Errorf("fr id=%q, want g3-id-abc", fr.Id)
	}
}

func TestUserToWire_ErrorResult(t *testing.T) {
	mapper := NewIDMapper()
	toolNameMap := map[string]string{"uuid-err": "cmd"}
	bus := observe.NewEventBus(16)

	msg := model.Message{
		Role: model.RoleUser,
		Content: []model.ContentPart{
			model.ToolResultPart{
				ToolCallID: "uuid-err",
				Content:    "command failed",
				IsError:    true,
			},
		},
	}

	contents := userToWire(msg, mapper, toolNameMap, bus)
	fr := contents[0].Parts[0].FunctionResponse

	var resp map[string]string
	json.Unmarshal(fr.Response, &resp)
	if resp["error"] != "command failed" {
		t.Errorf("error field=%q", resp["error"])
	}
	if _, ok := resp["output"]; ok {
		t.Error("error results should not have output field")
	}
}

func TestToolsToWire(t *testing.T) {
	tools := []model.ToolDef{
		{Name: "a", Description: "desc a", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "b", Description: "desc b", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}

	wire := toolsToWire(tools)
	if len(wire) != 1 {
		t.Fatalf("should wrap in single tool, got %d", len(wire))
	}
	if len(wire[0].FunctionDeclarations) != 2 {
		t.Fatalf("declarations=%d, want 2", len(wire[0].FunctionDeclarations))
	}
}

func TestBuildWireRequest_ThinkingConfig(t *testing.T) {
	bus := observe.NewEventBus(16)
	params := provider.RequestParams{
		Model: "gemini-2.5-flash",
		Thinking: &provider.ThinkingConfig{
			Enabled:      true,
			BudgetTokens: 8192,
		},
	}

	mapper := NewIDMapper()
	req := buildWireRequest(params, mapper, bus)

	if req.GenerationConfig == nil || req.GenerationConfig.ThinkingConfig == nil {
		t.Fatal("ThinkingConfig should be set")
	}
	tc := req.GenerationConfig.ThinkingConfig
	if !tc.IncludeThoughts {
		t.Error("IncludeThoughts should be true")
	}
	if tc.ThinkingBudget != 8192 {
		t.Errorf("budget=%d, want 8192", tc.ThinkingBudget)
	}
}

func TestBuildWireRequest_ThinkingGemini3UsesLevel(t *testing.T) {
	bus := observe.NewEventBus(16)
	params := provider.RequestParams{
		Model: "gemini-3-flash-preview",
		Thinking: &provider.ThinkingConfig{
			Enabled:      true,
			BudgetTokens: 10000,
		},
	}

	mapper := NewIDMapper()
	req := buildWireRequest(params, mapper, bus)

	tc := req.GenerationConfig.ThinkingConfig
	if tc == nil {
		t.Fatal("ThinkingConfig should be set")
	}
	if tc.ThinkingLevel != "high" {
		t.Errorf("thinkingLevel=%q, want high", tc.ThinkingLevel)
	}
	if tc.ThinkingBudget != 0 {
		t.Errorf("thinkingBudget=%d, want 0 (Gemini 3 should use level)", tc.ThinkingBudget)
	}
}

func TestSanitizeSchema_StripsUnsupportedFields(t *testing.T) {
	input := json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {"type": "string", "default": "foo", "description": "A name"},
			"flag": {"type": "boolean", "default": false}
		},
		"required": ["name"],
		"additionalProperties": false
	}`)

	result := sanitizeSchema(input)
	var obj map[string]any
	if err := json.Unmarshal(result, &obj); err != nil {
		t.Fatal(err)
	}

	if _, has := obj["additionalProperties"]; has {
		t.Error("additionalProperties should be stripped")
	}
	props := obj["properties"].(map[string]any)
	name := props["name"].(map[string]any)
	if _, has := name["default"]; has {
		t.Error("default should be stripped from name property")
	}
	if _, has := name["description"]; !has {
		t.Error("description should be preserved")
	}
	if _, has := name["type"]; !has {
		t.Error("type should be preserved")
	}
	flag := props["flag"].(map[string]any)
	if _, has := flag["default"]; has {
		t.Error("default should be stripped from flag property")
	}
	// required should survive
	if _, has := obj["required"]; !has {
		t.Error("required should be preserved")
	}
}

func TestSanitizeSchema_RecursesAnyOf(t *testing.T) {
	input := json.RawMessage(`{
		"anyOf": [
			{"type": "string", "minLength": 1},
			{"type": "integer", "minimum": 0, "maximum": 100}
		],
		"$defs": {
			"thing": {"type": "object"}
		}
	}`)

	result := sanitizeSchema(input)
	var obj map[string]any
	json.Unmarshal(result, &obj)

	anyOf := obj["anyOf"].([]any)
	strVariant := anyOf[0].(map[string]any)
	if _, has := strVariant["minLength"]; has {
		t.Error("minLength should be stripped from anyOf variant")
	}
	intVariant := anyOf[1].(map[string]any)
	if _, has := intVariant["minimum"]; has {
		t.Error("minimum should be stripped from anyOf variant")
	}

	// $defs is not supported in function declarations — should be stripped entirely
	if _, has := obj["$defs"]; has {
		t.Error("$defs should be stripped (not supported in function declarations)")
	}
}

func TestBudgetToLevel(t *testing.T) {
	tests := []struct {
		budget int
		want   string
	}{
		{512, "low"},
		{1024, "low"},
		{4096, "medium"},
		{8192, "medium"},
		{10000, "high"},
		{100000, "high"},
	}
	for _, tt := range tests {
		if got := budgetToLevel(tt.budget); got != tt.want {
			t.Errorf("budgetToLevel(%d)=%q, want %q", tt.budget, got, tt.want)
		}
	}
}

func TestBuildWireRequest_ThinkingNotSupportedOnOldModel(t *testing.T) {
	bus := observe.NewEventBus(16)
	params := provider.RequestParams{
		Model: "gemini-2.0-flash", // does NOT support thinking
		Thinking: &provider.ThinkingConfig{
			Enabled:      true,
			BudgetTokens: 8192,
		},
	}

	mapper := NewIDMapper()
	req := buildWireRequest(params, mapper, bus)

	if req.GenerationConfig != nil && req.GenerationConfig.ThinkingConfig != nil {
		t.Error("ThinkingConfig should not be set for non-thinking model")
	}
}
