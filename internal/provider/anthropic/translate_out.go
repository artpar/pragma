package anthropic

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/provider"
)

// buildWireParams converts internal RequestParams to Anthropic MessageNewParams.
// The IDMapper is pre-populated with synthetic wire IDs for history tool calls.
func buildWireParams(params provider.RequestParams, mapper *IDMapper) (sdk.MessageNewParams, error) {
	normalized := normalizeMessages(params.Messages)
	msgs, err := messagesToWire(normalized, mapper)
	if err != nil {
		return sdk.MessageNewParams{}, err
	}
	system := systemToWire(params.System)
	tools := toolsToWire(params.Tools)

	modelInfo, known := LookupModel(params.Model)

	maxTokens := int64(params.MaxTokens)
	if known && int(maxTokens) > modelInfo.UpperMaxOutput {
		maxTokens = int64(modelInfo.UpperMaxOutput)
	}

	p := sdk.MessageNewParams{
		Model:     sdk.Model(params.Model),
		MaxTokens: maxTokens,
		Messages:  msgs,
		System:    system,
		Tools:     tools,
	}

	// Temperature: omit when thinking is enabled (API requires default 1.0)
	if params.Thinking == nil || !params.Thinking.Enabled {
		if params.Temperature != nil {
			p.Temperature = param.NewOpt(*params.Temperature)
		}
	}

	// Thinking config
	if params.Thinking != nil && params.Thinking.Enabled {
		p.Thinking = thinkingToWire(params.Thinking, modelInfo, known)
	}

	return p, nil
}

// thinkingToWire translates internal ThinkingConfig to the SDK's union type.
func thinkingToWire(cfg *provider.ThinkingConfig, info ModelInfo, known bool) sdk.ThinkingConfigParamUnion {
	if cfg == nil || !cfg.Enabled {
		disabled := sdk.NewThinkingConfigDisabledParam()
		return sdk.ThinkingConfigParamUnion{OfDisabled: &disabled}
	}

	if known && info.ThinkingType == "adaptive" {
		return sdk.ThinkingConfigParamUnion{OfAdaptive: &sdk.ThinkingConfigAdaptiveParam{}}
	}

	budget := int64(cfg.BudgetTokens)
	if budget <= 0 {
		budget = 10000 // sensible default
	}
	if known && int(budget) > info.MaxThinking {
		budget = int64(info.MaxThinking)
	}
	return sdk.ThinkingConfigParamOfEnabled(budget)
}

// messagesToWire converts a slice of internal Messages to SDK MessageParams.
func messagesToWire(msgs []model.Message, mapper *IDMapper) ([]sdk.MessageParam, error) {
	out := make([]sdk.MessageParam, 0, len(msgs))
	for _, m := range msgs {
		mp, err := messageToWire(m, mapper)
		if err != nil {
			return nil, err
		}
		out = append(out, mp)
	}
	return out, nil
}

// messageToWire converts a single internal Message to an SDK MessageParam.
func messageToWire(m model.Message, mapper *IDMapper) (sdk.MessageParam, error) {
	role := sdk.MessageParamRoleUser
	if m.Role == model.RoleAssistant {
		role = sdk.MessageParamRoleAssistant
	}

	blocks := make([]sdk.ContentBlockParamUnion, 0, len(m.Content))
	for _, part := range m.Content {
		var block sdk.ContentBlockParamUnion
		var err error
		if m.Role == model.RoleAssistant {
			block, err = contentPartToWireAssistant(part, mapper)
		} else {
			block, err = contentPartToWireUser(part, mapper)
		}
		if err != nil {
			return sdk.MessageParam{}, err
		}
		blocks = append(blocks, block)
	}

	return sdk.MessageParam{
		Role:    role,
		Content: blocks,
	}, nil
}

// contentPartToWireUser converts an internal ContentPart to an SDK block
// suitable for a user message.
func contentPartToWireUser(part model.ContentPart, mapper *IDMapper) (sdk.ContentBlockParamUnion, error) {
	switch p := part.(type) {
	case model.TextPart:
		return sdk.NewTextBlock(p.Text), nil
	case model.ImagePart:
		encoded := base64.StdEncoding.EncodeToString(p.Data)
		return sdk.NewImageBlockBase64(p.MimeType, encoded), nil
	case model.ToolResultPart:
		wireID := mapper.ToWire(p.ToolCallID)
		if wireID == "" {
			wireID = syntheticWireID(p.ToolCallID)
			mapper.RegisterPair(p.ToolCallID, wireID)
		}
		return sdk.NewToolResultBlock(wireID, p.Content, p.IsError), nil
	default:
		return sdk.NewTextBlock(fmt.Sprintf("[unsupported content type in user message: %T]", part)), nil
	}
}

