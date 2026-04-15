package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/artpar/gogent/internal/model"
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
