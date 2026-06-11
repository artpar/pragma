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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if next == nil || tracker == nil {
		observe.GlobalTrace("if: next == nil || tracker == nil")
		observe.GlobalTrace("return: next")
		return next
	}
	if _, ok := next.(accountingWrapped); ok {
		observe.GlobalTrace("if: ok")
		observe.GlobalTrace("return: next")
		return next
	}
	base := &accountingProvider{next: next, tracker: tracker, bus: bus}
	counter, hasCounter := next.(TokenCounter)
	lister, hasLister := next.(ModelLister)
	switch {
	case hasCounter && hasLister:
		observe.GlobalTrace("case: hasCounter && hasLister")
		return &accountingProviderWithTokenCounterAndModelLister{accountingProvider: base, counter: counter, lister: lister}
	case hasCounter:
		observe.GlobalTrace("case: hasCounter")
		return &accountingProviderWithTokenCounter{accountingProvider: base, counter: counter}
	case hasLister:
		observe.GlobalTrace("case: hasLister")
		return &accountingProviderWithModelLister{accountingProvider: base, lister: lister}
	default:
		observe.GlobalTrace("default")
		return base
	}
}

func (p *accountingProvider) accountingWrapped() {}

func (p *accountingProvider) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: p.next.Name()")
	return p.next.Name()
}

func (p *accountingProvider) SupportsFeature(feature Feature) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: p.next.SupportsFeature(feature)")
	return p.next.SupportsFeature(feature)
}

func (p *accountingProvider) Pricing(modelID string) (model.Pricing, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: p.next.Pricing(modelID)")
	return p.next.Pricing(modelID)
}

func (p *accountingProvider) ContextWindow(modelID string) (int, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: p.next.ContextWindow(modelID)")
	return p.next.ContextWindow(modelID)
}

func (p *accountingProvider) Close(ctx context.Context) error {
	observe.TraceCtx(ctx, "provider", "accountingProvider.Close", "enter")
	defer observe.TraceCtx(ctx, "provider", "accountingProvider.Close", "exit")
	observe.TraceCtx(ctx, "provider", "accountingProvider.Close", "return: Close(ctx, p.next)")
	return Close(ctx, p.next)
}

func (p *accountingProvider) Complete(ctx context.Context, params RequestParams) (model.Response, error) {
	observe.TraceCtx(ctx, "provider", "accountingProvider.Complete", "enter")
	defer observe.TraceCtx(ctx, "provider", "accountingProvider.Complete", "exit")
	resp, err := p.next.Complete(ctx, params)
	if err != nil {
		observe.TraceCtx(ctx, "provider", "accountingProvider.Complete", "if: err != nil")
		observe.TraceCtx(ctx, "provider", "accountingProvider.Complete", "return: resp, err")
		return resp, err
	}
	p.record(params, resp.Usage, resp.Model)
	observe.TraceCtx(ctx, "provider", "accountingProvider.Complete", "return: resp, nil")
	return resp, nil
}

func (p *accountingProvider) Stream(ctx context.Context, params RequestParams) (<-chan StreamChunk, error) {
	observe.TraceCtx(ctx, "provider", "accountingProvider.Stream", "enter")
	defer observe.TraceCtx(ctx, "provider", "accountingProvider.Stream", "exit")
	chunks, err := p.next.Stream(ctx, params)
	if err != nil {
		observe.TraceCtx(ctx, "provider", "accountingProvider.Stream", "if: err != nil")
		observe.TraceCtx(ctx, "provider", "accountingProvider.Stream", "return: nil, err")
		return nil, err
	}
	out := make(chan StreamChunk)
	go func() {
		defer close(out)
		for chunk := range chunks {
			observe.TraceCtx(ctx, "provider", "accountingProvider.Stream", "range chunks")
			if chunk.Done != nil {
				observe.TraceCtx(ctx, "provider", "accountingProvider.Stream", "if: chunk.Done != nil")
				p.record(params, chunk.Done.Usage, chunk.Done.Model)
			}
			out <- chunk
		}
	}()
	observe.TraceCtx(ctx, "provider", "accountingProvider.Stream", "return: out, nil")
	return out, nil
}

func (p *accountingProvider) record(params RequestParams, usage model.TokenUsage, responseModel string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	modelID := responseModel
	if modelID == "" {
		observe.GlobalTrace("if: modelID == \"\"")
		modelID = params.Model
	}
	pricing, known := p.next.Pricing(modelID)
	if !known && params.Model != "" && params.Model != modelID {
		observe.GlobalTrace("if: !known && params.Model != \"\" && params.Model != modelID")
		pricing, known = p.next.Pricing(params.Model)
	}
	if !known && p.bus != nil {
		observe.GlobalTrace("if: !known && p.bus != nil")
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
	observe.TraceCtx(ctx, "provider", "accountingProviderWithTokenCounter.CountTokens", "enter")
	defer observe.TraceCtx(ctx, "provider", "accountingProviderWithTokenCounter.CountTokens", "exit")
	observe.TraceCtx(ctx, "provider", "accountingProviderWithTokenCounter.CountTokens", "return: p.counter.CountTokens(ctx, params)")
	return p.counter.CountTokens(ctx, params)
}

type accountingProviderWithModelLister struct {
	*accountingProvider
	lister ModelLister
}

func (p *accountingProviderWithModelLister) ListModels() []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: p.lister.ListModels()")
	return p.lister.ListModels()
}

type accountingProviderWithTokenCounterAndModelLister struct {
	*accountingProvider
	counter TokenCounter
	lister  ModelLister
}

func (p *accountingProviderWithTokenCounterAndModelLister) CountTokens(ctx context.Context, params RequestParams) (int, error) {
	observe.TraceCtx(ctx, "provider", "accountingProviderWithTokenCounterAndModelLister.CountTokens", "enter")
	defer observe.TraceCtx(ctx, "provider", "accountingProviderWithTokenCounterAndModelLister.CountTokens", "exit")
	observe.TraceCtx(ctx, "provider", "accountingProviderWithTokenCounterAndModelLister.CountTokens", "return: p.counter.CountTokens(ctx, params)")
	return p.counter.CountTokens(ctx, params)
}

func (p *accountingProviderWithTokenCounterAndModelLister) ListModels() []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: p.lister.ListModels()")
	return p.lister.ListModels()
}
