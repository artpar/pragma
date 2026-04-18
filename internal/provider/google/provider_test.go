package google

import (
	"encoding/json"
	"testing"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/provider"
	"google.golang.org/genai"
)

// --- SupportsFeature ---

func TestSupportsFeatureToolUse(t *testing.T) {
	p := &Provider{}
	if !p.SupportsFeature(provider.FeatureToolUse) {
		t.Error("expected FeatureToolUse supported")
	}
}

func TestSupportsFeatureStreaming(t *testing.T) {
	p := &Provider{}
	if !p.SupportsFeature(provider.FeatureStreaming) {
		t.Error("expected FeatureStreaming supported")
	}
}

func TestSupportsFeatureImages(t *testing.T) {
	p := &Provider{}
	if !p.SupportsFeature(provider.FeatureImages) {
		t.Error("expected FeatureImages supported")
	}
}

func TestSupportsFeatureThinking(t *testing.T) {
	p := &Provider{}
	if !p.SupportsFeature(provider.FeatureThinking) {
		t.Error("expected FeatureThinking supported")
	}
}

func TestSupportsFeatureStructuredOutput(t *testing.T) {
	p := &Provider{}
	if !p.SupportsFeature(provider.FeatureStructuredOutput) {
		t.Error("expected FeatureStructuredOutput supported")
	}
}

func TestSupportsFeaturePrefixCaching(t *testing.T) {
	p := &Provider{}
	if !p.SupportsFeature(provider.FeaturePrefixCaching) {
		t.Error("expected FeaturePrefixCaching supported")
	}
}

func TestSupportsFeatureUnknown(t *testing.T) {
	p := &Provider{}
	if p.SupportsFeature("nonexistent_feature") {
		t.Error("expected unknown feature not supported")
	}
}

// --- TokenCounter interface ---

func TestProviderImplementsTokenCounter(t *testing.T) {
	var p interface{} = &Provider{}
	if _, ok := p.(provider.TokenCounter); !ok {
		t.Error("Provider does not implement provider.TokenCounter")
	}
}

// --- messagesToGenai ---

func TestMessagesToGenaiUserText(t *testing.T) {
	msgs := []model.Message{
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "hello"}}},
	}
	contents := messagesToGenai(msgs)
	if len(contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(contents))
	}
	if contents[0].Role != "user" {
		t.Errorf("role: got %q, want %q", contents[0].Role, "user")
	}
	if len(contents[0].Parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(contents[0].Parts))
	}
	if contents[0].Parts[0].Text != "hello" {
		t.Errorf("text: got %q, want %q", contents[0].Parts[0].Text, "hello")
	}
}

func TestMessagesToGenaiAssistantText(t *testing.T) {
	msgs := []model.Message{
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: "hi"}}},
	}
	contents := messagesToGenai(msgs)
	if len(contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(contents))
	}
	if contents[0].Role != "model" {
		t.Errorf("role: got %q, want %q", contents[0].Role, "model")
	}
}

func TestMessagesToGenaiToolCallAndResult(t *testing.T) {
	msgs := []model.Message{
		{
			Role: model.RoleAssistant,
			Content: []model.ContentPart{
				model.ToolCallPart{ID: "call-1", Name: "Read", Input: json.RawMessage(`{"path":"/foo"}`)},
			},
		},
		{
			Role: model.RoleUser,
			Content: []model.ContentPart{
				model.ToolResultPart{ToolCallID: "call-1", Content: `{"result":"file contents"}`},
			},
		},
	}
	contents := messagesToGenai(msgs)
	// Expected: assistant message with function call, then tool response as user, then no extra user content
	if len(contents) < 2 {
		t.Fatalf("expected at least 2 contents, got %d", len(contents))
	}

	// First should be model role with function call
	if contents[0].Role != "model" {
		t.Errorf("contents[0].Role: got %q, want %q", contents[0].Role, "model")
	}
	if len(contents[0].Parts) != 1 {
		t.Fatalf("expected 1 part in model content, got %d", len(contents[0].Parts))
	}
	if contents[0].Parts[0].FunctionCall == nil {
		t.Fatal("expected FunctionCall part in model content")
	}
	if contents[0].Parts[0].FunctionCall.Name != "Read" {
		t.Errorf("function call name: got %q, want %q", contents[0].Parts[0].FunctionCall.Name, "Read")
	}

	// Second should be user role with function response
	foundFuncResp := false
	for _, c := range contents[1:] {
		if c.Role == "user" {
			for _, p := range c.Parts {
				if p.FunctionResponse != nil {
					foundFuncResp = true
					if p.FunctionResponse.Name != "Read" {
						t.Errorf("function response name: got %q, want %q", p.FunctionResponse.Name, "Read")
					}
				}
			}
		}
	}
	if !foundFuncResp {
		t.Error("expected function response part in user content")
	}
}

