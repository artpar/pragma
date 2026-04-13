package anthropic

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
)

// buildWireParams converts internal RequestParams to Anthropic MessageNewParams.
// The IDMapper is pre-populated with synthetic wire IDs for history tool calls.
func buildWireParams(params provider.RequestParams, mapper *IDMapper) (sdk.MessageNewParams, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	normalized := normalizeMessages(params.Messages)
	msgs, err := messagesToWire(normalized, mapper)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: sdk.MessageNewParams{}, err")
		return sdk.MessageNewParams{}, err
	}
	system := systemToWire(params.System)
	tools := toolsToWire(params.Tools)

	modelInfo, known := LookupModel(params.Model)

	maxTokens := int64(params.MaxTokens)
	if known && int(maxTokens) > modelInfo.UpperMaxOutput {
		observe.GlobalTrace("if: known && int(maxTokens) > modelInfo.UpperMaxOutput")
		maxTokens = int64(modelInfo.UpperMaxOutput)
	}

	p := sdk.MessageNewParams{
		Model:     sdk.Model(params.Model),
		MaxTokens: maxTokens,
		Messages:  msgs,
		System:    system,
		Tools:     tools,
	}

	if params.Thinking == nil || !params.Thinking.Enabled {
		observe.GlobalTrace("if: params.Thinking == nil || !params.Thinking.Enabled")
		if params.Temperature != nil {
			observe.GlobalTrace("if: params.Temperature != nil")
			p.Temperature = param.NewOpt(*params.Temperature)
		}
	}

	if params.Thinking != nil && params.Thinking.Enabled {
		observe.GlobalTrace("if: params.Thinking != nil && params.Thinking.Enabled")
		p.Thinking = thinkingToWire(params.Thinking, modelInfo, known)
	}
	observe.GlobalTrace("return: p, nil")

	return p, nil
}

// thinkingToWire translates internal ThinkingConfig to the SDK's union type.
func thinkingToWire(cfg *provider.ThinkingConfig, info ModelInfo, known bool) sdk.ThinkingConfigParamUnion {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if cfg == nil || !cfg.Enabled {
		observe.GlobalTrace("if: cfg == nil || !cfg.Enabled")
		disabled := sdk.NewThinkingConfigDisabledParam()
		observe.GlobalTrace("return: sdk.ThinkingConfigParamUnion{OfDisabled: &disabled}")
		return sdk.ThinkingConfigParamUnion{OfDisabled: &disabled}
	}

	if known && info.ThinkingType == "adaptive" {
		observe.GlobalTrace("if: known && info.ThinkingType == \"adaptive\"")
		observe.GlobalTrace("return: sdk.ThinkingConfigParamUnion{OfAdaptive: &sdk.ThinkingConfigAdaptiveParam{}}")
		return sdk.ThinkingConfigParamUnion{OfAdaptive: &sdk.ThinkingConfigAdaptiveParam{}}
	}

	budget := int64(cfg.BudgetTokens)
	if budget <= 0 {
		observe.GlobalTrace("if: budget <= 0")
		budget = 10000
	}
	if known && int(budget) > info.MaxThinking {
		observe.GlobalTrace("if: known && int(budget) > info.MaxThinking")
		budget = int64(info.MaxThinking)
	}
	observe.GlobalTrace("return: sdk.ThinkingConfigParamOfEnabled(budget)")
	return sdk.ThinkingConfigParamOfEnabled(budget)
}

// messagesToWire converts a slice of internal Messages to SDK MessageParams.
func messagesToWire(msgs []model.Message, mapper *IDMapper) ([]sdk.MessageParam, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	out := make([]sdk.MessageParam, 0, len(msgs))
	for _, m := range msgs {
		observe.GlobalTrace("range msgs")
		mp, err := messageToWire(m, mapper)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		out = append(out, mp)
	}
	observe.GlobalTrace("return: out, nil")
	return out, nil
}