// contentPartToWireAssistant converts an internal ContentPart to an SDK block
// suitable for an assistant message.
func contentPartToWireAssistant(part model.ContentPart, mapper *IDMapper) (sdk.ContentBlockParamUnion, error) {
	switch p := part.(type) {
	case model.TextPart:
		return sdk.NewTextBlock(p.Text), nil
	case model.ToolCallPart:
		wireID := mapper.ToWire(p.ID)
		if wireID == "" {
			wireID = syntheticWireID(p.ID)
			mapper.RegisterPair(p.ID, wireID)
		}
		// Input is json.RawMessage — unmarshal to any for SDK
		var input any
		if len(p.Input) > 0 {
			if err := json.Unmarshal(p.Input, &input); err != nil {
				return sdk.ContentBlockParamUnion{}, fmt.Errorf("malformed tool input JSON for %s: %w", p.Name, err)
			}
		}
		if input == nil {
			input = map[string]any{}
		}
		return sdk.NewToolUseBlock(wireID, input, p.Name), nil
	case model.ThinkingPart:
		if p.Redacted {
			return sdk.NewRedactedThinkingBlock(p.RedactedData), nil
		}
		return sdk.NewThinkingBlock(p.Signature, p.Text), nil
	default:
		return sdk.NewTextBlock(fmt.Sprintf("[unsupported content type in assistant message: %T]", part)), nil
	}
}

// systemToWire converts internal SystemPrompt to SDK TextBlockParam slice.
func systemToWire(sys model.SystemPrompt) []sdk.TextBlockParam {
	if len(sys.Blocks) == 0 {
		return nil
	}
	out := make([]sdk.TextBlockParam, len(sys.Blocks))
	for i, block := range sys.Blocks {
		out[i] = sdk.TextBlockParam{
			Text: block.Text,
		}
		if block.Cacheable {
			out[i].CacheControl = sdk.NewCacheControlEphemeralParam()
		}
	}
	return out
}

// toolsToWire converts internal ToolDefs to SDK ToolUnionParam slice.
func toolsToWire(tools []model.ToolDef) []sdk.ToolUnionParam {
	if len(tools) == 0 {
		return nil
	}
	out := make([]sdk.ToolUnionParam, len(tools))
	for i, td := range tools {
		out[i] = toolDefToWire(td)
	}
	return out
}

// toolDefToWire converts a single ToolDef to an SDK ToolUnionParam.
func toolDefToWire(td model.ToolDef) sdk.ToolUnionParam {
	schema := parseInputSchema(td.InputSchema)
	tp := sdk.ToolParam{
		Name:        td.Name,
		Description: param.NewOpt(td.Description),
		InputSchema: schema,
	}
	return sdk.ToolUnionParam{OfTool: &tp}
}

// parseInputSchema converts a json.RawMessage JSON Schema into the SDK's
// ToolInputSchemaParam, extracting properties/required and passing the rest
// through ExtraFields.
func parseInputSchema(raw json.RawMessage) sdk.ToolInputSchemaParam {
	if len(raw) == 0 {
		return sdk.ToolInputSchemaParam{}
	}

	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return sdk.ToolInputSchemaParam{}
	}

	result := sdk.ToolInputSchemaParam{
		ExtraFields: make(map[string]any),
	}

	if props, ok := schema["properties"]; ok {
		result.Properties = props
	}
	if req, ok := schema["required"]; ok {
		if arr, ok := req.([]any); ok {
			strs := make([]string, 0, len(arr))
			for _, v := range arr {
				if s, ok := v.(string); ok {
					strs = append(strs, s)
				}
			}
			result.Required = strs
		}
	}

	// Pass through additional schema fields
	for k, v := range schema {
		if k != "type" && k != "properties" && k != "required" {
			result.ExtraFields[k] = v
		}
	}

	return result
}

// prePopulateMapper walks all messages and registers synthetic wire IDs
// for any ToolCallPart found, ensuring history tool calls have consistent
// wire IDs when sent to the API.
func prePopulateMapper(msgs []model.Message, mapper *IDMapper) {
	for _, m := range msgs {
		for _, part := range m.Content {
			if tc, ok := part.(model.ToolCallPart); ok {
				if mapper.ToWire(tc.ID) == "" {
					mapper.RegisterPair(tc.ID, syntheticWireID(tc.ID))
				}
			}
		}
	}
}