func TestMessagesToGenaiToolResultFallbackName(t *testing.T) {
	// Tool result with no matching tool call should use "function" as name
	msgs := []model.Message{
		{
			Role: model.RoleUser,
			Content: []model.ContentPart{
				model.ToolResultPart{ToolCallID: "orphan-id", Content: `"ok"`},
			},
		},
	}
	contents := messagesToGenai(msgs)
	foundResp := false
	for _, c := range contents {
		for _, p := range c.Parts {
			if p.FunctionResponse != nil {
				foundResp = true
				if p.FunctionResponse.Name != "function" {
					t.Errorf("expected fallback name %q, got %q", "function", p.FunctionResponse.Name)
				}
			}
		}
	}
	if !foundResp {
		t.Error("expected function response for orphaned tool result")
	}
}

func TestMessagesToGenaiThinkingPart(t *testing.T) {
	msgs := []model.Message{
		{
			Role: model.RoleAssistant,
			Content: []model.ContentPart{
				model.ThinkingPart{Text: "let me think", Signature: "sig123"},
			},
		},
	}
	contents := messagesToGenai(msgs)
	if len(contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(contents))
	}
	part := contents[0].Parts[0]
	if !part.Thought {
		t.Error("expected Thought=true")
	}
	if part.Text != "let me think" {
		t.Errorf("text: got %q", part.Text)
	}
	if string(part.ThoughtSignature) != "sig123" {
		t.Errorf("signature: got %q", part.ThoughtSignature)
	}
}

func TestMessagesToGenaiImagePart(t *testing.T) {
	data := []byte{0x89, 0x50, 0x4e, 0x47}
	msgs := []model.Message{
		{
			Role: model.RoleUser,
			Content: []model.ContentPart{
				model.ImagePart{MimeType: "image/png", Data: data},
			},
		},
	}
	contents := messagesToGenai(msgs)
	if len(contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(contents))
	}
	part := contents[0].Parts[0]
	if part.InlineData == nil {
		t.Fatal("expected InlineData")
	}
	if part.InlineData.MIMEType != "image/png" {
		t.Errorf("mime: got %q", part.InlineData.MIMEType)
	}
	if len(part.InlineData.Data) != 4 {
		t.Errorf("data len: got %d, want 4", len(part.InlineData.Data))
	}
}

func TestMessagesToGenaiEmptyTextSkipped(t *testing.T) {
	msgs := []model.Message{
		{
			Role:    model.RoleUser,
			Content: []model.ContentPart{model.TextPart{Text: ""}},
		},
	}
	contents := messagesToGenai(msgs)
	// Empty text parts should be skipped, resulting in no content
	if len(contents) != 0 {
		t.Errorf("expected 0 contents for empty text, got %d", len(contents))
	}
}

func TestMessagesToGenaiToolResultNonJSON(t *testing.T) {
	// Non-JSON tool result content should be wrapped as {"result": ...}
	msgs := []model.Message{
		{
			Role: model.RoleAssistant,
			Content: []model.ContentPart{
				model.ToolCallPart{ID: "c1", Name: "Bash", Input: json.RawMessage(`{}`)},
			},
		},
		{
			Role: model.RoleUser,
			Content: []model.ContentPart{
				model.ToolResultPart{ToolCallID: "c1", Content: "plain text output"},
			},
		},
	}
	contents := messagesToGenai(msgs)
	foundResp := false
	for _, c := range contents {
		for _, p := range c.Parts {
			if p.FunctionResponse != nil {
				foundResp = true
				resp := p.FunctionResponse.Response
				if resp == nil {
					t.Fatal("expected non-nil response map")
				}
				if v, ok := resp["result"]; !ok || v != "plain text output" {
					t.Errorf("expected {\"result\":\"plain text output\"}, got %v", resp)
				}
			}
		}
	}
	if !foundResp {
		t.Error("expected function response for tool result")
	}
}

// --- buildRequest ---

