package ask

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/tool"
)

// testAsker is a real Asker that records the request and returns a fixed response.
type testAsker struct {
	resp       tool.AskResponse
	err        error
	lastReq    tool.AskRequest
}

func (a *testAsker) Ask(_ context.Context, req tool.AskRequest) (tool.AskResponse, error) {
	a.lastReq = req
	return a.resp, a.err
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

	t.Run("legacy plain question returns answer directly", func(t *testing.T) {
		asker := &testAsker{resp: tool.AskResponse{
			Answers: map[string]string{"Should I continue?": "Yes, proceed"},
		}}
		tl := &Tool{Asker: asker}
		input := json.RawMessage(`{"question": "Should I continue?"}`)
		result, err := tl.Invoke(context.Background(), input, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Content != "Yes, proceed" {
			t.Fatalf("expected 'Yes, proceed', got %q", result.Content)
		}
		// Verify request was legacy format
		if asker.lastReq.Question != "Should I continue?" {
			t.Fatalf("expected legacy question, got %q", asker.lastReq.Question)
		}
		if len(asker.lastReq.Questions) != 0 {
			t.Fatal("expected no structured questions for legacy")
		}
	})

	t.Run("structured questions returns formatted response", func(t *testing.T) {
		asker := &testAsker{resp: tool.AskResponse{
			Answers: map[string]string{
				"Which approach?": "Simple",
			},
		}}
		tl := &Tool{Asker: asker}
		input := json.RawMessage(`{
			"questions": [{
				"question": "Which approach?",
				"header": "Approach",
				"options": [
					{"label": "Simple", "description": "Basic impl"},
					{"label": "Advanced", "description": "Full impl"}
				]
			}]
		}`)
		result, err := tl.Invoke(context.Background(), input, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Content, "User has answered your questions") {
			t.Fatalf("expected structured format, got %q", result.Content)
		}
		if !strings.Contains(result.Content, `"Which approach?"="Simple"`) {
			t.Fatalf("expected answer in content, got %q", result.Content)
		}
		// Verify request was structured
		if len(asker.lastReq.Questions) != 1 {
			t.Fatalf("expected 1 question, got %d", len(asker.lastReq.Questions))
		}
		if len(asker.lastReq.Questions[0].Options) != 2 {
			t.Fatalf("expected 2 options, got %d", len(asker.lastReq.Questions[0].Options))
		}
	})

	t.Run("multi-question structured response", func(t *testing.T) {
		asker := &testAsker{resp: tool.AskResponse{
			Answers: map[string]string{
				"Which library?": "React",
				"Which style?":   "Tailwind",
			},
		}}
		tl := &Tool{Asker: asker}
		input := json.RawMessage(`{
			"questions": [
				{
					"question": "Which library?",
					"header": "Library",
					"options": [{"label": "React"}, {"label": "Vue"}]
				},
				{
					"question": "Which style?",
					"header": "Style",
					"options": [{"label": "Tailwind"}, {"label": "CSS Modules"}]
				}
			]
		}`)
		result, err := tl.Invoke(context.Background(), input, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Content, `"Which library?"="React"`) {
			t.Fatalf("expected library answer, got %q", result.Content)
		}
		if !strings.Contains(result.Content, `"Which style?"="Tailwind"`) {
			t.Fatalf("expected style answer, got %q", result.Content)
		}
	})

	t.Run("propagates asker error", func(t *testing.T) {
		tl := &Tool{Asker: &testAsker{err: errors.New("not interactive")}}
		input := json.RawMessage(`{"question": "Hello?"}`)
		_, err := tl.Invoke(context.Background(), input, nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("neither question nor questions rejected", func(t *testing.T) {
		tl := &Tool{Asker: &testAsker{}}
		input := json.RawMessage(`{}`)
		_, err := tl.Invoke(context.Background(), input, nil)
		if err == nil {
			t.Fatal("expected error for empty input")
		}
	})

	t.Run("empty question string rejected", func(t *testing.T) {
		tl := &Tool{Asker: &testAsker{}}
		input := json.RawMessage(`{"question": ""}`)
		_, err := tl.Invoke(context.Background(), input, nil)
		if err == nil {
			t.Fatal("expected error for empty question")
		}
	})

	t.Run("invalid JSON rejected", func(t *testing.T) {
		tl := &Tool{Asker: &testAsker{}}
		input := json.RawMessage(`{bad`)
		_, err := tl.Invoke(context.Background(), input, nil)
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})

	t.Run("schema is valid JSON", func(t *testing.T) {
		tl := &Tool{Asker: &testAsker{}}
		var schema map[string]any
		if err := json.Unmarshal(tl.InputSchema(), &schema); err != nil {
			t.Fatalf("invalid schema JSON: %v", err)
		}
	})

	t.Run("empty answers returns no-answer message", func(t *testing.T) {
		asker := &testAsker{resp: tool.AskResponse{
			Answers: map[string]string{},
		}}
		tl := &Tool{Asker: asker}
		input := json.RawMessage(`{
			"questions": [{
				"question": "Which approach?",
				"header": "Approach",
				"options": [{"label": "A"}, {"label": "B"}]
			}]
		}`)
		result, err := tl.Invoke(context.Background(), input, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Content, "did not answer") {
			t.Fatalf("expected no-answer message, got %q", result.Content)
		}
	})
}
