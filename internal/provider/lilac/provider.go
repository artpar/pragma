// Package lilac implements provider.Provider for Lilac (OpenAI-compatible) via any-llm-go.
package lilac

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/provider/anyllm"
	"github.com/artpar/pragma/internal/provider/rawcapture"
	"github.com/artpar/pragma/internal/provider/shared"
	"github.com/mozilla-ai/any-llm-go/config"
	"github.com/mozilla-ai/any-llm-go/providers"
	oai "github.com/mozilla-ai/any-llm-go/providers/openai"
	oaisdk "github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/param"
)

const (
	defaultBaseURL      = "https://api.getlilac.com/v1"
	lilacRequestTimeout = 90 * time.Second
)

// Provider wraps any-llm-go's OpenAI provider pointed at Lilac's endpoint.
type Provider struct {
	inner      providers.Provider
	bus        *observe.EventBus
	maxRetries int
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// Option configures the Provider.
type Option func(*providerConfig)

type providerConfig struct {
	baseURL string
}

// WithBaseURL overrides the API base URL.
func WithBaseURL(url string) Option {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: func(c *providerConfig) { c.baseURL = url }")
	return func(c *providerConfig) { c.baseURL = url }
}

// New creates a Lilac provider backed by any-llm-go.
func New(apiKey string, bus *observe.EventBus, opts ...Option) (*Provider, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	pc := providerConfig{baseURL: defaultBaseURL}
	for _, opt := range opts {
		observe.GlobalTrace("range opts")
		opt(&pc)
	}
	cfgOpts := []config.Option{
		config.WithAPIKey(apiKey),
		config.WithBaseURL(pc.baseURL),
		config.WithTimeout(lilacRequestTimeout),
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"lilac: create cookie jar: %w\", err)")
		return nil, fmt.Errorf("lilac: create cookie jar: %w", err)
	}
	httpClient := &http.Client{Timeout: lilacRequestTimeout, Jar: jar}
	if client, ok := rawcapture.HTTPClientFromEnv(lilacRequestTimeout); ok {
		observe.GlobalTrace("if: ok")
		httpClient = client
		httpClient.Jar = jar
		cfgOpts = append(cfgOpts, config.WithHTTPClient(client))
	}
	inner, err := oai.NewCompatible(oai.CompatibleConfig{
		Capabilities: providers.Capabilities{
			Completion:          true,
			CompletionImage:     true,
			CompletionPDF:       false,
			CompletionReasoning: true,
			CompletionStreaming: true,
			CompletionTools:     true,
			Embedding:           false,
			ListModels:          false,
		},
		DefaultBaseURL:                 pc.baseURL,
		Name:                           "lilac",
		RequireAPIKey:                  true,
		ChatCompletionRequestTransform: transformLilacRequest,
	}, cfgOpts...)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"lilac: create provider: %w\", err)")
		return nil, fmt.Errorf("lilac: create provider: %w", err)
	}
	observe.GlobalTrace("return: &Provider{inner: inner, bus: bus, maxRetries: 10}, nil")
	observe.GlobalTrace("return: &Provider{\n\tinner:\t\tinner,\n\tbus:\t\tbus,\n\tmaxRetries:\t10,\n\tapiKey:\t\tapiKey,\n\tba...")
	return &Provider{
		inner:      inner,
		bus:        bus,
		maxRetries: 10,
		apiKey:     apiKey,
		baseURL:    pc.baseURL,
		httpClient: httpClient,
	}, nil
}

func transformLilacRequest(req *oaisdk.ChatCompletionNewParams) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if req.MaxCompletionTokens.Valid() {
		observe.GlobalTrace("if: req.MaxCompletionTokens.Valid()")
		req.MaxTokens = oaisdk.Int(req.MaxCompletionTokens.Value)
	}
	req.MaxCompletionTokens = param.Opt[int64]{}
}

func (p *Provider) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"lilac\"")
	return "lilac"
}

func (p *Provider) SupportsFeature(feature provider.Feature) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch feature {
	case provider.FeatureToolUse, provider.FeatureStreaming, provider.FeatureImages, provider.FeatureThinking:
		observe.GlobalTrace("case: provider.FeatureToolUse, provider.FeatureStreaming, provider.FeatureImages, provider.FeatureThinking")
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

// ListModels returns sorted IDs of all known Lilac models.
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
	observe.GlobalTrace("return: 200_000, false")
	return 200_000, false
}