func TestBuildRequestSystemPrompt(t *testing.T) {
	p := &Provider{}
	_, cfg := p.buildRequest(provider.RequestParams{
		System: model.SystemPrompt{
			Blocks: []model.SystemBlock{
				{Text: "block one"},
				{Text: "block two"},
			},
		},
	})
	if cfg.SystemInstruction == nil {
		t.Fatal("expected SystemInstruction")
	}
	if len(cfg.SystemInstruction.Parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(cfg.SystemInstruction.Parts))
	}
	want := "block one\n\nblock two"
	if cfg.SystemInstruction.Parts[0].Text != want {
		t.Errorf("system text: got %q, want %q", cfg.SystemInstruction.Parts[0].Text, want)
	}
}

func TestBuildRequestNoSystemPrompt(t *testing.T) {
	p := &Provider{}
	_, cfg := p.buildRequest(provider.RequestParams{})
	if cfg.SystemInstruction != nil {
		t.Error("expected nil SystemInstruction for empty system blocks")
	}
}

func TestBuildRequestMaxTokens(t *testing.T) {
	p := &Provider{}
	_, cfg := p.buildRequest(provider.RequestParams{MaxTokens: 4096})
	if cfg.MaxOutputTokens != 4096 {
		t.Errorf("MaxOutputTokens: got %d, want 4096", cfg.MaxOutputTokens)
	}
}

func TestBuildRequestTemperature(t *testing.T) {
	p := &Provider{}
	temp := 0.7
	_, cfg := p.buildRequest(provider.RequestParams{Temperature: &temp})
	if cfg.Temperature == nil {
		t.Fatal("expected Temperature set")
	}
	if *cfg.Temperature != 0.7 {
		t.Errorf("Temperature: got %f, want 0.7", *cfg.Temperature)
	}
}

func TestBuildRequestTools(t *testing.T) {
	p := &Provider{}
	_, cfg := p.buildRequest(provider.RequestParams{
		Tools: []model.ToolDef{
			{Name: "Read", Description: "Read a file", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"file path"}}}`)},
		},
	})
	if len(cfg.Tools) != 1 {
		t.Fatalf("expected 1 tool group, got %d", len(cfg.Tools))
	}
	if len(cfg.Tools[0].FunctionDeclarations) != 1 {
		t.Fatalf("expected 1 function declaration, got %d", len(cfg.Tools[0].FunctionDeclarations))
	}
	if cfg.Tools[0].FunctionDeclarations[0].Name != "Read" {
		t.Errorf("tool name: got %q", cfg.Tools[0].FunctionDeclarations[0].Name)
	}
}

func TestBuildRequestThinkingEnabled(t *testing.T) {
	p := &Provider{}
	_, cfg := p.buildRequest(provider.RequestParams{
		Model:    "gemini-2.5-pro",
		Thinking: &provider.ThinkingConfig{Enabled: true, BudgetTokens: 8000},
	})
	if cfg.ThinkingConfig == nil {
		t.Fatal("expected ThinkingConfig")
	}
	if !cfg.ThinkingConfig.IncludeThoughts {
		t.Error("expected IncludeThoughts=true")
	}
	if cfg.ThinkingConfig.ThinkingBudget == nil || *cfg.ThinkingConfig.ThinkingBudget != 8000 {
		t.Errorf("ThinkingBudget: got %v", cfg.ThinkingConfig.ThinkingBudget)
	}
}

func TestBuildRequestFlashModelZeroBudget(t *testing.T) {
	p := &Provider{}
	_, cfg := p.buildRequest(provider.RequestParams{Model: "gemini-2.5-flash"})
	if cfg.ThinkingConfig == nil {
		t.Fatal("expected ThinkingConfig for flash model")
	}
	if cfg.ThinkingConfig.ThinkingBudget == nil || *cfg.ThinkingConfig.ThinkingBudget != 0 {
		t.Errorf("expected zero budget for flash model, got %v", cfg.ThinkingConfig.ThinkingBudget)
	}
}

func TestBuildRequestResponseSchema(t *testing.T) {
	p := &Provider{}
	schema := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`)
	_, cfg := p.buildRequest(provider.RequestParams{
		ResponseSchema: schema,
	})
	if cfg.ResponseMIMEType != "application/json" {
		t.Errorf("ResponseMIMEType: got %q, want %q", cfg.ResponseMIMEType, "application/json")
	}
	if cfg.ResponseSchema == nil {
		t.Fatal("expected ResponseSchema to be set")
	}
}

