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

	"github.com/artpar/pragma/internal/debug"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/provider/rawcapture"
	"github.com/artpar/pragma/internal/provider/shared"
	"google.golang.org/genai"
)

// Provider implements provider.Provider for Google Gemini.
type Provider struct {
	client     *genai.Client
	bus        *observe.EventBus
	maxRetries int
	cache      *cacheManager
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
	clientConfig := &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	}
	if client, ok := rawcapture.HTTPClientFromEnv(10 * time.Minute); ok {
		clientConfig.HTTPClient = client
	}
	client, err := genai.NewClient(context.Background(), clientConfig)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"google: create client: %w\", err)")
		return nil, fmt.Errorf("google: create client: %w", err)
	}
	p := &Provider{
		client:     client,
		bus:        bus,
		maxRetries: 10,
	}
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

func (p *Provider) Close(ctx context.Context) error {
	if p.cache == nil {
		return nil
	}
	return p.cache.Close(ctx)
}

func (p *Provider) SupportsFeature(feature provider.Feature) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch feature {
	case provider.FeatureToolUse, provider.FeatureStreaming,
		provider.FeatureImages, provider.FeatureThinking,
		provider.FeatureStructuredOutput:
		observe.GlobalTrace("case: provider.FeatureToolUse, provider.FeatureStreaming, provider.FeatureImages, p...")
		return true
	}
	observe.GlobalTrace("return: false")
	return false
}

