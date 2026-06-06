package provider

import (
	"context"
	"fmt"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

type accountingWrapped interface {
	accountingWrapped()
}

type accountingProvider struct {
	next    Provider
	tracker *model.CostTracker
	bus     *observe.EventBus
}

// WithAccounting records token usage and cost for successful provider calls.
func WithAccounting(next Provider, tracker *model.CostTracker, bus *observe.EventBus) Provider {
	if next == nil || tracker == nil {
		return next
	}
	if _, ok := next.(accountingWrapped); ok {
		return next
	}
	base := &accountingProvider{next: next, tracker: tracker, bus: bus}
	counter, hasCounter := next.(TokenCounter)
	lister, hasLister := next.(ModelLister)
	switch {
	case hasCounter && hasLister:
		return &accountingProviderWithTokenCounterAndModelLister{accountingProvider: base, counter: counter, lister: lister}
	case hasCounter:
		return &accountingProviderWithTokenCounter{accountingProvider: base, counter: counter}
	case hasLister:
		return &accountingProviderWithModelLister{accountingProvider: base, lister: lister}
	default:
		return base
	}
}

func (p *accountingProvider) accountingWrapped() {}

func (p *accountingProvider) Name() string {
	return p.next.Name()
}

func (p *accountingProvider) SupportsFeature(feature Feature) bool {
	return p.next.SupportsFeature(feature)
}

func (p *accountingProvider) Pricing(modelID string) (model.Pricing, bool) {
	return p.next.Pricing(modelID)
}

func (p *accountingProvider) ContextWindow(modelID string) (int, bool) {
	return p.next.ContextWindow(modelID)
}

func (p *accountingProvider) Close(ctx context.Context) error {
	return Close(ctx, p.next)
}

func (p *accountingProvider) Complete(ctx context.Context, params RequestParams) (model.Response, error) {
	resp, err := p.next.Complete(ctx, params)
	if err != nil {
		return resp, err
	}
	p.record(params, resp.Usage, resp.Model)
	return resp, nil
}

func (p *accountingProvider) Stream(ctx context.Context, params RequestParams) (<-chan StreamChunk, error) {
	chunks, err := p.next.Stream(ctx, params)
	if err != nil {
		return nil, err
	}
	out := make(chan StreamChunk)
	go func() {
		defer close(out)
		for chunk := range chunks {
			if chunk.Done != nil {
				p.record(params, chunk.Done.Usage, chunk.Done.Model)
			}
			out <- chunk
		}
	}()
	return out, nil
}

func (p *accountingProvider) record(params RequestParams, usage model.TokenUsage, responseModel string) {
	modelID := responseModel
	if modelID == "" {
		modelID = params.Model
	}
	pricing, known := p.next.Pricing(modelID)
	if !known && params.Model != "" && params.Model != modelID {
		pricing, known = p.next.Pricing(params.Model)
	}
	if !known && p.bus != nil {
		p.bus.Emit(observe.ErrorOccurred{
			EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
			Severity:     "warn",
			Component:    "provider",
			ErrorType:    "unknown_model_pricing",
			ErrorMessage: fmt.Sprintf("no pricing data for model %q, costs will be zero", modelID),
		})
	}
	p.tracker.Record(modelID, p.next.Name(), usage, pricing)
}

type accountingProviderWithTokenCounter struct {
	*accountingProvider
	counter TokenCounter
}

func (p *accountingProviderWithTokenCounter) CountTokens(ctx context.Context, params RequestParams) (int, error) {
	return p.counter.CountTokens(ctx, params)
}

type accountingProviderWithModelLister struct {
	*accountingProvider
	lister ModelLister
}

func (p *accountingProviderWithModelLister) ListModels() []string {
	return p.lister.ListModels()
}

type accountingProviderWithTokenCounterAndModelLister struct {
	*accountingProvider
	counter TokenCounter
	lister  ModelLister
}

func (p *accountingProviderWithTokenCounterAndModelLister) CountTokens(ctx context.Context, params RequestParams) (int, error) {
	return p.counter.CountTokens(ctx, params)
}

func (p *accountingProviderWithTokenCounterAndModelLister) ListModels() []string {
	return p.lister.ListModels()
}