// messageToWire converts a single internal Message to an SDK MessageParam.
func messageToWire(m model.Message, mapper *IDMapper) (sdk.MessageParam, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	role := sdk.MessageParamRoleUser
	if m.Role == model.RoleAssistant {
		observe.GlobalTrace("if: m.Role == model.RoleAssistant")
		role = sdk.MessageParamRoleAssistant
	}

	blocks := make([]sdk.ContentBlockParamUnion, 0, len(m.Content))
	for _, part := range m.Content {
		observe.GlobalTrace("range m.Content")
		var block sdk.ContentBlockParamUnion
		var err error
		if m.Role == model.RoleAssistant {
			observe.GlobalTrace("if: m.Role == model.RoleAssistant")
			block, err = contentPartToWireAssistant(part, mapper)
		} else {
			observe.GlobalTrace("else: m.Role == model.RoleAssistant")
			block, err = contentPartToWireUser(part, mapper)
		}
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: sdk.MessageParam{}, err")
			return sdk.MessageParam{}, err
		}
		blocks = append(blocks, block)
	}
	observe.GlobalTrace("return: sdk.MessageParam{\n\tRole:\t\trole,\n\tContent:\tblocks,\n}, nil")

	return sdk.MessageParam{
		Role:    role,
		Content: blocks,
	}, nil
}

// contentPartToWireUser converts an internal ContentPart to an SDK block
// suitable for a user message.
func contentPartToWireUser(part model.ContentPart, mapper *IDMapper) (sdk.ContentBlockParamUnion, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch p := part.(type) {
	case model.TextPart:
		observe.GlobalTrace("typecase: model.TextPart")
		return sdk.NewTextBlock(p.Text), nil
	case model.ImagePart:
		observe.GlobalTrace("typecase: model.ImagePart")
		encoded := base64.StdEncoding.EncodeToString(p.Data)
		return sdk.NewImageBlockBase64(p.MimeType, encoded), nil
	case model.DocumentPart:
		observe.GlobalTrace("typecase: model.DocumentPart")
		encoded := base64.StdEncoding.EncodeToString(p.Data)
		return sdk.NewDocumentBlock(sdk.Base64PDFSourceParam{Data: encoded}), nil
	case model.ToolResultPart:
		observe.GlobalTrace("typecase: model.ToolResultPart")
		wireID := mapper.ToWire(p.ToolCallID)
		if wireID == "" {
			wireID = syntheticWireID(p.ToolCallID)
			mapper.RegisterPair(p.ToolCallID, wireID)
		}
		return sdk.NewToolResultBlock(wireID, p.Content, p.IsError), nil
	default:
		observe.GlobalTrace("typedefault")
		return sdk.NewTextBlock(fmt.Sprintf("[unsupported content type in user message: %T]", part)), nil
	}
}

// contentPartToWireAssistant converts an internal ContentPart to an SDK block
// suitable for an assistant message.
func contentPartToWireAssistant(part model.ContentPart, mapper *IDMapper) (sdk.ContentBlockParamUnion, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch p := part.(type) {
	case model.TextPart:
		observe.GlobalTrace("typecase: model.TextPart")
		return sdk.NewTextBlock(p.Text), nil
	case model.ToolCallPart:
		observe.GlobalTrace("typecase: model.ToolCallPart")
		wireID := mapper.ToWire(p.ID)
		if wireID == "" {
			wireID = syntheticWireID(p.ID)
			mapper.RegisterPair(p.ID, wireID)
		}
		// Input is json.RawMessage — unmarshal to any for SDK
		var input any
		if len(p.Input) > 0 {
			if err := json.Unmarshal(p.Input, &input); err != nil {
				observe.GlobalTrace("if: err != nil")
				observe.GlobalTrace("return: sdk.ContentBlockParamUnion{}, fmt.Errorf(\"malformed tool input JSON for %s: %...")
				return sdk.ContentBlockParamUnion{}, fmt.Errorf("malformed tool input JSON for %s: %w", p.Name, err)
			}
		}
		if input == nil {
			input = map[string]any{}
		}
		return sdk.NewToolUseBlock(wireID, input, p.Name), nil
	case model.ThinkingPart:
		observe.GlobalTrace("typecase: model.ThinkingPart")
		if p.Redacted {
			observe.GlobalTrace("return: sdk.NewRedactedThinkingBlock(p.RedactedData), nil")
			return sdk.NewRedactedThinkingBlock(p.RedactedData), nil
		}
		return sdk.NewThinkingBlock(p.Signature, p.Text), nil
	default:
		observe.GlobalTrace("typedefault")
		return sdk.NewTextBlock(fmt.Sprintf("[unsupported content type in assistant message: %T]", part)), nil
	}
}