// CountTokens returns the precise token count for the given request parameters
// using the Gemini CountTokens API. Implements provider.TokenCounter.
func (p *Provider) CountTokens(ctx context.Context, params provider.RequestParams) (int, error) {
	observe.TraceCtx(ctx, "google", "Provider.CountTokens", "enter")
	defer observe.TraceCtx(ctx, "google", "Provider.CountTokens", "exit")

	mapper := newGoogleToolNameMapper(params.Tools)
	contents, cfg := p.buildRequestWithToolNameMapper(params, mapper)
	resp, err := p.client.Models.CountTokens(ctx, params.Model, contents, &genai.CountTokensConfig{
		SystemInstruction: cfg.SystemInstruction,
		Tools:             cfg.Tools,
	})
	if err != nil {
		observe.TraceCtx(ctx, "google", "Provider.CountTokens", "if: err != nil")
		debug.Log("Google CountTokens error: %v", err)
		observe.TraceCtx(ctx, "google", "Provider.CountTokens", "if: err != nil")
		observe.TraceCtx(ctx, "google", "Provider.CountTokens", "return: 0, fmt.Errorf(\"google: count tokens: %w\", err)")
		return 0, fmt.Errorf("google: count tokens: %w", err)
	}
	debug.Log("Google CountTokens result for %s: %d", params.Model, resp.TotalTokens)
	observe.TraceCtx(ctx, "google", "Provider.CountTokens", fmt.Sprintf("return: %d", resp.TotalTokens))
	observe.TraceCtx(ctx, "google", "Provider.CountTokens", "return: int(resp.TotalTokens), nil")
	return int(resp.TotalTokens), nil
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

// ListModels returns sorted IDs of all known Google models.
func (p *Provider) ListModels() []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: ListModels()")
	return ListModels()
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

	mapper := newGoogleToolNameMapper(params.Tools)
	contents, cfg := p.buildRequestWithToolNameMapper(params, mapper)
	contents = p.applyCache(ctx, params.Model, contents, cfg)

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

	result := responseFromGenai(resp, params.Model, mapper)
	if result.StopReason == model.StopMalformedToolCall {
		err := fmt.Errorf("google: model returned MALFORMED_FUNCTION_CALL finish reason")
		p.bus.Emit(observe.APIRequestFailed{
			EventHeader:  observe.NewEventHeader("APIRequestFailed", traceID, spanID, ""),
			ErrorType:    "malformed_function_call",
			ErrorMessage: err.Error(),
			Retryable:    false,
			Attempt:      1,
		})
		observe.TraceCtx(ctx, "google", "Provider.Complete", "return: model.Response{}, malformed function call")
		return model.Response{}, err
	}
	p.bus.Emit(observe.APIRequestCompleted{
		EventHeader: observe.NewEventHeader("APIRequestCompleted", traceID, spanID, ""),
		StopReason:  result.StopReason, Usage: result.Usage,
		DurationMs: time.Since(start).Milliseconds(), Model: result.Model,
		Content: shared.MarshalContent(result.Content),
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

	mapper := newGoogleToolNameMapper(params.Tools)
	contents, cfg := p.buildRequestWithToolNameMapper(params, mapper)
	contents = p.applyCache(ctx, params.Model, contents, cfg)
	ch := make(chan provider.StreamChunk, 32)

	go func() {
		defer close(ch)
		start := time.Now()
		var usage model.TokenUsage
		seenToolCalls := make(map[string]bool)
		var accText strings.Builder
		var accToolCalls []model.ContentPart

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
					reason := ""
					if cand.FinishReason != "" {
						observe.TraceCtx(ctx, "google", "Provider.Stream", "if: cand.FinishReason != \"\"")
						reason = string(cand.FinishReason)
					}
					if p.bus != nil {
						observe.TraceCtx(ctx, "google", "Provider.Stream", "if: p.bus != nil")
						p.bus.Emit(observe.ErrorOccurred{
							EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
							Severity:     "warn",
							Component:    "google",
							ErrorType:    "nil_candidate_content",
							ErrorMessage: "candidate has nil Content, finish_reason=" + reason,
						})
					}
					if reason == "MALFORMED_FUNCTION_CALL" {
						observe.TraceCtx(ctx, "google", "Provider.Stream", "if: reason == \"MALFORMED_FUNCTION_CALL\"")
						ch <- provider.StreamChunk{
							Error: fmt.Errorf("%w: MALFORMED_FUNCTION_CALL (retryable)", provider.ErrServerError),
						}
						return
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
						accText.WriteString(part.Text)
						ch <- provider.StreamChunk{TextDelta: part.Text}
						if len(part.ThoughtSignature) > 0 {
							ch <- provider.StreamChunk{TextSignatureDelta: string(part.ThoughtSignature)}
						}
					case part.FunctionCall != nil:
						observe.TraceCtx(ctx, "google", "Provider.Stream", "case: part.FunctionCall != nil")
						fc := part.FunctionCall
						toolName := mapper.fromWire(fc.Name)
						id := fc.ID
						if id == "" {
							id = model.NewUUID()
						}
						seenToolCalls[id] = true
						tc := model.ToolCallPart{ID: id, Name: toolName}
						if fc.Args != nil {
							argsJSON, _ := json.Marshal(fc.Args)
							tc.Input = argsJSON
						}
						if len(part.ThoughtSignature) > 0 {
							tc.Signature = string(part.ThoughtSignature)
						}
						accToolCalls = append(accToolCalls, tc)
						ch <- provider.StreamChunk{
							ToolCallStart: &model.ToolCallPart{ID: id, Name: toolName, Signature: tc.Signature},
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
					var accContent []model.ContentPart
					if accText.Len() > 0 {
						observe.TraceCtx(ctx, "google", "Provider.Stream", "if: accText.Len() > 0")
						accContent = append(accContent, model.TextPart{Text: accText.String()})
					}
					accContent = append(accContent, accToolCalls...)
					p.bus.Emit(observe.APIRequestCompleted{
						EventHeader: observe.NewEventHeader("APIRequestCompleted", traceID, spanID, ""),
						StopReason:  stopReason, Usage: usage,
						DurationMs: time.Since(start).Milliseconds(), Model: params.Model,
						Content: shared.MarshalContent(accContent),
					})
				}
			}
		}
	}()
	observe.TraceCtx(ctx, "google", "Provider.Stream", "return: ch, nil")

	return ch, nil
}

// applyCache attempts to use context caching for the request. If the stable prefix
// is large enough and caching succeeds, it sets CachedContent on the config and
// returns only the tail (uncached) contents. Otherwise returns the original contents unchanged.
func (p *Provider) applyCache(ctx context.Context, model string, contents []*genai.Content, cfg *genai.GenerateContentConfig) []*genai.Content {
	observe.TraceCtx(ctx, "google", "Provider.applyCache", "enter")
	defer observe.TraceCtx(ctx, "google", "Provider.applyCache", "exit")

	if p.cache == nil {
		observe.TraceCtx(ctx, "google", "Provider.applyCache", "if: p.cache == nil")
		observe.TraceCtx(ctx, "google", "Provider.applyCache", "return: contents")
		return contents
	}

	stable, tail := splitStablePrefix(contents)
	if len(stable) == 0 {
		observe.TraceCtx(ctx, "google", "Provider.applyCache", "no stable prefix")
		observe.TraceCtx(ctx, "google", "Provider.applyCache", "return: contents")
		return contents
	}

	estimatedTokens := 0
	for _, c := range stable {
		observe.TraceCtx(ctx, "google", "Provider.applyCache", "range stable")
		for _, part := range c.Parts {
			observe.TraceCtx(ctx, "google", "Provider.applyCache", "range c.Parts")
			if part.Text != "" {
				observe.TraceCtx(ctx, "google", "Provider.applyCache", "if: part.Text != \"\"")
				estimatedTokens += len(part.Text) / 4
			}
		}
	}

	if cfg.SystemInstruction != nil {
		observe.TraceCtx(ctx, "google", "Provider.applyCache", "if: cfg.SystemInstruction != nil")
		for _, part := range cfg.SystemInstruction.Parts {
			observe.TraceCtx(ctx, "google", "Provider.applyCache", "range cfg.SystemInstruction.Parts")
			if part.Text != "" {
				observe.TraceCtx(ctx, "google", "Provider.applyCache", "if: part.Text != \"\"")
				estimatedTokens += len(part.Text) / 4
			}
		}
	}

	if estimatedTokens < minCacheTokens {
		observe.TraceCtx(ctx, "google", "Provider.applyCache", fmt.Sprintf("estimated %d tokens < min %d, skipping", estimatedTokens, minCacheTokens))
		observe.TraceCtx(ctx, "google", "Provider.applyCache", "return: contents")
		return contents
	}

	cacheName, err := p.cache.getOrCreateCache(ctx, model, stable, cfg.SystemInstruction, cfg.Tools, estimatedTokens)
	if err != nil || cacheName == "" {
		observe.TraceCtx(ctx, "google", "Provider.applyCache", "cache unavailable, using full contents")
		observe.TraceCtx(ctx, "google", "Provider.applyCache", "return: contents")
		return contents
	}

	cfg.CachedContent = cacheName

	cfg.SystemInstruction = nil
	cfg.Tools = nil
	observe.TraceCtx(ctx, "google", "Provider.applyCache", fmt.Sprintf("using cache %s, tail has %d messages", cacheName, len(tail)))
	observe.TraceCtx(ctx, "google", "Provider.applyCache", "return: tail")
	return tail
}

// buildRequest converts pragma params to genai SDK types.
func (p *Provider) buildRequest(params provider.RequestParams) ([]*genai.Content, *genai.GenerateContentConfig) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	mapper := newGoogleToolNameMapper(params.Tools)
	observe.GlobalTrace("return: p.buildRequestWithToolNameMapper(params, mapper)")
	return p.buildRequestWithToolNameMapper(params, mapper)
}