var lilacStatusClassify = shared.ClassifyByStatusCodes([]string{"429", "500", "502", "503", "504"})

func lilacClassify(err error) shared.ErrorClassification {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if isRetryableLilacTimeout(err) {
		observe.GlobalTrace("if: isRetryableLilacTimeout(err)")
		observe.GlobalTrace("return: shared.ErrorClassification{\n\tWrapped:\terr,\n\tRetryable:\ttrue,\n\tErrorType:\t\"tim...")
		return shared.ErrorClassification{
			Wrapped:   err,
			Retryable: true,
			ErrorType: "timeout",
		}
	}
	observe.GlobalTrace("return: lilacStatusClassify(err)")
	return lilacStatusClassify(err)
}

func isRetryableLilacTimeout(err error) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Timeout() {
		observe.GlobalTrace("if: errors.As(err, &urlErr) && urlErr.Timeout()")
		observe.GlobalTrace("return: true")
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		observe.GlobalTrace("if: errors.As(err, &netErr) && netErr.Timeout()")
		observe.GlobalTrace("return: true")
		return true
	}
	observe.GlobalTrace("return: strings.Contains(err.Error(), \"Client.Timeout exceeded while awaiting headers\")")

	return strings.Contains(err.Error(), "Client.Timeout exceeded while awaiting headers")
}

func (p *Provider) ensureMaxTokens(params *provider.RequestParams) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	info, known := LookupModel(params.Model)
	if params.MaxTokens == 0 && known && info.MaxOutput > 0 {
		observe.GlobalTrace("if: params.MaxTokens == 0 && known && info.MaxOutput > 0")
		params.MaxTokens = info.MaxOutput
	}

	if known && info.MaxContext > 0 && params.MaxTokens > 0 {
		observe.GlobalTrace("if: known && info.MaxContext > 0 && params.MaxTokens > 0")
		promptEst := shared.EstimateTokens(*params)
		headroom := info.MaxContext - promptEst
		if headroom < 1024 {
			observe.GlobalTrace("if: headroom < 1024")
			headroom = 1024
		}
		if params.MaxTokens > headroom {
			observe.GlobalTrace("if: params.MaxTokens > headroom")
			params.MaxTokens = headroom
		}
	}
}

func (p *Provider) Complete(ctx context.Context, params provider.RequestParams) (model.Response, error) {
	observe.TraceCtx(ctx, "lilac", "Provider.Complete", "enter")
	defer observe.TraceCtx(ctx, "lilac", "Provider.Complete", "exit")
	traceID := observe.NewTraceID()
	spanID := observe.NewSpanID()
	ctx = rawcapture.WithTrace(ctx, traceID, spanID)
	p.ensureMaxTokens(&params)
	p.emitStart(traceID, spanID, params)
	start := time.Now()

	llmParams := anyllm.RequestToParams(params)

	llmParams.StreamOptions = nil

	var comp *completionResponse
	err := shared.WithRetry(ctx, p.bus, p.maxRetries, traceID, spanID, lilacClassify, func() error {
		var reqErr error
		comp, reqErr = p.completeDirect(ctx, llmParams)
		return reqErr
	})
	if err != nil {
		observe.TraceCtx(ctx, "lilac", "Provider.Complete", "if: err != nil")
		observe.TraceCtx(ctx, "lilac", "Provider.Complete", "return: model.Response{}, err")
		return model.Response{}, err
	}

	resp := anyllm.ResponseFromCompletion(comp.toAnyLLM())
	if comp.Usage != nil {
		observe.TraceCtx(ctx, "lilac", "Provider.Complete", "if: comp.Usage != nil")
		resp.Usage = comp.Usage.toTokenUsage()
	}
	if resp.Usage.OutputTokens == 0 {
		observe.TraceCtx(ctx, "lilac", "Provider.Complete", "if: resp.Usage.OutputTokens == 0")
		resp.Usage.OutputTokens = shared.EstimateOutputTokens(resp.Content)
	}
	if resp.Usage.InputTokens == 0 {
		observe.TraceCtx(ctx, "lilac", "Provider.Complete", "if: resp.Usage.InputTokens == 0")
		resp.Usage.InputTokens = shared.EstimateTokens(params)
	}
	p.bus.Emit(observe.APIRequestCompleted{
		EventHeader: observe.NewEventHeader("APIRequestCompleted", traceID, spanID, ""),
		StopReason:  resp.StopReason,
		Usage:       resp.Usage,
		DurationMs:  time.Since(start).Milliseconds(),
		Model:       resp.Model,
		Content:     shared.MarshalContent(resp.Content),
	})
	observe.TraceCtx(ctx, "lilac", "Provider.Complete", "return: resp, nil")
	return resp, nil
}

