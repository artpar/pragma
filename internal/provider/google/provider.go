// Package google implements provider.Provider for Google Gemini via google.golang.org/genai.
// Uses the genai SDK directly (not any-llm-go wrapper) for full control over ThinkingConfig,
// which is required to work around gemini-2.5-flash's empty response bug with many tools.
package google

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
	"github.com/artpar/gogent/internal/provider/shared"
	"google.golang.org/genai"
)

// Provider implements provider.Provider for Google Gemini.
type Provider struct {
	client     *genai.Client
	bus        *observe.EventBus
	maxRetries int
}

// Option configures the Provider.
type Option func(*Provider)

// WithBaseURL is a no-op placeholder for API compatibility.
func WithBaseURL(_ string) Option {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: func(_ *Provider) {}")
	return func(_ *Provider) {}
}

// New creates a Google Gemini provider.
func New(apiKey string, bus *observe.EventBus, opts ...Option) (*Provider, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"google: create client: %w\", err)")
		return nil, fmt.Errorf("google: create client: %w", err)
	}
	p := &Provider{client: client, bus: bus, maxRetries: 10}
	for _, opt := range opts {
		observe.GlobalTrace("range opts")
		opt(p)
	}
	observe.GlobalTrace("return: p, nil")
	return p, nil
}

func (p *Provider) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"google\"")
	return "google"
}

func (p *Provider) SupportsFeature(feature provider.Feature) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch feature {
	case provider.FeatureToolUse, provider.FeatureStreaming,
		provider.FeatureImages, provider.FeatureThinking:
		observe.GlobalTrace("case: provider.FeatureToolUse, provider.FeatureStreaming, provider.FeatureImages, p...")
		return true
	}
	observe.GlobalTrace("return: false")
	return false
}

func (p *Provider) Pricing(modelID string) (model.Pricing, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if info, ok := LookupModel(modelID); ok {
		observe.GlobalTrace("if: ok")
		observe.GlobalTrace("return: info.Pricing, true")
		return info.Pricing, true
	}
	observe.GlobalTrace("return: model.Pricing{}, false")
	return model.Pricing{}, false
}

func (p *Provider) ContextWindow(modelID string) (int, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if info, ok := LookupModel(modelID); ok {
		observe.GlobalTrace("if: ok")
		observe.GlobalTrace("return: info.MaxContext, true")
		return info.MaxContext, true
	}
	observe.GlobalTrace("return: 1_048_576, false")
	return 1_048_576, false
}

// googleClassify classifies errors using HTTP status code string matching.
var googleClassify = shared.ClassifyByStatusCodes([]string{"429", "500", "502", "503", "504", "RESOURCE_EXHAUSTED"})

// Complete sends a non-streaming request.
func (p *Provider) Complete(ctx context.Context, params provider.RequestParams) (model.Response, error) {
	observe.TraceCtx(ctx, "google", "Provider.Complete", "enter")
	defer observe.TraceCtx(ctx, "google", "Provider.Complete", "exit")
	traceID := observe.NewTraceID()
	spanID := observe.NewSpanID()
	p.emitStart(traceID, spanID, params)
	start := time.Now()

	contents, cfg := p.buildRequest(params)

	var resp *genai.GenerateContentResponse
	err := shared.WithRetry(ctx, p.bus, p.maxRetries, traceID, spanID, googleClassify, func() error {
		var reqErr error
		resp, reqErr = p.client.Models.GenerateContent(ctx, params.Model, contents, cfg)
		return reqErr
	})
	if err != nil {
		observe.TraceCtx(ctx, "google", "Provider.Complete", "if: err != nil")
		observe.TraceCtx(ctx, "google", "Provider.Complete", "return: model.Response{}, err")
		return model.Response{}, err
	}

	result := responseFromGenai(resp, params.Model)
	p.bus.Emit(observe.APIRequestCompleted{
		EventHeader: observe.NewEventHeader("APIRequestCompleted", traceID, spanID, ""),
		StopReason:  result.StopReason, Usage: result.Usage,
		DurationMs: time.Since(start).Milliseconds(), Model: result.Model,
	})
	observe.TraceCtx(ctx, "google", "Provider.Complete", "return: result, nil")
	return result, nil
}

