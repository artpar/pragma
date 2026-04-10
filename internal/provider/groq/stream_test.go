package groq

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
)

func makeSSE(lines ...string) string {
	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString("data: ")
		sb.WriteString(l)
		sb.WriteString("\n\n")
	}
	return sb.String()
}

func newMockResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{},
	}
}

func TestStream_TextOnly(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	sse := makeSSE(
		`{"id":"1","model":"llama","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"}}]}`,
		`{"id":"1","model":"llama","choices":[{"index":0,"delta":{"content":" world"}}]}`,
		`{"id":"1","model":"llama","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
		"[DONE]",
	)

	p := New("test-key", bus, WithIdleTimeout(0))
	resp := newMockResponse(sse)
	ch := p.startStream(t.Context(), resp, NewIDMapper(), bus, "t", "s")

	chunks := drainChunks(ch)
	assertChunkText(t, chunks, "Hello world")
	assertDone(t, chunks, model.StopEndTurn)
}

func TestStream_ToolCall(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	sse := makeSSE(
		`{"id":"1","model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"bash","arguments":""}}]}}]}`,
		`{"id":"1","model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"cmd\""}}]}}]}`,
		`{"id":"1","model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"ls\"}"}}]}}]}`,
		`{"id":"1","model":"m","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`,
		"[DONE]",
	)

	mapper := NewIDMapper()
	p := New("test-key", bus, WithIdleTimeout(0))
	resp := newMockResponse(sse)
	ch := p.startStream(t.Context(), resp, mapper, bus, "t", "s")

	var toolStart *model.ToolCallPart
	var argDeltas []string
	var done *provider.StreamDone

	for chunk := range ch {
		if chunk.ToolCallStart != nil {
			toolStart = chunk.ToolCallStart
		}
		if chunk.ToolCallInputDelta != nil {
			argDeltas = append(argDeltas, chunk.ToolCallInputDelta.JSONDelta)
		}
		if chunk.Done != nil {
			done = chunk.Done
		}
		if chunk.Error != nil {
			t.Fatalf("unexpected error: %v", chunk.Error)
		}
	}

	if toolStart == nil {
		t.Fatal("no tool call start")
	}
	if toolStart.Name != "bash" {
		t.Errorf("tool name=%q, want bash", toolStart.Name)
	}
	if len(argDeltas) != 2 {
		t.Errorf("got %d arg deltas, want 2", len(argDeltas))
	}

	fullArgs := strings.Join(argDeltas, "")
	if fullArgs != `{"cmd":"ls"}` {
		t.Errorf("accumulated args=%q", fullArgs)
	}

	if done == nil {
		t.Fatal("no done chunk")
	}
	if done.StopReason != model.StopToolUse {
		t.Errorf("stop_reason=%q, want tool_use", done.StopReason)
	}
}

func TestStream_Reasoning(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	sse := makeSSE(
		`{"id":"1","model":"m","choices":[{"index":0,"delta":{"reasoning":"Step 1..."}}]}`,
		`{"id":"1","model":"m","choices":[{"index":0,"delta":{"content":"Answer: 42"}}]}`,
		`{"id":"1","model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":10,"total_tokens":15}}`,
		"[DONE]",
	)

	p := New("test-key", bus, WithIdleTimeout(0))
	resp := newMockResponse(sse)
	ch := p.startStream(t.Context(), resp, NewIDMapper(), bus, "t", "s")

	var thinking, text string
	for chunk := range ch {
		if chunk.ThinkingDelta != "" {
			thinking += chunk.ThinkingDelta
		}
		if chunk.TextDelta != "" {
			text += chunk.TextDelta
		}
		if chunk.Error != nil {
			t.Fatalf("unexpected error: %v", chunk.Error)
		}
	}

	if thinking != "Step 1..." {
		t.Errorf("thinking=%q", thinking)
	}
	if text != "Answer: 42" {
		t.Errorf("text=%q", text)
	}
}

func TestStream_SplitFinishAndUsage(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	// Groq often sends finish_reason and usage in separate chunks
	sse := makeSSE(
		`{"id":"1","model":"m","choices":[{"index":0,"delta":{"content":"Hi"}}]}`,
		`{"id":"1","model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`{"id":"1","model":"m","choices":[],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`,
		"[DONE]",
	)

	p := New("test-key", bus, WithIdleTimeout(0))
	resp := newMockResponse(sse)
	ch := p.startStream(t.Context(), resp, NewIDMapper(), bus, "t", "s")

	chunks := drainChunks(ch)
	assertChunkText(t, chunks, "Hi")
	assertDone(t, chunks, model.StopEndTurn)

	// Verify no errors
	for _, c := range chunks {
		if c.Error != nil {
			t.Errorf("unexpected error: %v", c.Error)
		}
	}
}

func TestStream_FinishReasonNoUsage(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	// finish_reason but no usage at all, then [DONE]
	sse := makeSSE(
		`{"id":"1","model":"m","choices":[{"index":0,"delta":{"content":"Hi"}}]}`,
		`{"id":"1","model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		"[DONE]",
	)

	p := New("test-key", bus, WithIdleTimeout(0))
	resp := newMockResponse(sse)
	ch := p.startStream(t.Context(), resp, NewIDMapper(), bus, "t", "s")

	chunks := drainChunks(ch)
	assertChunkText(t, chunks, "Hi")
	assertDone(t, chunks, model.StopEndTurn)

	// Verify no errors
	for _, c := range chunks {
		if c.Error != nil {
			t.Errorf("unexpected error: %v", c.Error)
		}
	}
}

func TestStream_DoneSignal(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	// Stream with no [DONE] and no usage
	sse := "data: {\"id\":\"1\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"}}]}\n\n"

	p := New("test-key", bus, WithIdleTimeout(0))
	resp := newMockResponse(sse)
	ch := p.startStream(t.Context(), resp, NewIDMapper(), bus, "t", "s")

	var gotError bool
	for chunk := range ch {
		if chunk.Error != nil {
			gotError = true
		}
	}
	if !gotError {
		t.Error("expected error when stream ends without Done")
	}
}

// helpers

func drainChunks(ch <-chan provider.StreamChunk) []provider.StreamChunk {
	var out []provider.StreamChunk
	for c := range ch {
		out = append(out, c)
	}
	return out
}

func assertChunkText(t *testing.T, chunks []provider.StreamChunk, want string) {
	t.Helper()
	var sb strings.Builder
	for _, c := range chunks {
		sb.WriteString(c.TextDelta)
	}
	if sb.String() != want {
		t.Errorf("accumulated text=%q, want %q", sb.String(), want)
	}
}

func assertDone(t *testing.T, chunks []provider.StreamChunk, wantStop model.StopReason) {
	t.Helper()
	for _, c := range chunks {
		if c.Done != nil {
			if c.Done.StopReason != wantStop {
				t.Errorf("stop_reason=%q, want %q", c.Done.StopReason, wantStop)
			}
			return
		}
	}
	t.Error("no Done chunk found")
}