func TestBuildRequestResponseSchemaEmpty(t *testing.T) {
	p := &Provider{}
	_, cfg := p.buildRequest(provider.RequestParams{})
	if cfg.ResponseMIMEType != "" {
		t.Errorf("expected empty ResponseMIMEType, got %q", cfg.ResponseMIMEType)
	}
	if cfg.ResponseSchema != nil {
		t.Error("expected nil ResponseSchema")
	}
}

// --- rawJSONToGenaiSchema ---

func TestRawJSONToGenaiSchemaSimple(t *testing.T) {
	raw := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","description":"user name"}},"required":["name"]}`)
	schema := rawJSONToGenaiSchema(raw)
	if schema == nil {
		t.Fatal("expected non-nil schema")
	}
	if schema.Type != "object" {
		t.Errorf("type: got %q, want %q", schema.Type, "object")
	}
	if schema.Properties == nil {
		t.Fatal("expected properties")
	}
	if _, ok := schema.Properties["name"]; !ok {
		t.Error("expected 'name' property")
	}
}

func TestRawJSONToGenaiSchemaWithDisallowedFields(t *testing.T) {
	// Schema with fields Gemini doesn't support — should be stripped
	raw := json.RawMessage(`{"type":"object","default":"foo","additionalProperties":false,"$schema":"draft-07","properties":{"x":{"type":"string"}}}`)
	schema := rawJSONToGenaiSchema(raw)
	if schema == nil {
		t.Fatal("expected non-nil schema")
	}
	// Verify the sanitized schema doesn't have the removed fields by re-marshaling
	out, _ := json.Marshal(schema)
	var m map[string]any
	json.Unmarshal(out, &m)
	for _, banned := range []string{"default", "additionalProperties", "$schema"} {
		if _, ok := m[banned]; ok {
			t.Errorf("expected %q to be stripped from schema", banned)
		}
	}
}

func TestRawJSONToGenaiSchemaInvalid(t *testing.T) {
	schema := rawJSONToGenaiSchema(json.RawMessage(`not json`))
	if schema != nil {
		t.Error("expected nil for invalid JSON")
	}
}

func TestRawJSONToGenaiSchemaEmpty(t *testing.T) {
	schema := rawJSONToGenaiSchema(json.RawMessage(`{}`))
	if schema == nil {
		t.Fatal("expected non-nil schema for empty object")
	}
}

// --- sanitizeSchema ---

func TestSanitizeSchemaRemovesDefault(t *testing.T) {
	raw := json.RawMessage(`{"type":"string","default":"hello"}`)
	sanitized := sanitizeSchema(raw)
	var m map[string]any
	json.Unmarshal(sanitized, &m)
	if _, ok := m["default"]; ok {
		t.Error("expected 'default' to be removed")
	}
	if m["type"] != "string" {
		t.Errorf("type should be preserved, got %v", m["type"])
	}
}

func TestSanitizeSchemaRemovesAdditionalProperties(t *testing.T) {
	raw := json.RawMessage(`{"type":"object","additionalProperties":false}`)
	sanitized := sanitizeSchema(raw)
	var m map[string]any
	json.Unmarshal(sanitized, &m)
	if _, ok := m["additionalProperties"]; ok {
		t.Error("expected 'additionalProperties' to be removed")
	}
}

func TestSanitizeSchemaFiltersEmptyEnumStrings(t *testing.T) {
	raw := json.RawMessage(`{"type":"string","enum":["","foo","bar"]}`)
	sanitized := sanitizeSchema(raw)
	var m map[string]any
	json.Unmarshal(sanitized, &m)
	enum, ok := m["enum"].([]any)
	if !ok {
		t.Fatal("expected enum array")
	}
	if len(enum) != 2 {
		t.Errorf("expected 2 enum values after filtering, got %d", len(enum))
	}
	for _, v := range enum {
		if v == "" {
			t.Error("empty string should have been filtered from enum")
		}
	}
}

func TestSanitizeSchemaDeletesEnumIfAllEmpty(t *testing.T) {
	raw := json.RawMessage(`{"type":"string","enum":[""]}`)
	sanitized := sanitizeSchema(raw)
	var m map[string]any
	json.Unmarshal(sanitized, &m)
	if _, ok := m["enum"]; ok {
		t.Error("expected 'enum' to be deleted when all values are empty strings")
	}
}

