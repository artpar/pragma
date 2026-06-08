package replay

import (
	"context"
	"fmt"
	"sync"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
)

// Provider implements provider.Provider using recorded API responses.
// For turns with recorded data, returns the recorded response.
// For turns beyond the recorded data, delegates to a fallback provider (if set).
type Provider struct {
	engine   *observe.ReplayEngine
	fallback provider.Provider // nil for pure deterministic mode
	maxTurn  int               // 0 = use all recorded, >0 = fallback after this turn
	mu       sync.Mutex
	turn     int
}

// New creates a ReplayProvider.
// If fallback is nil, turns without recorded responses return errors.
// If maxTurn > 0, turns after maxTurn delegate to fallback.
func New(engine *observe.ReplayEngine, fallback provider.Provider, maxTurn int) *Provider {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &Provider{\n\tengine:\t\tengine,\n\tfallback:\tfallback,\n\tmaxTurn:\tmaxTurn,\n}")
	return &Provider{
		engine:   engine,
		fallback: fallback,
		maxTurn:  maxTurn,
	}
}

func (p *Provider) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"replay\"")
	return "replay"
}

func (p *Provider) Stream(ctx context.Context, params provider.RequestParams) (<-chan provider.StreamChunk, error) {
	observe.TraceCtx(ctx, "replay", "Provider.Stream", "enter")
	defer observe.TraceCtx(ctx, "replay", "Provider.Stream", "exit")
	resp, useFallback, err := p.nextResponse()
	if err != nil {
		observe.TraceCtx(ctx, "replay", "Provider.Stream", "if: err != nil")
		observe.TraceCtx(ctx, "replay", "Provider.Stream", "return: nil, err")
		return nil, err
	}
	if useFallback {
		observe.TraceCtx(ctx, "replay", "Provider.Stream", "if: useFallback")
		observe.TraceCtx(ctx, "replay", "Provider.Stream", "return: p.fallback.Stream(ctx, params)")
		return p.fallback.Stream(ctx, params)
	}

	chunks := ResponseToChunks(resp)
	ch := make(chan provider.StreamChunk, len(chunks))
	for _, c := range chunks {
		observe.TraceCtx(ctx, "replay", "Provider.Stream", "range chunks")
		ch <- c
	}
	close(ch)
	observe.TraceCtx(ctx, "replay", "Provider.Stream", "return: ch, nil")
	return ch, nil
}

func (p *Provider) Complete(ctx context.Context, params provider.RequestParams) (model.Response, error) {
	observe.TraceCtx(ctx, "replay", "Provider.Complete", "enter")
	defer observe.TraceCtx(ctx, "replay", "Provider.Complete", "exit")
	resp, useFallback, err := p.nextResponse()
	if err != nil {
		observe.TraceCtx(ctx, "replay", "Provider.Complete", "if: err != nil")
		observe.TraceCtx(ctx, "replay", "Provider.Complete", "return: model.Response{}, err")
		return model.Response{}, err
	}
	if useFallback {
		observe.TraceCtx(ctx, "replay", "Provider.Complete", "if: useFallback")
		observe.TraceCtx(ctx, "replay", "Provider.Complete", "return: p.fallback.Complete(ctx, params)")
		return p.fallback.Complete(ctx, params)
	}
	observe.TraceCtx(ctx, "replay", "Provider.Complete", "return: resp, nil")
	return resp, nil
}

func (p *Provider) SupportsFeature(_ provider.Feature) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: true")
	return true
}

func (p *Provider) Pricing(_ string) (model.Pricing, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: model.Pricing{}, false")
	return model.Pricing{}, false
}

func (p *Provider) ContextWindow(_ string) (int, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: 200_000, true")
	return 200_000, true
}

// nextResponse advances the turn counter and returns the recorded response.
// Returns (response, useFallback, error).
func (p *Provider) nextResponse() (model.Response, bool, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	p.mu.Lock()
	defer p.mu.Unlock()
	p.turn++

	if p.maxTurn > 0 && p.turn > p.maxTurn {
		observe.GlobalTrace("if: p.maxTurn > 0 && p.turn > p.maxTurn")
		if p.fallback != nil {
			observe.GlobalTrace("if: p.fallback != nil")
			observe.GlobalTrace("return: model.Response{}, true, nil")
			return model.Response{}, true, nil
		}
		observe.GlobalTrace("return: model.Response{}, false, fmt.Errorf(\"turn %d exceeds max replay turn %d and n...")
		return model.Response{}, false, fmt.Errorf("turn %d exceeds max replay turn %d and no fallback provider configured", p.turn, p.maxTurn)
	}

	resp, ok := p.engine.APIResponse(p.turn)
	if ok {
		observe.GlobalTrace("if: ok")
		observe.GlobalTrace("return: resp, false, nil")
		return resp, false, nil
	}

	if p.fallback != nil {
		observe.GlobalTrace("if: p.fallback != nil")
		observe.GlobalTrace("return: model.Response{}, true, nil")
		return model.Response{}, true, nil
	}
	observe.GlobalTrace("return: model.Response{}, false, fmt.Errorf(\"no recorded API response for turn %d\")")
	return model.Response{}, false, fmt.Errorf("no recorded API response for turn %d", p.turn)
}

// ResponseToChunks converts a model.Response into StreamChunks that the engine
// loop can process. Each content part becomes one or more chunks, followed by Done.
func ResponseToChunks(resp model.Response) []provider.StreamChunk {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var chunks []provider.StreamChunk

	for _, part := range resp.Content {
		observe.GlobalTrace("range resp.Content")
		switch p := part.(type) {
		case model.TextPart:
			observe.GlobalTrace("typecase: model.TextPart")
			if p.Text != "" {
				chunks = append(chunks, provider.StreamChunk{TextDelta: p.Text})
			}
		case model.ThinkingPart:
			observe.GlobalTrace("typecase: model.ThinkingPart")
			if p.Text != "" {
				chunks = append(chunks, provider.StreamChunk{ThinkingDelta: p.Text})
			}
		case model.ToolCallPart:
			observe.GlobalTrace("typecase: model.ToolCallPart")
			chunks = append(chunks, provider.StreamChunk{
				ToolCallStart: &model.ToolCallPart{
					ID:   p.ID,
					Name: p.Name,
				},
			})
			if len(p.Input) > 0 {
				chunks = append(chunks, provider.StreamChunk{
					ToolCallInputDelta: &provider.ToolCallDelta{
						ToolCallID: p.ID,
						JSONDelta:  string(p.Input),
					},
				})
			}
		}
	}

	chunks = append(chunks, provider.StreamChunk{
		Done: &provider.StreamDone{
			StopReason: resp.StopReason,
			Usage:      resp.Usage,
			Model:      resp.Model,
		},
	})
	observe.GlobalTrace("return: chunks")

	return chunks
}