func (p *Provider) buildRequestWithToolNameMapper(params provider.RequestParams, mapper googleToolNameMapper) ([]*genai.Content, *genai.GenerateContentConfig) {
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
		cfg.Tools = toolsToGenai(params.Tools, mapper)
	}

	if len(params.ResponseSchema) > 0 {
		observe.GlobalTrace("if: len(params.ResponseSchema) > 0")
		cfg.ResponseMIMEType = "application/json"
		schema := rawJSONToGenaiSchema(params.ResponseSchema)
		if schema != nil {
			observe.GlobalTrace("if: schema != nil")
			cfg.ResponseSchema = schema
		}
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

	contents := messagesToGenai(params.Messages, mapper)
	observe.GlobalTrace("return: contents, cfg")

	return contents, cfg
}

// messagesToGenai converts internal messages to genai Content.
func messagesToGenai(msgs []model.Message, mappers ...googleToolNameMapper) []*genai.Content {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	mapper := googleToolNameMapper{}
	if len(mappers) > 0 {
		observe.GlobalTrace("if: len(mappers) > 0")
		mapper = mappers[0]
	}

	toolNames := make(map[string]string)
	for _, m := range msgs {
		observe.GlobalTrace("range msgs")
		if m.Role == model.RoleAssistant {
			observe.GlobalTrace("if: m.Role == model.RoleAssistant")
			for _, p := range m.Content {
				observe.GlobalTrace("range m.Content")
				if tc, ok := p.(model.ToolCallPart); ok {
					observe.GlobalTrace("if: ok")
					toolNames[tc.ID] = mapper.toWire(tc.Name)
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
					tp := &genai.Part{Text: part.Text}
					if part.Signature != "" {
						tp.ThoughtSignature = []byte(part.Signature)
					}
					parts = append(parts, tp)
				}
			case model.ToolCallPart:
				observe.GlobalTrace("typecase: model.ToolCallPart")
				var args map[string]any
				if len(part.Input) > 0 {
					_ = json.Unmarshal(part.Input, &args)
				}
				p := genai.NewPartFromFunctionCall(mapper.toWire(part.Name), args)
				if part.Signature != "" {
					p.ThoughtSignature = []byte(part.Signature)
				}
				parts = append(parts, p)
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

type googleToolNameMapper struct {
	originalToWire map[string]string
	wireToOriginal map[string]string
}

func newGoogleToolNameMapper(tools []model.ToolDef) googleToolNameMapper {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	mapper := googleToolNameMapper{
		originalToWire: make(map[string]string, len(tools)),
		wireToOriginal: make(map[string]string, len(tools)),
	}
	used := make(map[string]int, len(tools))
	for _, tool := range tools {
		observe.GlobalTrace("range tools")
		wire := googleSafeToolName(tool.Name)
		if count := used[wire]; count > 0 {
			observe.GlobalTrace("if: count > 0")
			used[wire] = count + 1
			wire = fmt.Sprintf("%s_%d", wire, count+1)
		} else {
			observe.GlobalTrace("else: count > 0")
			used[wire] = 1
		}
		mapper.originalToWire[tool.Name] = wire
		mapper.wireToOriginal[wire] = tool.Name
	}
	observe.GlobalTrace("return: mapper")
	return mapper
}

func (m googleToolNameMapper) toWire(name string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if mapped := m.originalToWire[name]; mapped != "" {
		observe.GlobalTrace("if: mapped != \"\"")
		observe.GlobalTrace("return: mapped")
		return mapped
	}
	observe.GlobalTrace("return: name")
	return name
}

func (m googleToolNameMapper) fromWire(name string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if mapped := m.wireToOriginal[name]; mapped != "" {
		observe.GlobalTrace("if: mapped != \"\"")
		observe.GlobalTrace("return: mapped")
		return mapped
	}
	observe.GlobalTrace("return: name")
	return name
}

func googleSafeToolName(name string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var b strings.Builder
	for _, r := range name {
		observe.GlobalTrace("range name")
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			observe.GlobalTrace("case: r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_'")
			b.WriteRune(r)
		default:
			observe.GlobalTrace("default")
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		observe.GlobalTrace("if: b.Len() == 0")
		observe.GlobalTrace("return: \"tool\"")
		return "tool"
	}
	observe.GlobalTrace("return: b.String()")
	return b.String()
}

// toolsToGenai converts pragma tool definitions to genai tools.
func toolsToGenai(tools []model.ToolDef, mappers ...googleToolNameMapper) []*genai.Tool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	mapper := googleToolNameMapper{}
	if len(mappers) > 0 {
		observe.GlobalTrace("if: len(mappers) > 0")
		mapper = mappers[0]
	}
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
			Name:        mapper.toWire(t.Name),
			Description: t.Description,
			Parameters:  schema,
		})
	}
	observe.GlobalTrace("return: []*genai.Tool{{FunctionDeclarations: decls}}")
	return []*genai.Tool{{FunctionDeclarations: decls}}
}