func TestSanitizeSchemaNestedProperties(t *testing.T) {
	raw := json.RawMessage(`{"type":"object","properties":{"child":{"type":"string","default":"x","description":"a child"}}}`)
	sanitized := sanitizeSchema(raw)
	var m map[string]any
	json.Unmarshal(sanitized, &m)
	props := m["properties"].(map[string]any)
	child := props["child"].(map[string]any)
	if _, ok := child["default"]; ok {
		t.Error("expected 'default' removed from nested property")
	}
	if child["description"] != "a child" {
		t.Error("expected 'description' preserved in nested property")
	}
}

func TestSanitizeSchemaAnyOf(t *testing.T) {
	raw := json.RawMessage(`{"anyOf":[{"type":"string","default":"x"},{"type":"number"}]}`)
	sanitized := sanitizeSchema(raw)
	var m map[string]any
	json.Unmarshal(sanitized, &m)
	anyOf := m["anyOf"].([]any)
	first := anyOf[0].(map[string]any)
	if _, ok := first["default"]; ok {
		t.Error("expected 'default' removed from anyOf variant")
	}
}

func TestSanitizeSchemaItems(t *testing.T) {
	raw := json.RawMessage(`{"type":"array","items":{"type":"string","default":"x"}}`)
	sanitized := sanitizeSchema(raw)
	var m map[string]any
	json.Unmarshal(sanitized, &m)
	items := m["items"].(map[string]any)
	if _, ok := items["default"]; ok {
		t.Error("expected 'default' removed from items")
	}
}

func TestSanitizeSchemaInvalidJSON(t *testing.T) {
	raw := json.RawMessage(`not json`)
	sanitized := sanitizeSchema(raw)
	// Should return raw input unchanged
	if string(sanitized) != "not json" {
		t.Errorf("expected raw returned for invalid JSON, got %q", string(sanitized))
	}
}

// --- toolsToGenai ---