// Stream starts a streaming request.
func (p *Provider) Stream(ctx context.Context, params provider.RequestParams) (<-chan provider.StreamChunk, error) {
	observe.TraceCtx(ctx, "google", "Provider.Stream", "enter")
	defer observe.TraceCtx(ctx, "google", "Provider.Stream", "exit")
	traceID := observe.NewTraceID()
	spanID := observe.NewSpanID()
	p.emitStart(traceID, spanID, params)

	contents, cfg := p.buildRequest(params)
	ch := make(chan provider.StreamChunk, 32)

	go func() {
		defer close(ch)
		start := time.Now()
		var usage model.TokenUsage
		seenToolCalls := make(map[string]bool)

		for resp, err := range p.client.Models.GenerateContentStream(ctx, params.Model, contents, cfg) {
			observe.TraceCtx(ctx, "google", "Provider.Stream", "range p.client.Models.GenerateContentStream(ctx, params.Model, contents, cfg)")
			if err != nil {
				observe.TraceCtx(ctx, "google", "Provider.Stream", "if: err != nil")
				ch <- provider.StreamChunk{Error: err}
				p.bus.Emit(observe.APIRequestFailed{
					EventHeader:  observe.NewEventHeader("APIRequestFailed", traceID, spanID, ""),
					ErrorType:    "stream_error",
					ErrorMessage: err.Error(),
					Retryable:    false,
				})
				return
			}

			if resp.UsageMetadata != nil {
				observe.TraceCtx(ctx, "google", "Provider.Stream", "if: resp.UsageMetadata != nil")
				usage = usageFromGenai(resp.UsageMetadata)
			}

			for _, cand := range resp.Candidates {
				observe.TraceCtx(ctx, "google", "Provider.Stream", "range resp.Candidates")
				if cand.Content == nil {
					observe.TraceCtx(ctx, "google", "Provider.Stream", "if: cand.Content == nil")
					if p.bus != nil {
						reason := ""
						if cand.FinishReason != "" {
							reason = string(cand.FinishReason)
						}
						p.bus.Emit(observe.ErrorOccurred{
							EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
							Severity:     "warn",
							Component:    "google",
							ErrorType:    "nil_candidate_content",
							ErrorMessage: "candidate has nil Content, finish_reason=" + reason,
						})
					}
					continue
				}
				for _, part := range cand.Content.Parts {
					observe.TraceCtx(ctx, "google", "Provider.Stream", "range cand.Content.Parts")
					switch {
					case part.Text != "" && part.Thought:
						observe.TraceCtx(ctx, "google", "Provider.Stream", "case: part.Text != \"\" && part.Thought")
						ch <- provider.StreamChunk{ThinkingDelta: part.Text}
						if len(part.ThoughtSignature) > 0 {
							ch <- provider.StreamChunk{ThinkingSignatureDelta: string(part.ThoughtSignature)}
						}
					case part.Text != "":
						observe.TraceCtx(ctx, "google", "Provider.Stream", "case: part.Text != \"\"")
						ch <- provider.StreamChunk{TextDelta: part.Text}
					case part.FunctionCall != nil:
						observe.TraceCtx(ctx, "google", "Provider.Stream", "case: part.FunctionCall != nil")
						fc := part.FunctionCall
						id := fc.ID
						if id == "" {
							id = model.NewUUID()
						}
						seenToolCalls[id] = true
						ch <- provider.StreamChunk{
							ToolCallStart: &model.ToolCallPart{ID: id, Name: fc.Name},
						}
						if fc.Args != nil {
							argsJSON, _ := json.Marshal(fc.Args)
							ch <- provider.StreamChunk{
								ToolCallInputDelta: &provider.ToolCallDelta{
									ToolCallID: id, JSONDelta: string(argsJSON),
								},
							}
						}
					}
				}

				if cand.FinishReason != "" {
					observe.TraceCtx(ctx, "google", "Provider.Stream", "if: cand.FinishReason != \"\"")
					stopReason := stopReasonFromGenai(cand.FinishReason)
					if stopReason == model.StopEndTurn && len(seenToolCalls) > 0 {
						observe.TraceCtx(ctx, "google", "Provider.Stream", "if: stopReason == model.StopEndTurn && len(seenToolCalls) > 0")
						stopReason = model.StopToolUse
					}
					ch <- provider.StreamChunk{
						Done: &provider.StreamDone{
							StopReason: stopReason, Usage: usage, Model: params.Model,
						},
					}
					p.bus.Emit(observe.APIRequestCompleted{
						EventHeader: observe.NewEventHeader("APIRequestCompleted", traceID, spanID, ""),
						StopReason:  stopReason, Usage: usage,
						DurationMs: time.Since(start).Milliseconds(), Model: params.Model,
					})
				}
			}
		}
	}()
	observe.TraceCtx(ctx, "google", "Provider.Stream", "return: ch, nil")

	return ch, nil
}