// systemToWire converts internal SystemPrompt to SDK TextBlockParam slice.
func systemToWire(sys model.SystemPrompt) []sdk.TextBlockParam {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(sys.Blocks) == 0 {
		observe.GlobalTrace("if: len(sys.Blocks) == 0")
		observe.GlobalTrace("return: nil")
		return nil
	}
	out := make([]sdk.TextBlockParam, len(sys.Blocks))

	lastCacheable := -1
	for i, block := range sys.Blocks {
		observe.GlobalTrace("range sys.Blocks")
		if block.Cacheable {
			observe.GlobalTrace("if: block.Cacheable")
			lastCacheable = i
		}
	}
	for i, block := range sys.Blocks {
		observe.GlobalTrace("range sys.Blocks")
		out[i] = sdk.TextBlockParam{
			Text: block.Text,
		}
		if i == lastCacheable {
			observe.GlobalTrace("if: i == lastCacheable")
			out[i].CacheControl = sdk.NewCacheControlEphemeralParam()
		}
	}
	observe.GlobalTrace("return: out")
	return out
}

// toolsToWire converts internal ToolDefs to SDK ToolUnionParam slice.
func toolsToWire(tools []model.ToolDef) []sdk.ToolUnionParam {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(tools) == 0 {
		observe.GlobalTrace("if: len(tools) == 0")
		observe.GlobalTrace("return: nil")
		return nil
	}
	out := make([]sdk.ToolUnionParam, len(tools))
	for i, td := range tools {
		observe.GlobalTrace("range tools")
		out[i] = toolDefToWire(td)
	}
	observe.GlobalTrace("return: out")
	return out
}

// toolDefToWire converts a single ToolDef to an SDK ToolUnionParam.
func toolDefToWire(td model.ToolDef) sdk.ToolUnionParam {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	schema := parseInputSchema(td.InputSchema)
	tp := sdk.ToolParam{
		Name:        td.Name,
		Description: param.NewOpt(td.Description),
		InputSchema: schema,
	}
	observe.GlobalTrace("return: sdk.ToolUnionParam{OfTool: &tp}")
	return sdk.ToolUnionParam{OfTool: &tp}
}

// parseInputSchema converts a json.RawMessage JSON Schema into the SDK's
// ToolInputSchemaParam, extracting properties/required and passing the rest
// through ExtraFields.
func parseInputSchema(raw json.RawMessage) sdk.ToolInputSchemaParam {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(raw) == 0 {
		observe.GlobalTrace("if: len(raw) == 0")
		observe.GlobalTrace("return: sdk.ToolInputSchemaParam{}")
		return sdk.ToolInputSchemaParam{}
	}

	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: sdk.ToolInputSchemaParam{}")
		return sdk.ToolInputSchemaParam{}
	}

	result := sdk.ToolInputSchemaParam{
		ExtraFields: make(map[string]any),
	}

	if props, ok := schema["properties"]; ok {
		observe.GlobalTrace("if: ok")
		result.Properties = props
	}
	if req, ok := schema["required"]; ok {
		observe.GlobalTrace("if: ok")
		if arr, ok := req.([]any); ok {
			observe.GlobalTrace("if: ok")
			strs := make([]string, 0, len(arr))
			for _, v := range arr {
				observe.GlobalTrace("range arr")
				if s, ok := v.(string); ok {
					observe.GlobalTrace("if: ok")
					strs = append(strs, s)
				}
			}
			result.Required = strs
		}
	}

	for k, v := range schema {
		observe.GlobalTrace("range schema")
		if k != "type" && k != "properties" && k != "required" {
			observe.GlobalTrace("if: k != \"type\" && k != \"properties\" && k != \"required\"")
			result.ExtraFields[k] = v
		}
	}
	observe.GlobalTrace("return: result")

	return result
}

// prePopulateMapper walks all messages and registers synthetic wire IDs
// for any ToolCallPart found, ensuring history tool calls have consistent
// wire IDs when sent to the API.
func prePopulateMapper(msgs []model.Message, mapper *IDMapper) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, m := range msgs {
		observe.GlobalTrace("range msgs")
		for _, part := range m.Content {
			observe.GlobalTrace("range m.Content")
			if tc, ok := part.(model.ToolCallPart); ok {
				observe.GlobalTrace("if: ok")
				if mapper.ToWire(tc.ID) == "" {
					observe.GlobalTrace("if: mapper.ToWire(tc.ID) == \"\"")
					mapper.RegisterPair(tc.ID, syntheticWireID(tc.ID))
				}
			}
		}
	}
}
