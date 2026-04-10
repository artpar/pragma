package anthropic

import (
	"encoding/json"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/provider"
)

func TestContentPartToWireUserText(t *testing.T) {
	mapper := NewIDMapper()
	block, err := contentPartToWireUser(model.TextPart{Text: "hello"}, mapper)
	if err != nil {
		t.Fatal(err)
	}
	if block.OfText == nil {
		t.Fatal("expected OfText")
	}
	if block.OfText.Text != "hello" {
		t.Errorf("text: got %q", block.OfText.Text)
	}
}

func TestContentPartToWireUserImage(t *testing.T) {
	mapper := NewIDMapper()
	block, err := contentPartToWireUser(model.ImagePart{
		MimeType: "image/png",
		Data:     []byte{0x89, 0x50, 0x4e, 0x47},
	}, mapper)
	if err != nil {
		t.Fatal(err)
	}
	if block.OfImage == nil {
		t.Fatal("expected OfImage")
	}
}

func TestContentPartToWireUserToolResult(t *testing.T) {
	mapper := NewIDMapper()
	mapper.RegisterPair("internal-1", "toolu_abc")

	block, err := contentPartToWireUser(model.ToolResultPart{
		ToolCallID: "internal-1",
		Content:    "output text",
		IsError:    false,
	}, mapper)
	if err != nil {
		t.Fatal(err)
	}
	if block.OfToolResult == nil {
		t.Fatal("expected OfToolResult")
	}
	if block.OfToolResult.ToolUseID != "toolu_abc" {
		t.Errorf("ToolUseID: got %q, want %q", block.OfToolResult.ToolUseID, "toolu_abc")
	}
}

func TestContentPartToWireUserToolResultSyntheticID(t *testing.T) {
	mapper := NewIDMapper()
	block, err := contentPartToWireUser(model.ToolResultPart{
		ToolCallID: "unknown-id",
		Content:    "ok",
	}, mapper)
	if err != nil {
		t.Fatal(err)
	}
	if block.OfToolResult == nil {
		t.Fatal("expected OfToolResult")
	}
	if block.OfToolResult.ToolUseID == "" {
		t.Error("expected non-empty ToolUseID")
	}
	if mapper.ToWire("unknown-id") == "" {
		t.Error("expected synthetic ID to be registered in mapper")
	}
}

func TestContentPartToWireAssistantText(t *testing.T) {
	mapper := NewIDMapper()
	block, err := contentPartToWireAssistant(model.TextPart{Text: "response"}, mapper)
	if err != nil {
		t.Fatal(err)
	}
	if block.OfText == nil {
		t.Fatal("expected OfText")
	}
}

func TestContentPartToWireAssistantToolCall(t *testing.T) {
	mapper := NewIDMapper()
	mapper.RegisterPair("tc-1", "toolu_xyz")

	block, err := contentPartToWireAssistant(model.ToolCallPart{
		ID:    "tc-1",
		Name:  "Bash",
		Input: json.RawMessage(`{"cmd":"ls"}`),
	}, mapper)
	if err != nil {
		t.Fatal(err)
	}
	if block.OfToolUse == nil {
		t.Fatal("expected OfToolUse")
	}
	if block.OfToolUse.ID != "toolu_xyz" {
		t.Errorf("ID: got %q, want %q", block.OfToolUse.ID, "toolu_xyz")
	}
	if block.OfToolUse.Name != "Bash" {
		t.Errorf("Name: got %q", block.OfToolUse.Name)
	}
}

func TestContentPartToWireAssistantThinking(t *testing.T) {
	mapper := NewIDMapper()
	block, err := contentPartToWireAssistant(model.ThinkingPart{
		Text:      "reasoning trace",
		Signature: "sig_abc123",
	}, mapper)
	if err != nil {
		t.Fatal(err)
	}
	if block.OfThinking == nil {
		t.Fatal("expected OfThinking")
	}
	if block.OfThinking.Thinking != "reasoning trace" {
		t.Errorf("Thinking: got %q", block.OfThinking.Thinking)
	}
	if block.OfThinking.Signature != "sig_abc123" {
		t.Errorf("Signature: got %q", block.OfThinking.Signature)
	}
}

func TestContentPartToWireAssistantMalformedInput(t *testing.T) {
	mapper := NewIDMapper()
	_, err := contentPartToWireAssistant(model.ToolCallPart{
		ID:    "tc-bad",
		Name:  "Bash",
		Input: json.RawMessage(`{broken json`),
	}, mapper)
	if err == nil {
		t.Error("expected error for malformed tool input JSON")
	}
}