// buildRequest converts gogent params to genai SDK types.
func (p *Provider) buildRequest(params provider.RequestParams) ([]*genai.Content, *genai.GenerateContentConfig) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	cfg := &genai.GenerateContentConfig{}

	if len(params.System.Blocks) > 0 {
		observe.GlobalTrace("if: len(params.System.Blocks) > 0")
		var sb strings.Builder
		for i, block := range params.System.Blocks {
			observe.GlobalTrace("range params.System.Blocks")
			if i > 0 {
				observe.GlobalTrace("if: i > 0")
				sb.WriteString("\n\n")
			}
			sb.WriteString(block.Text)
		}
		cfg.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: sb.String()}},
		}
	}

	if params.MaxTokens > 0 {
		observe.GlobalTrace("if: params.MaxTokens > 0")
		cfg.MaxOutputTokens = int32(params.MaxTokens)
	}

	if params.Temperature != nil {
		observe.GlobalTrace("if: params.Temperature != nil")
		t := float32(*params.Temperature)
		cfg.Temperature = &t
	}

	if len(params.Tools) > 0 {
		observe.GlobalTrace("if: len(params.Tools) > 0")
		cfg.Tools = toolsToGenai(params.Tools)
	}

	if params.Thinking != nil && params.Thinking.Enabled {
		observe.GlobalTrace("if: params.Thinking != nil && params.Thinking.Enabled")
		budget := int32(params.Thinking.BudgetTokens)
		cfg.ThinkingConfig = &genai.ThinkingConfig{
			IncludeThoughts: true,
			ThinkingBudget:  &budget,
		}
	} else if strings.Contains(params.Model, "flash") {
		observe.GlobalTrace("else-if: strings.Contains(params.Model, \"flash\")")

		zero := int32(0)
		cfg.ThinkingConfig = &genai.ThinkingConfig{
			ThinkingBudget: &zero,
		}
	}

	contents := messagesToGenai(params.Messages)
	observe.GlobalTrace("return: contents, cfg")

	return contents, cfg
}

// messagesToGenai converts internal messages to genai Content.
func messagesToGenai(msgs []model.Message) []*genai.Content {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	toolNames := make(map[string]string)
	for _, m := range msgs {
		observe.GlobalTrace("range msgs")
		if m.Role == model.RoleAssistant {
			observe.GlobalTrace("if: m.Role == model.RoleAssistant")
			for _, p := range m.Content {
				observe.GlobalTrace("range m.Content")
				if tc, ok := p.(model.ToolCallPart); ok {
					observe.GlobalTrace("if: ok")
					toolNames[tc.ID] = tc.Name
				}
			}
		}
	}

	var contents []*genai.Content
	for _, m := range msgs {
		observe.GlobalTrace("range msgs")
		role := "user"
		if m.Role == model.RoleAssistant {
			observe.GlobalTrace("if: m.Role == model.RoleAssistant")
			role = "model"
		}

		var parts []*genai.Part
		var toolResponses []*genai.Part

		for _, p := range m.Content {
			observe.GlobalTrace("range m.Content")
			switch part := p.(type) {
			case model.TextPart:
				observe.GlobalTrace("typecase: model.TextPart")
				if part.Text != "" {
					parts = append(parts, &genai.Part{Text: part.Text})
				}
			case model.ToolCallPart:
				observe.GlobalTrace("typecase: model.ToolCallPart")
				var args map[string]any
				if len(part.Input) > 0 {
					_ = json.Unmarshal(part.Input, &args)
				}
				parts = append(parts, genai.NewPartFromFunctionCall(part.Name, args))
			case model.ToolResultPart:
				observe.GlobalTrace("typecase: model.ToolResultPart")
				name := toolNames[part.ToolCallID]
				if name == "" {
					name = "function"
				}
				var resp map[string]any
				if err := json.Unmarshal([]byte(part.Content), &resp); err != nil {
					resp = map[string]any{"result": part.Content}
				}
				toolResponses = append(toolResponses, genai.NewPartFromFunctionResponse(name, resp))
			case model.ThinkingPart:
				observe.GlobalTrace("typecase: model.ThinkingPart")
				tp := &genai.Part{Text: part.Text, Thought: true}
				if part.Signature != "" {
					tp.ThoughtSignature = []byte(part.Signature)
				}
				parts = append(parts, tp)
			case model.ImagePart:
				observe.GlobalTrace("typecase: model.ImagePart")
				parts = append(parts, &genai.Part{
					InlineData: &genai.Blob{
						MIMEType: part.MimeType,
						Data:     part.Data,
					},
				})
			}
		}

		if len(toolResponses) > 0 {
			observe.GlobalTrace("if: len(toolResponses) > 0")
			contents = append(contents, &genai.Content{
				Role: "user", Parts: toolResponses,
			})
		}

		if len(parts) > 0 {
			observe.GlobalTrace("if: len(parts) > 0")
			contents = append(contents, &genai.Content{
				Role: role, Parts: parts,
			})
		}
	}
	observe.GlobalTrace("return: contents")
	return contents
}