func TestToolsToGenaiSingle(t *testing.T) {
	tools := []model.ToolDef{
		{
			Name:        "Bash",
			Description: "Run a command",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}`),
		},
	}
	result := toolsToGenai(tools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool group, got %d", len(result))
	}
	if len(result[0].FunctionDeclarations) != 1 {
		t.Fatalf("expected 1 declaration, got %d", len(result[0].FunctionDeclarations))
	}
	decl := result[0].FunctionDeclarations[0]
	if decl.Name != "Bash" {
		t.Errorf("name: got %q", decl.Name)
	}
	if decl.Description != "Run a command" {
		t.Errorf("description: got %q", decl.Description)
	}
	if decl.Parameters == nil {
		t.Fatal("expected Parameters schema")
	}
}

func TestToolsToGenaiMultiple(t *testing.T) {
	tools := []model.ToolDef{
		{Name: "Read", Description: "Read file", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "Write", Description: "Write file", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "Bash", Description: "Run cmd", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	result := toolsToGenai(tools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool group, got %d", len(result))
	}
	if len(result[0].FunctionDeclarations) != 3 {
		t.Fatalf("expected 3 declarations, got %d", len(result[0].FunctionDeclarations))
	}
}

func TestToolsToGenaiNoSchema(t *testing.T) {
	tools := []model.ToolDef{
		{Name: "NoSchema", Description: "A tool without schema"},
	}
	result := toolsToGenai(tools)
	if result[0].FunctionDeclarations[0].Parameters != nil {
		t.Error("expected nil Parameters when no InputSchema")
	}
}

// --- responseFromGenai ---

func TestResponseFromGenaiTextOnly(t *testing.T) {
	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []*genai.Part{{Text: "hello world"}},
				},
				FinishReason: genai.FinishReasonStop,
			},
		},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     100,
			CandidatesTokenCount: 10,
		},
	}
	result := responseFromGenai(resp, "gemini-2.5-flash")
	if result.Model != "gemini-2.5-flash" {
		t.Errorf("model: got %q", result.Model)
	}
	if result.StopReason != model.StopEndTurn {
		t.Errorf("stop reason: got %q", result.StopReason)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content part, got %d", len(result.Content))
	}
	tp, ok := result.Content[0].(model.TextPart)
	if !ok {
		t.Fatalf("expected TextPart, got %T", result.Content[0])
	}
	if tp.Text != "hello world" {
		t.Errorf("text: got %q", tp.Text)
	}
	if result.Usage.InputTokens != 100 {
		t.Errorf("input tokens: got %d", result.Usage.InputTokens)
	}
	if result.Usage.OutputTokens != 10 {
		t.Errorf("output tokens: got %d", result.Usage.OutputTokens)
	}
}

func TestResponseFromGenaiWithToolCalls(t *testing.T) {
	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{FunctionCall: &genai.FunctionCall{Name: "Read", ID: "fc-1", Args: map[string]any{"path": "/foo"}}},
					},
				},
				FinishReason: genai.FinishReasonStop,
			},
		},
	}
	result := responseFromGenai(resp, "gemini-2.5-flash")
	// StopEndTurn should be corrected to StopToolUse when tool calls present
	if result.StopReason != model.StopToolUse {
		t.Errorf("stop reason: got %q, want %q", result.StopReason, model.StopToolUse)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content part, got %d", len(result.Content))
	}
	tc, ok := result.Content[0].(model.ToolCallPart)
	if !ok {
		t.Fatalf("expected ToolCallPart, got %T", result.Content[0])
	}
	if tc.Name != "Read" {
		t.Errorf("tool name: got %q", tc.Name)
	}
	if tc.ID != "fc-1" {
		t.Errorf("tool ID: got %q", tc.ID)
	}
}

func TestResponseFromGenaiToolCallNoID(t *testing.T) {
	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{FunctionCall: &genai.FunctionCall{Name: "Bash", Args: map[string]any{}}},
					},
				},
				FinishReason: genai.FinishReasonStop,
			},
		},
	}
	result := responseFromGenai(resp, "test")
	tc := result.Content[0].(model.ToolCallPart)
	if tc.ID == "" {
		t.Error("expected generated UUID for tool call with no ID")
	}
}

func TestResponseFromGenaiThinkingPart(t *testing.T) {
	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{Text: "thinking...", Thought: true, ThoughtSignature: []byte("sig-abc")},
						{Text: "answer"},
					},
				},
				FinishReason: genai.FinishReasonStop,
			},
		},
	}
	result := responseFromGenai(resp, "test")
	if len(result.Content) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(result.Content))
	}
	tp, ok := result.Content[0].(model.ThinkingPart)
	if !ok {
		t.Fatalf("expected ThinkingPart, got %T", result.Content[0])
	}
	if tp.Text != "thinking..." {
		t.Errorf("thinking text: got %q", tp.Text)
	}
	if tp.Signature != "sig-abc" {
		t.Errorf("signature: got %q", tp.Signature)
	}
}

func TestResponseFromGenaiNoCandidates(t *testing.T) {
	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{},
	}
	result := responseFromGenai(resp, "test")
	if result.StopReason != model.StopError {
		t.Errorf("expected StopError for no candidates, got %q", result.StopReason)
	}
}

func TestResponseFromGenaiNilContent(t *testing.T) {
	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{Content: nil, FinishReason: genai.FinishReasonStop},
		},
	}
	result := responseFromGenai(resp, "test")
	if result.StopReason != model.StopEndTurn {
		t.Errorf("stop reason: got %q", result.StopReason)
	}
	if len(result.Content) != 0 {
		t.Errorf("expected 0 content parts, got %d", len(result.Content))
	}
}

func TestResponseFromGenaiMaxTokens(t *testing.T) {
	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content:      &genai.Content{Parts: []*genai.Part{{Text: "truncated"}}},
				FinishReason: genai.FinishReasonMaxTokens,
			},
		},
	}
	result := responseFromGenai(resp, "test")
	if result.StopReason != model.StopMaxTokens {
		t.Errorf("expected StopMaxTokens, got %q", result.StopReason)
	}
}

// --- usageFromGenai ---

func TestUsageFromGenai(t *testing.T) {
	u := &genai.GenerateContentResponseUsageMetadata{
		PromptTokenCount:        500,
		CandidatesTokenCount:    200,
		ThoughtsTokenCount:      50,
		CachedContentTokenCount: 100,
	}
	usage := usageFromGenai(u)
	// InputTokens = PromptTokenCount - CachedContentTokenCount
	// (PromptTokenCount includes cached tokens per SDK docs, must subtract to avoid double-billing)
	if usage.InputTokens != 400 {
		t.Errorf("InputTokens: got %d, want 400 (500 - 100 cached)", usage.InputTokens)
	}
	// OutputTokens = CandidatesTokenCount + ThoughtsTokenCount
	if usage.OutputTokens != 250 {
		t.Errorf("OutputTokens: got %d, want 250", usage.OutputTokens)
	}
	if usage.CacheReadInputTokens != 100 {
		t.Errorf("CacheReadInputTokens: got %d, want 100", usage.CacheReadInputTokens)
	}
}

func TestUsageFromGenaiNoCaching(t *testing.T) {
	u := &genai.GenerateContentResponseUsageMetadata{
		PromptTokenCount:     1000,
		CandidatesTokenCount: 300,
	}
	usage := usageFromGenai(u)
	// No caching: InputTokens = PromptTokenCount (CachedContentTokenCount is 0)
	if usage.InputTokens != 1000 {
		t.Errorf("InputTokens: got %d, want 1000", usage.InputTokens)
	}
	if usage.CacheReadInputTokens != 0 {
		t.Errorf("CacheReadInputTokens: got %d, want 0", usage.CacheReadInputTokens)
	}
}

// --- stopReasonFromGenai ---

func TestStopReasonFromGenai(t *testing.T) {
	tests := []struct {
		input genai.FinishReason
		want  model.StopReason
	}{
		{genai.FinishReasonStop, model.StopEndTurn},
		{genai.FinishReasonUnspecified, model.StopEndTurn},
		{genai.FinishReasonMaxTokens, model.StopMaxTokens},
		{genai.FinishReasonMalformedFunctionCall, model.StopMalformedToolCall},
		{genai.FinishReasonSafety, model.StopContentFiltered},
		{genai.FinishReasonRecitation, model.StopContentFiltered},
		{genai.FinishReasonBlocklist, model.StopContentFiltered},
		{genai.FinishReasonProhibitedContent, model.StopContentFiltered},
		{genai.FinishReasonSPII, model.StopContentFiltered},
		{genai.FinishReasonImageSafety, model.StopContentFiltered},
		{genai.FinishReasonImageProhibitedContent, model.StopContentFiltered},
		{genai.FinishReasonImageRecitation, model.StopContentFiltered},
		{genai.FinishReasonImageOther, model.StopContentFiltered},
		{genai.FinishReasonLanguage, model.StopError},
		{genai.FinishReasonOther, model.StopError},
		{genai.FinishReasonUnexpectedToolCall, model.StopError},
	}
	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			got := stopReasonFromGenai(tt.input)
			if got != tt.want {
				t.Errorf("stopReasonFromGenai(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- Pricing and ContextWindow ---

func TestPricingKnownModel(t *testing.T) {
	p := &Provider{}
	pricing, ok := p.Pricing("gemini-2.5-pro")
	if !ok {
		t.Fatal("expected pricing for gemini-2.5-pro")
	}
	if pricing.InputPerMToken != 1.25 {
		t.Errorf("InputPerMToken: got %f", pricing.InputPerMToken)
	}
	if pricing.CacheReadPerMToken != 0.125 {
		t.Errorf("CacheReadPerMToken: got %f, want 0.125", pricing.CacheReadPerMToken)
	}
	if pricing.CacheCreatePerMToken != 0 {
		t.Errorf("CacheCreatePerMToken should be 0 (Google charges hourly storage), got %f", pricing.CacheCreatePerMToken)
	}
}

func TestPricingUnknownModel(t *testing.T) {
	p := &Provider{}
	_, ok := p.Pricing("nonexistent-model")
	if ok {
		t.Error("expected no pricing for unknown model")
	}
}

func TestContextWindowKnownModel(t *testing.T) {
	p := &Provider{}
	window, ok := p.ContextWindow("gemini-2.5-flash")
	if !ok {
		t.Fatal("expected context window for gemini-2.5-flash")
	}
	if window != 1_048_576 {
		t.Errorf("context window: got %d, want 1048576", window)
	}
}

func TestContextWindowUnknownModel(t *testing.T) {
	p := &Provider{}
	window, ok := p.ContextWindow("unknown")
	if ok {
		t.Error("expected ok=false for unknown model")
	}
	if window != 1_048_576 {
		t.Errorf("expected fallback 1048576, got %d", window)
	}
}

func TestProviderName(t *testing.T) {
	p := &Provider{}
	if p.Name() != "google" {
		t.Errorf("Name: got %q, want %q", p.Name(), "google")
	}
}