func TestSystemToWire(t *testing.T) {
	sys := model.SystemPrompt{
		Blocks: []model.SystemBlock{
			{Text: "You are a helpful assistant.", Cacheable: true},
			{Text: "Be concise.", Cacheable: false},
		},
	}
	blocks := systemToWire(sys)
	if len(blocks) != 2 {
		t.Fatalf("length: got %d, want 2", len(blocks))
	}
	if blocks[0].Text != "You are a helpful assistant." {
		t.Errorf("block[0] text: got %q", blocks[0].Text)
	}
	if string(blocks[0].CacheControl.Type) == "" {
		t.Error("block[0] should have CacheControl set")
	}
	if string(blocks[1].CacheControl.Type) != "" {
		t.Error("block[1] should not have CacheControl set")
	}
}

func TestSystemToWireEmpty(t *testing.T) {
	blocks := systemToWire(model.SystemPrompt{})
	if blocks != nil {
		t.Error("expected nil for empty system prompt")
	}
}

func TestToolDefToWire(t *testing.T) {
	td := model.ToolDef{
		Name:        "Bash",
		Description: "Run a shell command",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"cmd":{"type":"string"}},"required":["cmd"]}`),
	}
	tool := toolDefToWire(td)
	if tool.OfTool == nil {
		t.Fatal("expected OfTool")
	}
	if tool.OfTool.Name != "Bash" {
		t.Errorf("Name: got %q", tool.OfTool.Name)
	}
	if tool.OfTool.InputSchema.Required == nil {
		t.Fatal("expected Required")
	}
	if len(tool.OfTool.InputSchema.Required) != 1 || tool.OfTool.InputSchema.Required[0] != "cmd" {
		t.Errorf("Required: got %v", tool.OfTool.InputSchema.Required)
	}
}

func TestBuildWireParamsTemperatureOmittedWhenThinking(t *testing.T) {
	temp := 0.5
	params := provider.RequestParams{
		Model:       "claude-sonnet-4-20250514",
		MaxTokens:   8192,
		Messages:    []model.Message{{ID: "m1", Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "hi"}}}},
		System:      model.SystemPrompt{},
		Temperature: &temp,
		Thinking:    &provider.ThinkingConfig{Enabled: true, BudgetTokens: 5000},
	}
	mapper := NewIDMapper()
	wire, err := buildWireParams(params, mapper)
	if err != nil {
		t.Fatal(err)
	}
	if wire.Temperature.Valid() {
		t.Error("temperature should be omitted when thinking is enabled")
	}
}

func TestBuildWireParamsTemperatureSetWhenNoThinking(t *testing.T) {
	temp := 0.7
	params := provider.RequestParams{
		Model:       "claude-sonnet-4-20250514",
		MaxTokens:   8192,
		Messages:    []model.Message{{ID: "m1", Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "hi"}}}},
		Temperature: &temp,
	}
	mapper := NewIDMapper()
	wire, err := buildWireParams(params, mapper)
	if err != nil {
		t.Fatal(err)
	}
	if !wire.Temperature.Valid() {
		t.Error("temperature should be set when thinking is disabled")
	}
}

func TestBuildWireParamsMaxTokensCapped(t *testing.T) {
	params := provider.RequestParams{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 100000,
		Messages:  []model.Message{{ID: "m1", Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "hi"}}}},
	}
	mapper := NewIDMapper()
	wire, err := buildWireParams(params, mapper)
	if err != nil {
		t.Fatal(err)
	}
	if wire.MaxTokens > 64000 {
		t.Errorf("MaxTokens should be capped at 64000, got %d", wire.MaxTokens)
	}
}

func TestBuildWireParamsUnknownModelPassesThrough(t *testing.T) {
	params := provider.RequestParams{
		Model:     "unknown-model-v1",
		MaxTokens: 50000,
		Messages:  []model.Message{{ID: "m1", Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "hi"}}}},
	}
	mapper := NewIDMapper()
	wire, err := buildWireParams(params, mapper)
	if err != nil {
		t.Fatal(err)
	}
	if wire.MaxTokens != 50000 {
		t.Errorf("MaxTokens should pass through for unknown model, got %d", wire.MaxTokens)
	}
}

func TestThinkingToWireAdaptive(t *testing.T) {
	info := ModelInfo{ThinkingType: "adaptive", MaxThinking: 127999}
	cfg := &provider.ThinkingConfig{Enabled: true, BudgetTokens: 10000}
	result := thinkingToWire(cfg, info, true)
	if result.OfAdaptive == nil {
		t.Fatal("expected adaptive thinking config")
	}
}

func TestThinkingToWireEnabled(t *testing.T) {
	info := ModelInfo{ThinkingType: "enabled", MaxThinking: 63999}
	cfg := &provider.ThinkingConfig{Enabled: true, BudgetTokens: 10000}
	result := thinkingToWire(cfg, info, true)
	if result.OfEnabled == nil {
		t.Fatal("expected enabled thinking config")
	}
	if result.OfEnabled.BudgetTokens != 10000 {
		t.Errorf("BudgetTokens: got %d, want 10000", result.OfEnabled.BudgetTokens)
	}
}

func TestThinkingToWireBudgetCapped(t *testing.T) {
	info := ModelInfo{ThinkingType: "enabled", MaxThinking: 5000}
	cfg := &provider.ThinkingConfig{Enabled: true, BudgetTokens: 100000}
	result := thinkingToWire(cfg, info, true)
	if result.OfEnabled == nil {
		t.Fatal("expected enabled thinking config")
	}
	if result.OfEnabled.BudgetTokens != 5000 {
		t.Errorf("BudgetTokens should be capped at 5000, got %d", result.OfEnabled.BudgetTokens)
	}
}

func TestPrePopulateMapper(t *testing.T) {
	msgs := []model.Message{
		{
			ID:   "m1",
			Role: model.RoleAssistant,
			Content: []model.ContentPart{
				model.ToolCallPart{ID: "tc-a", Name: "X", Input: json.RawMessage(`{}`)},
				model.ToolCallPart{ID: "tc-b", Name: "Y", Input: json.RawMessage(`{}`)},
			},
		},
	}
	mapper := NewIDMapper()
	prePopulateMapper(msgs, mapper)

	if mapper.ToWire("tc-a") == "" {
		t.Error("tc-a should have a wire ID")
	}
	if mapper.ToWire("tc-b") == "" {
		t.Error("tc-b should have a wire ID")
	}
}

func TestMessageToWireUserRole(t *testing.T) {
	m := model.Message{
		ID:      "m1",
		Role:    model.RoleUser,
		Content: []model.ContentPart{model.TextPart{Text: "hello"}},
	}
	mapper := NewIDMapper()
	wire, err := messageToWire(m, mapper)
	if err != nil {
		t.Fatal(err)
	}
	if wire.Role != sdk.MessageParamRoleUser {
		t.Errorf("role: got %q, want user", wire.Role)
	}
}

func TestMessageToWireAssistantRole(t *testing.T) {
	m := model.Message{
		ID:      "m1",
		Role:    model.RoleAssistant,
		Content: []model.ContentPart{model.TextPart{Text: "hi"}},
	}
	mapper := NewIDMapper()
	wire, err := messageToWire(m, mapper)
	if err != nil {
		t.Fatal(err)
	}
	if wire.Role != sdk.MessageParamRoleAssistant {
		t.Errorf("role: got %q, want assistant", wire.Role)
	}
}

func TestParseInputSchemaWithExtraFields(t *testing.T) {
	raw := json.RawMessage(`{"type":"object","properties":{"x":{"type":"string"}},"required":["x"],"additionalProperties":false}`)
	schema := parseInputSchema(raw)
	if schema.Properties == nil {
		t.Error("expected properties")
	}
	if len(schema.Required) != 1 {
		t.Errorf("required: got %v", schema.Required)
	}
	if schema.ExtraFields["additionalProperties"] != false {
		t.Errorf("additionalProperties: got %v", schema.ExtraFields["additionalProperties"])
	}
}

func TestSyntheticMessageIDsAreUnique(t *testing.T) {
	msgs := []model.Message{
		{ID: "m1", Role: model.RoleAssistant, Content: []model.ContentPart{
			model.ToolCallPart{ID: "tc-1", Name: "X", Input: json.RawMessage(`{}`)},
		}},
		// No user message after — normalizer will inject synthetic
		{ID: "m2", Role: model.RoleAssistant, Content: []model.ContentPart{
			model.ToolCallPart{ID: "tc-2", Name: "Y", Input: json.RawMessage(`{}`)},
		}},
	}
	result := normalizeMessages(msgs)

	// Collect all message IDs
	ids := make(map[string]int)
	for _, m := range result {
		ids[m.ID]++
	}
	for id, count := range ids {
		if count > 1 {
			t.Errorf("duplicate message ID %q appears %d times", id, count)
		}
	}
}