// toolsToGenai converts gogent tool definitions to genai tools.
func toolsToGenai(tools []model.ToolDef) []*genai.Tool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var decls []*genai.FunctionDeclaration
	for _, t := range tools {
		observe.GlobalTrace("range tools")
		var schema *genai.Schema
		if len(t.InputSchema) > 0 {
			observe.GlobalTrace("if: len(t.InputSchema) > 0")
			sanitized := sanitizeSchema(t.InputSchema)
			schema = &genai.Schema{}
			_ = json.Unmarshal(sanitized, schema)
		}
		decls = append(decls, &genai.FunctionDeclaration{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  schema,
		})
	}
	observe.GlobalTrace("return: []*genai.Tool{{FunctionDeclarations: decls}}")
	return []*genai.Tool{{FunctionDeclarations: decls}}
}

// sanitizeSchema cleans a JSON Schema for Gemini compatibility.
// Gemini rejects fields like "default", "additionalProperties", "$schema",
// empty enum strings, etc.
func sanitizeSchema(raw json.RawMessage) json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var obj map[string]any
	if json.Unmarshal(raw, &obj) != nil {
		observe.GlobalTrace("if: json.Unmarshal(raw, &obj) != nil")
		observe.GlobalTrace("return: raw")
		return raw
	}
	sanitizeObj(obj)
	out, err := json.Marshal(obj)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: raw")
		return raw
	}
	observe.GlobalTrace("return: out")
	return out
}

var allowedFields = map[string]bool{
	"type": true, "nullable": true, "required": true,
	"format": true, "description": true, "properties": true,
	"items": true, "enum": true, "anyOf": true,
	"propertyOrdering": true,
}

func sanitizeObj(obj map[string]any) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for key := range obj {
		observe.GlobalTrace("range obj")
		if !allowedFields[key] {
			observe.GlobalTrace("if: !allowedFields[key]")
			delete(obj, key)
		}
	}
	if enum, ok := obj["enum"].([]any); ok {
		observe.GlobalTrace("if: ok")
		var cleaned []any
		for _, v := range enum {
			observe.GlobalTrace("range enum")
			if s, isStr := v.(string); isStr && s == "" {
				observe.GlobalTrace("if: isStr && s == \"\"")
				continue
			}
			cleaned = append(cleaned, v)
		}
		if len(cleaned) == 0 {
			observe.GlobalTrace("if: len(cleaned) == 0")
			delete(obj, "enum")
		} else {
			observe.GlobalTrace("else: len(cleaned) == 0")
			obj["enum"] = cleaned
		}
	}
	if props, ok := obj["properties"].(map[string]any); ok {
		observe.GlobalTrace("if: ok")
		for _, v := range props {
			observe.GlobalTrace("range props")
			if propObj, isMap := v.(map[string]any); isMap {
				observe.GlobalTrace("if: isMap")
				sanitizeObj(propObj)
			}
		}
	}
	if items, ok := obj["items"].(map[string]any); ok {
		observe.GlobalTrace("if: ok")
		sanitizeObj(items)
	}
	if anyOf, ok := obj["anyOf"].([]any); ok {
		observe.GlobalTrace("if: ok")
		for _, v := range anyOf {
			observe.GlobalTrace("range anyOf")
			if variant, isMap := v.(map[string]any); isMap {
				observe.GlobalTrace("if: isMap")
				sanitizeObj(variant)
			}
		}
	}
}

