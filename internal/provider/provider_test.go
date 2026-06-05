package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/artpar/pragma/internal/model"
)

// --- RequestParams.ResponseSchema ---

func TestRequestParamsResponseSchemaJSON(t *testing.T) {
	params := RequestParams{
		Model:          "test-model",
		MaxTokens:      1000,
		ResponseSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`),
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}

	var decoded RequestParams
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.ResponseSchema == nil {
		t.Fatal("expected ResponseSchema after round-trip")
	}
	if string(decoded.ResponseSchema) != string(params.ResponseSchema) {
		t.Errorf("ResponseSchema mismatch: got %s", string(decoded.ResponseSchema))
	}
}

func TestRequestParamsResponseSchemaOmittedWhenEmpty(t *testing.T) {
	params := RequestParams{
		Model:     "test-model",
		MaxTokens: 1000,
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}

	if _, ok := m["response_schema"]; ok {
		t.Error("expected response_schema to be omitted when nil")
	}
}

// --- Feature constants ---

func TestFeatureConstants(t *testing.T) {
	features := []Feature{
		FeaturePrefixCaching,
		FeatureThinking,
		FeatureImages,
		FeatureToolUse,
		FeatureStreaming,
		FeatureStructuredOutput,
	}

	// All features should be unique
	seen := make(map[Feature]bool)
	for _, f := range features {
		if seen[f] {
			t.Errorf("duplicate feature: %q", f)
		}
		seen[f] = true
	}

	if FeatureStructuredOutput != "structured_output" {
		t.Errorf("FeatureStructuredOutput: got %q", FeatureStructuredOutput)
	}
}

// --- TokenCounter interface ---

// mockTokenCounter implements TokenCounter for testing.
type mockTokenCounter struct {
	count int
	err   error
}

func (m *mockTokenCounter) CountTokens(_ context.Context, _ RequestParams) (int, error) {
	return m.count, m.err
}

func TestTokenCounterInterface(t *testing.T) {
	var tc TokenCounter = &mockTokenCounter{count: 42}
	count, err := tc.CountTokens(context.Background(), RequestParams{})
	if err != nil {
		t.Fatal(err)
	}
	if count != 42 {
		t.Errorf("count: got %d, want 42", count)
	}
}

type accountingTestProvider struct {
	response model.Response
	done     StreamDone
}

func (p *accountingTestProvider) Name() string { return "test-provider" }
func (p *accountingTestProvider) Stream(context.Context, RequestParams) (<-chan StreamChunk, error) {
	ch := make(chan StreamChunk, 1)
	ch <- StreamChunk{Done: &p.done}
	close(ch)
	return ch, nil
}
func (p *accountingTestProvider) Complete(context.Context, RequestParams) (model.Response, error) {
	return p.response, nil
}
func (p *accountingTestProvider) SupportsFeature(Feature) bool { return true }
func (p *accountingTestProvider) Pricing(string) (model.Pricing, bool) {
	return model.Pricing{InputPerMToken: 1, OutputPerMToken: 2}, true
}
func (p *accountingTestProvider) ContextWindow(string) (int, bool) { return 128000, true }
func (p *accountingTestProvider) CountTokens(context.Context, RequestParams) (int, error) {
	return 42, nil
}
func (p *accountingTestProvider) ListModels() []string { return []string{"test-model"} }

func TestWithAccountingRecordsCompleteUsage(t *testing.T) {
	ct := model.NewCostTracker(0)
	prov := WithAccounting(&accountingTestProvider{
		response: model.Response{
			Model: "test-model",
			Usage: model.TokenUsage{InputTokens: 1000, OutputTokens: 500},
		},
	}, ct, nil)

	if _, err := prov.Complete(context.Background(), RequestParams{Model: "test-model"}); err != nil {
		t.Fatal(err)
	}

	entries := ct.Snapshot()
	if len(entries) != 1 {
		t.Fatalf("cost entries = %d, want 1", len(entries))
	}
	if entries[0].Provider != "test-provider" || entries[0].Model != "test-model" {
		t.Fatalf("entry target = %s/%s, want test-provider/test-model", entries[0].Provider, entries[0].Model)
	}
	if entries[0].Usage.InputTokens != 1000 || entries[0].Usage.OutputTokens != 500 {
		t.Fatalf("usage = %+v, want input=1000 output=500", entries[0].Usage)
	}
}

func TestWithAccountingRecordsStreamUsage(t *testing.T) {
	ct := model.NewCostTracker(0)
	prov := WithAccounting(&accountingTestProvider{
		done: StreamDone{
			Model: "stream-model",
			Usage: model.TokenUsage{InputTokens: 25, OutputTokens: 10},
		},
	}, ct, nil)

	chunks, err := prov.Stream(context.Background(), RequestParams{Model: "stream-model"})
	if err != nil {
		t.Fatal(err)
	}
	for range chunks {
	}

	entries := ct.Snapshot()
	if len(entries) != 1 {
		t.Fatalf("cost entries = %d, want 1", len(entries))
	}
	if entries[0].Model != "stream-model" {
		t.Fatalf("entry model = %q, want stream-model", entries[0].Model)
	}
	if entries[0].Usage.InputTokens != 25 || entries[0].Usage.OutputTokens != 10 {
		t.Fatalf("usage = %+v, want input=25 output=10", entries[0].Usage)
	}
}

func TestWithAccountingPreservesOptionalProviderInterfaces(t *testing.T) {
	prov := WithAccounting(&accountingTestProvider{}, model.NewCostTracker(0), nil)
	counter, ok := prov.(TokenCounter)
	if !ok {
		t.Fatal("accounting provider should preserve TokenCounter")
	}
	if got, err := counter.CountTokens(context.Background(), RequestParams{}); err != nil || got != 42 {
		t.Fatalf("CountTokens = %d, %v; want 42, nil", got, err)
	}
	lister, ok := prov.(ModelLister)
	if !ok {
		t.Fatal("accounting provider should preserve ModelLister")
	}
	if got := lister.ListModels(); len(got) != 1 || got[0] != "test-model" {
		t.Fatalf("ListModels = %v, want [test-model]", got)
	}
	if WithAccounting(prov, model.NewCostTracker(0), nil) != prov {
		t.Fatal("WithAccounting should be idempotent")
	}
}

// --- StreamChunk and StreamDone ---

func TestStreamDoneCarriesUsage(t *testing.T) {
	done := StreamDone{
		StopReason: model.StopEndTurn,
		Usage: model.TokenUsage{
			InputTokens:              100,
			OutputTokens:             50,
			CacheReadInputTokens:     30,
			CacheCreationInputTokens: 0,
		},
		Model: "test-model",
	}
	if done.Usage.CacheReadInputTokens != 30 {
		t.Errorf("CacheReadInputTokens: got %d", done.Usage.CacheReadInputTokens)
	}
}
