package google

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
)

// buildWireRequest converts internal RequestParams to a Google Gemini wire request.
func buildWireRequest(params provider.RequestParams, mapper *IDMapper, bus *observe.EventBus) wireRequest {
	normalized := normalizeMessages(params.Messages)

	modelInfo, _ := LookupModel(params.Model)

	sysInstruction, contents := messagesToWire(params.System, normalized, mapper, bus)

	req := wireRequest{
		Contents:          contents,
		SystemInstruction: sysInstruction,
	}

	genConfig := &wireGenConfig{}
	hasGenConfig := false

	maxTokens := params.MaxTokens
	if modelInfo.MaxOutput > 0 && maxTokens > modelInfo.MaxOutput {
		maxTokens = modelInfo.MaxOutput
	}
	if maxTokens > 0 {
		genConfig.MaxOutputTokens = maxTokens
		hasGenConfig = true
	}

	if params.Temperature != nil {
		genConfig.Temperature = params.Temperature
		hasGenConfig = true
	}

	if params.Thinking != nil && params.Thinking.Enabled && modelInfo.SupportsThinking {
		tc := &wireThinkConfig{IncludeThoughts: true}
		if params.Thinking.BudgetTokens > 0 {
			tc.ThinkingBudget = params.Thinking.BudgetTokens
		}
		genConfig.ThinkingConfig = tc
		hasGenConfig = true
	}

	if hasGenConfig {
		req.GenerationConfig = genConfig
	}

	if len(params.Tools) > 0 {
		req.Tools = toolsToWire(params.Tools)
		req.ToolConfig = &wireToolConfig{
			FunctionCallingConfig: &wireFCConfig{Mode: "AUTO"},
		}
	}

	return req
}

// messagesToWire converts system prompt + internal messages to Google wire format.
func messagesToWire(system model.SystemPrompt, msgs []model.Message, mapper *IDMapper, bus *observe.EventBus) (*wireContent, []wireContent) {
	var sysInstruction *wireContent

	if len(system.Blocks) > 0 {
		var sb strings.Builder
		for i, block := range system.Blocks {
			if i > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(block.Text)
		}
		if sb.Len() > 0 {
			sysInstruction = &wireContent{
				Parts: []wirePart{{Text: sb.String()}},
			}
		}
	}

	toolNameMap := buildToolNameMap(msgs)
	var contents []wireContent

	for _, m := range msgs {
		switch m.Role {
		case model.RoleAssistant:
			contents = append(contents, assistantToWire(m, mapper))
		case model.RoleUser:
			contents = append(contents, userToWire(m, mapper, toolNameMap, bus)...)
		}
	}

	return sysInstruction, contents
}

// buildToolNameMap scans all assistant messages and builds toolCallID -> toolName.
func buildToolNameMap(msgs []model.Message) map[string]string {
	names := make(map[string]string)
	for _, m := range msgs {
		if m.Role != model.RoleAssistant {
			continue
		}
		for _, p := range m.Content {
			if tc, ok := p.(model.ToolCallPart); ok {
				names[tc.ID] = tc.Name
			}
		}
	}
	return names
}

// assistantToWire converts an assistant message to a Google "model" wire content.
func assistantToWire(m model.Message, mapper *IDMapper) wireContent {
	content := wireContent{Role: "model"}

	for _, p := range m.Content {
		switch part := p.(type) {
		case model.TextPart:
			content.Parts = append(content.Parts, wirePart{Text: part.Text})
		case model.ToolCallPart:
			// Preserve the function call id for Gemini 3 round-trip.
			// The mapper holds internalID → wireID (which is the Google-provided id).
			wireID := mapper.ToWire(part.ID)
			if wireID == "" {
				wireID = syntheticWireID(part.ID, part.Name)
				mapper.RegisterPair(part.ID, wireID)
			}
			args := part.Input
			if len(args) == 0 || string(args) == "null" {
				args = json.RawMessage("{}")
			}
			content.Parts = append(content.Parts, wirePart{
				FunctionCall: &wireFunctionCall{
					Name: part.Name,
					Args: args,
					Id:   wireID,
				},
			})
		case model.ThinkingPart:
			// Gemini 3: thoughtSignature must round-trip for function calling.
			// Emit thinking parts back with their signature preserved.
			wp := wirePart{Text: part.Text}
			t := true
			wp.Thought = &t
			if part.Signature != "" {
				wp.ThoughtSignature = part.Signature
			}
			content.Parts = append(content.Parts, wp)
		case model.ImagePart, model.DocumentPart, model.ToolResultPart:
			// These don't appear in assistant messages
		}
	}

	return content
}

// userToWire converts a user message to one or more Google wire contents.
// Tool results become functionResponse parts in a user-role content.
func userToWire(m model.Message, mapper *IDMapper, toolNameMap map[string]string, bus *observe.EventBus) []wireContent {
	var functionResponses []wirePart
	var otherParts []wirePart

	for _, p := range m.Content {
		switch part := p.(type) {
		case model.ToolResultPart:
			name := toolNameMap[part.ToolCallID]
			resp := map[string]string{"output": part.Content}
			if part.IsError {
				resp["error"] = part.Content
				delete(resp, "output")
			}
			respJSON, _ := json.Marshal(resp)

			// Gemini 3: FunctionResponse must include the matching id from the FunctionCall.
			fcID := mapper.ToWire(part.ToolCallID)

			functionResponses = append(functionResponses, wirePart{
				FunctionResponse: &wireFunctionResponse{
					Name:     name,
					Response: respJSON,
					Id:       fcID,
				},
			})
		case model.TextPart:
			otherParts = append(otherParts, wirePart{Text: part.Text})
		case model.ImagePart:
			otherParts = append(otherParts, wirePart{
				InlineData: &wireInlineData{
					MimeType: part.MimeType,
					Data:     base64.StdEncoding.EncodeToString(part.Data),
				},
			})
		case model.DocumentPart:
			otherParts = append(otherParts, wirePart{
				InlineData: &wireInlineData{
					MimeType: part.MimeType,
					Data:     base64.StdEncoding.EncodeToString(part.Data),
				},
			})
		case model.ThinkingPart:
			// Drop thinking parts from user messages
		case model.ToolCallPart:
			// Tool calls don't appear in user messages
		}
	}

	var out []wireContent

	if len(functionResponses) > 0 {
		out = append(out, wireContent{
			Role:  "user",
			Parts: functionResponses,
		})
	}

	if len(otherParts) > 0 {
		out = append(out, wireContent{
			Role:  "user",
			Parts: otherParts,
		})
	}

	return out
}

// toolsToWire converts internal tool definitions to Google wire format.
func toolsToWire(tools []model.ToolDef) []wireTool {
	decls := make([]wireFunctionDecl, len(tools))
	for i, t := range tools {
		decls[i] = wireFunctionDecl{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.InputSchema,
		}
	}
	return []wireTool{{FunctionDeclarations: decls}}
}

// prePopulateMapper walks history messages and registers synthetic wire IDs
// for all ToolCallParts that don't already have mappings.
func prePopulateMapper(msgs []model.Message, mapper *IDMapper) {
	for _, m := range msgs {
		if m.Role != model.RoleAssistant {
			continue
		}
		for _, p := range m.Content {
			if tc, ok := p.(model.ToolCallPart); ok {
				if !mapper.HasInternal(tc.ID) {
					wireID := syntheticWireID(tc.ID, tc.Name)
					mapper.RegisterPair(tc.ID, wireID)
				}
			}
		}
	}
}