// responseFromGenai converts a genai response to gogent model.Response.
func responseFromGenai(resp *genai.GenerateContentResponse, modelName string) model.Response {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	result := model.Response{Model: modelName}
	if resp.UsageMetadata != nil {
		observe.GlobalTrace("if: resp.UsageMetadata != nil")
		result.Usage = usageFromGenai(resp.UsageMetadata)
	}
	if len(resp.Candidates) == 0 {
		observe.GlobalTrace("if: len(resp.Candidates) == 0")
		result.StopReason = model.StopError
		observe.GlobalTrace("return: result")
		return result
	}
	cand := resp.Candidates[0]
	result.StopReason = stopReasonFromGenai(cand.FinishReason)

	if cand.Content != nil {
		observe.GlobalTrace("if: cand.Content != nil")
		for _, part := range cand.Content.Parts {
			observe.GlobalTrace("range cand.Content.Parts")
			switch {
			case part.Text != "" && part.Thought:
				observe.GlobalTrace("case: part.Text != \"\" && part.Thought")
				tp := model.ThinkingPart{Text: part.Text}
				if len(part.ThoughtSignature) > 0 {
					tp.Signature = string(part.ThoughtSignature)
				}
				result.Content = append(result.Content, tp)
			case part.Text != "":
				observe.GlobalTrace("case: part.Text != \"\"")
				result.Content = append(result.Content, model.TextPart{Text: part.Text})
			case part.FunctionCall != nil:
				observe.GlobalTrace("case: part.FunctionCall != nil")
				fc := part.FunctionCall
				id := fc.ID
				if id == "" {
					id = model.NewUUID()
				}
				argsJSON, _ := json.Marshal(fc.Args)
				result.Content = append(result.Content, model.ToolCallPart{
					ID: id, Name: fc.Name, Input: argsJSON,
				})
			}
		}
	}

	if result.StopReason == model.StopEndTurn {
		observe.GlobalTrace("if: result.StopReason == model.StopEndTurn")
		for _, p := range result.Content {
			observe.GlobalTrace("range result.Content")
			if _, ok := p.(model.ToolCallPart); ok {
				observe.GlobalTrace("if: ok")
				result.StopReason = model.StopToolUse
				break
			}
		}
	}
	observe.GlobalTrace("return: result")
	return result
}

func usageFromGenai(u *genai.GenerateContentResponseUsageMetadata) model.TokenUsage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: model.TokenUsage{\n\tInputTokens:\t\tint(u.PromptTokenCount),\n\tOutputTokens:\t\tint...")
	return model.TokenUsage{
		InputTokens:          int(u.PromptTokenCount),
		OutputTokens:         int(u.CandidatesTokenCount) + int(u.ThoughtsTokenCount),
		CacheReadInputTokens: int(u.CachedContentTokenCount),
	}
}

func stopReasonFromGenai(fr genai.FinishReason) model.StopReason {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch fr {
	case genai.FinishReasonStop:
		observe.GlobalTrace("case: genai.FinishReasonStop")
		return model.StopEndTurn
	case genai.FinishReasonMaxTokens:
		observe.GlobalTrace("case: genai.FinishReasonMaxTokens")
		return model.StopMaxTokens
	default:
		observe.GlobalTrace("default")
		return model.StopError
	}
}

func (p *Provider) emitStart(traceID, spanID string, params provider.RequestParams) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	p.bus.Emit(observe.APIRequestStarted{
		EventHeader:   observe.NewEventHeader("APIRequestStarted", traceID, spanID, ""),
		Model:         params.Model,
		MessageCount:  len(params.Messages),
		ToolCount:     len(params.Tools),
		TokenEstimate: shared.EstimateTokens(params),
	})
}