type chatCompletionRequest struct {
	Messages          []providers.Message       `json:"messages"`
	Model             string                    `json:"model"`
	MaxTokens         *int                      `json:"max_tokens,omitempty"`
	Temperature       *temperatureParam         `json:"temperature,omitempty"`
	TopP              *float64                  `json:"top_p,omitempty"`
	Stop              []string                  `json:"stop,omitempty"`
	Tools             []providers.Tool          `json:"tools,omitempty"`
	ToolChoice        any                       `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool                     `json:"parallel_tool_calls,omitempty"`
	ResponseFormat    *providers.ResponseFormat `json:"response_format,omitempty"`
	ReasoningEffort   providers.ReasoningEffort `json:"reasoning_effort,omitempty"`
	Seed              *int                      `json:"seed,omitempty"`
	User              string                    `json:"user,omitempty"`
	Stream            bool                      `json:"stream,omitempty"`
	StreamOptions     *providers.StreamOptions  `json:"stream_options,omitempty"`
}

type temperatureParam float64

func formatTemperatureParam(value *float64) *temperatureParam {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if value == nil {
		observe.GlobalTrace("if: value == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	temperature := temperatureParam(*value)
	observe.GlobalTrace("return: &temperature")
	return &temperature
}

func (t temperatureParam) MarshalJSON() ([]byte, error) {
	value := float64(t)
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil, fmt.Errorf("unsupported temperature value %v", value)
	}
	if math.Trunc(value) == value {
		return []byte(strconv.FormatFloat(value, 'f', 1, 64)), nil
	}
	return []byte(strconv.FormatFloat(value, 'f', -1, 64)), nil
}

func (p *Provider) completeDirect(ctx context.Context, params providers.CompletionParams) (*completionResponse, error) {
	observe.TraceCtx(ctx, "lilac", "Provider.completeDirect", "enter")
	defer observe.TraceCtx(ctx, "lilac", "Provider.completeDirect", "exit")
	reqBody := chatCompletionRequest{
		Messages:          params.Messages,
		Model:             params.Model,
		MaxTokens:         params.MaxTokens,
		Temperature:       formatTemperatureParam(params.Temperature),
		TopP:              params.TopP,
		Stop:              params.Stop,
		Tools:             params.Tools,
		ToolChoice:        params.ToolChoice,
		ParallelToolCalls: params.ParallelToolCalls,
		ResponseFormat:    params.ResponseFormat,
		ReasoningEffort:   params.ReasoningEffort,
		Seed:              params.Seed,
		User:              params.User,
		Stream:            params.Stream,
		StreamOptions:     params.StreamOptions,
	}
	if len(reqBody.Tools) == 0 && reqBody.ToolChoice == nil {
		observe.TraceCtx(ctx, "lilac", "Provider.completeDirect", "if: len(reqBody.Tools) == 0 && reqBody.ToolChoice == nil")
		reqBody.ToolChoice = "none"
	}

	var body bytes.Buffer
	enc := json.NewEncoder(&body)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(reqBody); err != nil {
		observe.TraceCtx(ctx, "lilac", "Provider.completeDirect", "if: err != nil")
		observe.TraceCtx(ctx, "lilac", "Provider.completeDirect", "return: nil, fmt.Errorf(\"lilac: encode chat completion request: %w\", err)")
		return nil, fmt.Errorf("lilac: encode chat completion request: %w", err)
	}
	bodyBytes := bytes.TrimSuffix(body.Bytes(), []byte("\n"))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(p.baseURL, "/")+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		observe.TraceCtx(ctx, "lilac", "Provider.completeDirect", "if: err != nil")
		observe.TraceCtx(ctx, "lilac", "Provider.completeDirect", "return: nil, fmt.Errorf(\"lilac: create chat completion request: %w\", err)")
		return nil, fmt.Errorf("lilac: create chat completion request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	setOpenAICompatibleHeaders(req, lilacRequestTimeout)

	httpClient := p.httpClient
	if httpClient == nil {
		observe.TraceCtx(ctx, "lilac", "Provider.completeDirect", "if: httpClient == nil")
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		observe.TraceCtx(ctx, "lilac", "Provider.completeDirect", "if: err != nil")
		observe.TraceCtx(ctx, "lilac", "Provider.completeDirect", "return: nil, err")
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		observe.TraceCtx(ctx, "lilac", "Provider.completeDirect", "if: err != nil")
		observe.TraceCtx(ctx, "lilac", "Provider.completeDirect", "return: nil, fmt.Errorf(\"lilac: read chat completion response: %w\", err)")
		return nil, fmt.Errorf("lilac: read chat completion response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		observe.TraceCtx(ctx, "lilac", "Provider.completeDirect", "if: resp.StatusCode < 200 || resp.StatusCode >= 300")
		observe.TraceCtx(ctx, "lilac", "Provider.completeDirect", "return: nil, fmt.Errorf(\"lilac: status %d: %s\", resp.StatusCode, strings.TrimSpace(st...")
		return nil, fmt.Errorf("lilac: status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var wire completionResponse
	if err := json.Unmarshal(respBody, &wire); err != nil {
		observe.TraceCtx(ctx, "lilac", "Provider.completeDirect", "if: err != nil")
		observe.TraceCtx(ctx, "lilac", "Provider.completeDirect", "return: nil, fmt.Errorf(\"lilac: decode chat completion response: %w\", err)")
		return nil, fmt.Errorf("lilac: decode chat completion response: %w", err)
	}
	observe.TraceCtx(ctx, "lilac", "Provider.completeDirect", "return: &wire, nil")
	return &wire, nil
}

func setOpenAICompatibleHeaders(req *http.Request, timeout time.Duration) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	req.Header.Set("User-Agent", "OpenAI/Python 2.38.0")
	req.Header.Set("X-Stainless-Lang", "python")
	req.Header.Set("X-Stainless-Package-Version", "2.38.0")
	req.Header.Set("X-Stainless-OS", "Linux")
	req.Header.Set("X-Stainless-Arch", "arm64")
	req.Header.Set("X-Stainless-Runtime", "CPython")
	req.Header.Set("X-Stainless-Runtime-Version", "3.12.12")
	req.Header.Set("X-Stainless-Async", "false")
	req.Header.Set("X-Stainless-Raw-Response", "true")
	req.Header.Set("X-Stainless-Retry-Count", "0")
	req.Header.Set("X-Stainless-Read-Timeout", strconv.FormatFloat(timeout.Seconds(), 'f', 1, 64))
}

type completionResponse struct {
	ID                string             `json:"id"`
	Object            string             `json:"object"`
	Created           int64              `json:"created"`
	Model             string             `json:"model"`
	Choices           []completionChoice `json:"choices"`
	Usage             *completionUsage   `json:"usage,omitempty"`
	SystemFingerprint string             `json:"system_fingerprint,omitempty"`
}

type completionUsage struct {
	PromptTokens        int                 `json:"prompt_tokens"`
	CompletionTokens    int                 `json:"completion_tokens"`
	TotalTokens         int                 `json:"total_tokens"`
	ReasoningTokens     int                 `json:"reasoning_tokens,omitempty"`
	PromptTokensDetails promptTokensDetails `json:"prompt_tokens_details,omitempty"`
}

type promptTokensDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
}

func (u *completionUsage) toAnyLLM() *providers.Usage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if u == nil {
		observe.GlobalTrace("if: u == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	observe.GlobalTrace("return: &providers.Usage{\n\tPromptTokens:\t\tu.PromptTokens,\n\tCompletionTokens:\tu.Comple...")
	return &providers.Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
		ReasoningTokens:  u.ReasoningTokens,
	}
}

func (u *completionUsage) toTokenUsage() model.TokenUsage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if u == nil {
		observe.GlobalTrace("if: u == nil")
		observe.GlobalTrace("return: model.TokenUsage{}")
		return model.TokenUsage{}
	}
	cachedTokens := u.PromptTokensDetails.CachedTokens
	if cachedTokens < 0 {
		observe.GlobalTrace("if: cachedTokens < 0")
		cachedTokens = 0
	}
	if cachedTokens > u.PromptTokens {
		observe.GlobalTrace("if: cachedTokens > u.PromptTokens")
		cachedTokens = u.PromptTokens
	}
	observe.GlobalTrace("return: model.TokenUsage{\n\tInputTokens:\t\tu.PromptTokens - cachedTokens,\n\tOutputTokens...")
	return model.TokenUsage{
		InputTokens:          u.PromptTokens - cachedTokens,
		OutputTokens:         u.CompletionTokens,
		CacheReadInputTokens: cachedTokens,
	}
}

type completionChoice struct {
	Index        int               `json:"index"`
	Message      completionMessage `json:"message"`
	FinishReason string            `json:"finish_reason,omitempty"`
}

type completionMessage struct {
	Role      string               `json:"role"`
	Content   *string              `json:"content"`
	ToolCalls []providers.ToolCall `json:"tool_calls,omitempty"`
	Reasoning flexibleReasoning    `json:"reasoning,omitempty"`
}

type flexibleReasoning struct {
	value *providers.Reasoning
}

func (r *flexibleReasoning) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		r.value = &providers.Reasoning{Content: text}
		return nil
	}
	var reasoning providers.Reasoning
	if err := json.Unmarshal(data, &reasoning); err != nil {
		return err
	}
	r.value = &reasoning
	return nil
}

func (r flexibleReasoning) MarshalJSON() ([]byte, error) {
	if r.value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(r.value)
}

func (r completionResponse) toAnyLLM() *providers.ChatCompletion {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	choices := make([]providers.Choice, 0, len(r.Choices))
	for _, choice := range r.Choices {
		observe.GlobalTrace("range r.Choices")
		msg := providers.Message{
			Role:      choice.Message.Role,
			ToolCalls: choice.Message.ToolCalls,
		}
		if choice.Message.Content != nil {
			observe.GlobalTrace("if: choice.Message.Content != nil")
			msg.Content = *choice.Message.Content
		}
		choices = append(choices, providers.Choice{
			Index:        choice.Index,
			Message:      msg,
			FinishReason: choice.FinishReason,
		})
	}
	observe.GlobalTrace("return: &providers.ChatCompletion{\n\tID:\t\t\tr.ID,\n\tObject:\t\t\tr.Object,\n\tCreated:\t\tr.Cre...")
	return &providers.ChatCompletion{
		ID:                r.ID,
		Object:            r.Object,
		Created:           r.Created,
		Model:             r.Model,
		Choices:           choices,
		Usage:             r.Usage.toAnyLLM(),
		SystemFingerprint: r.SystemFingerprint,
	}
}

func (p *Provider) Stream(ctx context.Context, params provider.RequestParams) (<-chan provider.StreamChunk, error) {
	observe.TraceCtx(ctx, "lilac", "Provider.Stream", "enter")
	defer observe.TraceCtx(ctx, "lilac", "Provider.Stream", "exit")
	traceID := observe.NewTraceID()
	spanID := observe.NewSpanID()
	ctx = rawcapture.WithTrace(ctx, traceID, spanID)
	p.ensureMaxTokens(&params)
	p.emitStart(traceID, spanID, params)

	llmParams := anyllm.RequestToParams(params)
	chunks, errs := p.inner.CompletionStream(ctx, llmParams)

	ch := make(chan provider.StreamChunk, 32)
	go func() {
		defer close(ch)
		start := time.Now()
		var usage model.TokenUsage
		var respModel string
		var toolCallIDs []string
		seenToolCalls := make(map[string]bool)
		var accText strings.Builder
		accToolInputs := make(map[string]*strings.Builder)
		accToolNames := make(map[string]string)

		for chunk := range chunks {
			observe.TraceCtx(ctx, "lilac", "Provider.Stream", "range chunks")
			if chunk.Usage != nil {
				observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: chunk.Usage != nil")
				usage = anyllm.UsageFromAnyLLM(chunk.Usage)
			}
			if chunk.Model != "" {
				observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: chunk.Model != \"\"")
				respModel = chunk.Model
			}
			for _, choice := range chunk.Choices {
				observe.TraceCtx(ctx, "lilac", "Provider.Stream", "range chunk.Choices")
				delta := choice.Delta
				if delta.Content != "" {
					observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: delta.Content != \"\"")
					accText.WriteString(delta.Content)
					ch <- provider.StreamChunk{TextDelta: delta.Content}
				}
				if delta.Reasoning != nil && delta.Reasoning.Content != "" {
					observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: delta.Reasoning != nil && delta.Reasoning.Content != \"\"")
					ch <- provider.StreamChunk{ThinkingDelta: delta.Reasoning.Content}
				}
				for _, tc := range delta.ToolCalls {
					observe.TraceCtx(ctx, "lilac", "Provider.Stream", "range delta.ToolCalls")
					if tc.ID != "" && !seenToolCalls[tc.ID] {
						observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: tc.ID != \"\" && !seenToolCalls[tc.ID]")
						seenToolCalls[tc.ID] = true
						toolCallIDs = append(toolCallIDs, tc.ID)
						accToolInputs[tc.ID] = &strings.Builder{}
						accToolNames[tc.ID] = tc.Function.Name
						ch <- provider.StreamChunk{
							ToolCallStart: &model.ToolCallPart{ID: tc.ID, Name: tc.Function.Name},
						}
					}
					if tc.Function.Arguments != "" {
						observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: tc.Function.Arguments != \"\"")
						id := tc.ID
						if id == "" && len(toolCallIDs) > 0 {
							observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: id == \"\" && len(toolCallIDs) > 0")
							id = toolCallIDs[len(toolCallIDs)-1]
						}
						if id == "" {
							observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: id == \"\"")

							continue
						}
						if b, ok := accToolInputs[id]; ok {
							observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: ok")
							b.WriteString(tc.Function.Arguments)
						}
						ch <- provider.StreamChunk{
							ToolCallInputDelta: &provider.ToolCallDelta{ToolCallID: id, JSONDelta: tc.Function.Arguments},
						}
					}
				}
				if choice.FinishReason != "" {
					observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: choice.FinishReason != \"\"")
					stopReason := anyllm.StopReasonFromAnyLLM(choice.FinishReason)
					if stopReason == model.StopEndTurn && len(seenToolCalls) > 0 {
						observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: stopReason == model.StopEndTurn && len(seenToolCalls) > 0")
						stopReason = model.StopToolUse
					}

					if usage.OutputTokens == 0 {
						observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: usage.OutputTokens == 0")
						outputChars := accText.Len()
						for _, b := range accToolInputs {
							observe.TraceCtx(ctx, "lilac", "Provider.Stream", "range accToolInputs")
							outputChars += b.Len()
						}
						if outputChars > 0 {
							observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: outputChars > 0")
							usage.OutputTokens = outputChars / 4
						}
					}
					if usage.InputTokens == 0 {
						observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: usage.InputTokens == 0")
						usage.InputTokens = shared.EstimateTokens(params)
					}
					ch <- provider.StreamChunk{
						Done: &provider.StreamDone{StopReason: stopReason, Usage: usage, Model: respModel},
					}
					var accContent []model.ContentPart
					if accText.Len() > 0 {
						observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: accText.Len() > 0")
						accContent = append(accContent, model.TextPart{Text: accText.String()})
					}
					for _, id := range toolCallIDs {
						observe.TraceCtx(ctx, "lilac", "Provider.Stream", "range toolCallIDs")
						tc := model.ToolCallPart{ID: id, Name: accToolNames[id]}
						if b, ok := accToolInputs[id]; ok {
							observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: ok")
							tc.Input = json.RawMessage(b.String())
						}
						accContent = append(accContent, tc)
					}
					p.bus.Emit(observe.APIRequestCompleted{
						EventHeader: observe.NewEventHeader("APIRequestCompleted", traceID, spanID, ""),
						StopReason:  stopReason, Usage: usage,
						DurationMs: time.Since(start).Milliseconds(), Model: respModel,
						Content: shared.MarshalContent(accContent),
					})
				}
			}
		}
		select {
		case err, ok := <-errs:
			observe.TraceCtx(ctx, "lilac", "Provider.Stream", "select: err, ok := <-errs")
			if ok && err != nil {
				ch <- provider.StreamChunk{Error: err}
			}
		default:
			observe.TraceCtx(ctx, "lilac", "Provider.Stream", "select: default")
		}
	}()
	observe.TraceCtx(ctx, "lilac", "Provider.Stream", "return: ch, nil")
	return ch, nil
}

func (p *Provider) emitStart(traceID, spanID string, params provider.RequestParams) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	p.bus.Emit(shared.RequestStartedEvent(traceID, spanID, params))
}
