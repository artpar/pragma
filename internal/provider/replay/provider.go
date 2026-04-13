package replay

import (
	"context"
	"fmt"
	"sync"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
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
	return &Provider{
		engine:   engine,
		fallback: fallback,
		maxTurn:  maxTurn,
	}
}

func (p *Provider) Name() string { return "replay" }

func (p *Provider) Stream(ctx context.Context, params provider.RequestParams) (<-chan provider.StreamChunk, error) {
	resp, useFallback, err := p.nextResponse()
	if err != nil {
		return nil, err
	}
	if useFallback {
		return p.fallback.Stream(ctx, params)
	}

	// Convert recorded response into stream chunks (content first, then Done).
	// The engine expects TextDelta/ToolCallStart/ToolCallInputDelta before StreamDone.
	chunks := responseToChunks(resp)
	ch := make(chan provider.StreamChunk, len(chunks))
	for _, c := range chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}

func (p *Provider) Complete(ctx context.Context, params provider.RequestParams) (model.Response, error) {
	resp, useFallback, err := p.nextResponse()
	if err != nil {
		return model.Response{}, err
	}
	if useFallback {
		return p.fallback.Complete(ctx, params)
	}
	return resp, nil
}

func (p *Provider) SupportsFeature(_ provider.Feature) bool { return true }

func (p *Provider) Pricing(_ string) (model.Pricing, bool) { return model.Pricing{}, false }

func (p *Provider) ContextWindow(_ string) (int, bool) { return 200_000, true }

// nextResponse advances the turn counter and returns the recorded response.
// Returns (response, useFallback, error).
func (p *Provider) nextResponse() (model.Response, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.turn++

	// Check if we should fall back to live provider
	if p.maxTurn > 0 && p.turn > p.maxTurn {
		if p.fallback != nil {
			return model.Response{}, true, nil
		}
		return model.Response{}, false, fmt.Errorf("turn %d exceeds max replay turn %d and no fallback provider configured", p.turn, p.maxTurn)
	}

	resp, ok := p.engine.APIResponse(p.turn)
	if ok {
		return resp, false, nil
	}

	// No recorded response for this turn
	if p.fallback != nil {
		return model.Response{}, true, nil
	}
	return model.Response{}, false, fmt.Errorf("no recorded API response for turn %d; use --until-turn=%d --then-live to switch to live provider", p.turn, p.turn-1)
}

// responseToChunks converts a model.Response into StreamChunks that the engine
// loop can process. Each content part becomes one or more chunks, followed by Done.
func responseToChunks(resp model.Response) []provider.StreamChunk {
	var chunks []provider.StreamChunk

	for _, part := range resp.Content {
		switch p := part.(type) {
		case model.TextPart:
			if p.Text != "" {
				chunks = append(chunks, provider.StreamChunk{TextDelta: p.Text})
			}
		case model.ThinkingPart:
			if p.Text != "" {
				chunks = append(chunks, provider.StreamChunk{ThinkingDelta: p.Text})
			}
		case model.ToolCallPart:
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

	return chunks
}