// rawJSONToGenaiSchema converts a json.RawMessage JSON Schema to a *genai.Schema,
// applying Gemini-compatible sanitization.
func rawJSONToGenaiSchema(raw json.RawMessage) *genai.Schema {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	sanitized := sanitizeSchema(raw)
	schema := &genai.Schema{}
	if err := json.Unmarshal(sanitized, schema); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	observe.GlobalTrace("return: schema")
	return schema
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

// responseFromGenai converts a genai response to pragma model.Response.
func responseFromGenai(resp *genai.GenerateContentResponse, modelName string, mappers ...googleToolNameMapper) model.Response {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	mapper := googleToolNameMapper{}
	if len(mappers) > 0 {
		observe.GlobalTrace("if: len(mappers) > 0")
		mapper = mappers[0]
	}
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
				tp := model.TextPart{Text: part.Text}
				if len(part.ThoughtSignature) > 0 {
					tp.Signature = string(part.ThoughtSignature)
				}
				result.Content = append(result.Content, tp)
			case part.FunctionCall != nil:
				observe.GlobalTrace("case: part.FunctionCall != nil")

				if cand.FinishReason == genai.FinishReasonMalformedFunctionCall || isContentFilteredFinishReason(cand.FinishReason) {
					observe.GlobalTrace("if: malformed or content-filtered — skipping tool call")
					continue
				}
				fc := part.FunctionCall
				id := fc.ID
				if id == "" {
					id = model.NewUUID()
				}
				argsJSON, _ := json.Marshal(fc.Args)
				tc := model.ToolCallPart{ID: id, Name: mapper.fromWire(fc.Name), Input: argsJSON}
				if len(part.ThoughtSignature) > 0 {
					tc.Signature = string(part.ThoughtSignature)
				}
				result.Content = append(result.Content, tc)
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

	inputTokens := int(u.PromptTokenCount) - int(u.CachedContentTokenCount)
	observe.GlobalTrace("return: model.TokenUsage{...}")
	observe.GlobalTrace("return: model.TokenUsage{\n\tInputTokens:\t\tinputTokens,\n\tOutputTokens:\t\tint(u.Candidate...")
	return model.TokenUsage{
		InputTokens:          inputTokens,
		OutputTokens:         int(u.CandidatesTokenCount) + int(u.ThoughtsTokenCount),
		CacheReadInputTokens: int(u.CachedContentTokenCount),
	}
}

func stopReasonFromGenai(fr genai.FinishReason) model.StopReason {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch fr {
	case genai.FinishReasonStop, genai.FinishReasonUnspecified:
		observe.GlobalTrace("case: genai.FinishReasonStop/Unspecified")
		return model.StopEndTurn
	case genai.FinishReasonMaxTokens:
		observe.GlobalTrace("case: genai.FinishReasonMaxTokens")
		return model.StopMaxTokens
	case genai.FinishReasonMalformedFunctionCall:
		observe.GlobalTrace("case: genai.FinishReasonMalformedFunctionCall")
		return model.StopMalformedToolCall
	case genai.FinishReasonSafety, genai.FinishReasonRecitation,
		genai.FinishReasonBlocklist, genai.FinishReasonProhibitedContent,
		genai.FinishReasonSPII, genai.FinishReasonImageSafety,
		genai.FinishReasonImageProhibitedContent,
		genai.FinishReasonImageRecitation, genai.FinishReasonImageOther:
		observe.GlobalTrace("case: content filtered — " + string(fr))
		return model.StopContentFiltered
	default:
		observe.GlobalTrace("default")
		return model.StopError
	}
}

func isContentFilteredFinishReason(fr genai.FinishReason) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch fr {
	case genai.FinishReasonSafety, genai.FinishReasonRecitation,
		genai.FinishReasonBlocklist, genai.FinishReasonProhibitedContent,
		genai.FinishReasonSPII, genai.FinishReasonImageSafety,
		genai.FinishReasonImageProhibitedContent,
		genai.FinishReasonImageRecitation, genai.FinishReasonImageOther:
		observe.GlobalTrace("case: genai.FinishReasonSafety, genai.FinishReasonRecitation, genai.FinishReasonBlo...")
		return true
	default:
		observe.GlobalTrace("default")
		return false
	}
}

func (p *Provider) emitStart(traceID, spanID string, params provider.RequestParams) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	p.bus.Emit(shared.RequestStartedEvent(traceID, spanID, params))
}
