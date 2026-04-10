package ask

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// testAsker is a real Asker that returns a fixed answer.
type testAsker struct {
	answer string
	err    error
}

func (a *testAsker) Ask(_ context.Context, _ string) (string, error) {
	return a.answer, a.err
}

func TestAskUserQuestion(t *testing.T) {
	t.Run("name and flags", func(t *testing.T) {
		tool := &Tool{Asker: &testAsker{}}
		if tool.Name() != "AskUserQuestion" {
			t.Fatalf("expected AskUserQuestion, got %s", tool.Name())
		}
		if !tool.Flags().ReadOnly {
			t.Fatal("expected read-only")
		}
		if tool.Flags().Concurrent {
			t.Fatal("expected not concurrent")
		}
	})

	t.Run("returns user answer", func(t *testing.T) {
		tool := &Tool{Asker: &testAsker{answer: "Yes, proceed"}}
		input := json.RawMessage(`{"question": "Should I continue?"}`)
		result, err := tool.Invoke(context.Background(), input, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Content != "Yes, proceed" {
			t.Fatalf("expected 'Yes, proceed', got %q", result.Content)
		}
	})

	t.Run("propagates asker error", func(t *testing.T) {
		tool := &Tool{Asker: &testAsker{err: errors.New("not interactive")}}
		input := json.RawMessage(`{"question": "Hello?"}`)
		_, err := tool.Invoke(context.Background(), input, nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty question rejected", func(t *testing.T) {
		tool := &Tool{Asker: &testAsker{}}
		input := json.RawMessage(`{"question": ""}`)
		_, err := tool.Invoke(context.Background(), input, nil)
		if err == nil {
			t.Fatal("expected error for empty question")
		}
	})

	t.Run("invalid JSON rejected", func(t *testing.T) {
		tool := &Tool{Asker: &testAsker{}}
		input := json.RawMessage(`{bad`)
		_, err := tool.Invoke(context.Background(), input, nil)
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})

	t.Run("schema is valid JSON", func(t *testing.T) {
		tool := &Tool{Asker: &testAsker{}}
		var schema map[string]any
		if err := json.Unmarshal(tool.InputSchema(), &schema); err != nil {
			t.Fatalf("invalid schema JSON: %v", err)
		}
	})
}
