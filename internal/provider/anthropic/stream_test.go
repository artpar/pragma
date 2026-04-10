package anthropic

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
)

// sseResponse builds a raw SSE byte stream from a list of event data strings.
func sseResponse(events []string) string {
	var buf string
	for _, e := range events {
		buf += fmt.Sprintf("event: message\ndata: %s\n\n", e)
	}
	return buf
}

func newTestProvider(serverURL string) *Provider {
	client := sdk.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(serverURL),
	)
	bus := observe.NewEventBus(100)
	return &Provider{
		client:      client,
		bus:         bus,
		maxRetries:  0,
		idleTimeout: 5 * time.Second,
	}
}

func TestStreamTextOnly(t *testing.T) {
	events := []string{
		`{"type":"message_start","message":{"id":"msg_1","model":"claude-sonnet-4-20250514","content":[],"role":"assistant","stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":", world!"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":10,"output_tokens":5}}`,
		`{"type":"message_stop"}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseResponse(events))
	}))
	defer server.Close()

	p := newTestProvider(server.URL)
	mapper := NewIDMapper()

	stream := p.client.Messages.NewStreaming(context.Background(), sdk.MessageNewParams{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 1024,
		Messages:  []sdk.MessageParam{sdk.NewUserMessage(sdk.NewTextBlock("hi"))},
	})

	ch := p.startStream(context.Background(), stream, mapper, p.bus, "trace", "span")

	resp, err := provider.AccumulateStream(ch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StopReason != model.StopEndTurn {
		t.Errorf("stop reason: got %q", resp.StopReason)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("content length: got %d, want 1", len(resp.Content))
	}
	tp, ok := resp.Content[0].(model.TextPart)
	if !ok {
		t.Fatalf("expected TextPart, got %T", resp.Content[0])
	}
	if tp.Text != "Hello, world!" {
		t.Errorf("text: got %q", tp.Text)
	}
}

func TestStreamToolUse(t *testing.T) {
	events := []string{
		`{"type":"message_start","message":{"id":"msg_2","model":"claude-sonnet-4-20250514","content":[],"role":"assistant","stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_abc","name":"Bash"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"cmd\":"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"ls\"}"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"input_tokens":10,"output_tokens":20}}`,
		`{"type":"message_stop"}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseResponse(events))
	}))
	defer server.Close()

	p := newTestProvider(server.URL)
	mapper := NewIDMapper()

	stream := p.client.Messages.NewStreaming(context.Background(), sdk.MessageNewParams{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 1024,
		Messages:  []sdk.MessageParam{sdk.NewUserMessage(sdk.NewTextBlock("run ls"))},
	})

	ch := p.startStream(context.Background(), stream, mapper, p.bus, "trace", "span")

	resp, err := provider.AccumulateStream(ch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StopReason != model.StopToolUse {
		t.Errorf("stop reason: got %q", resp.StopReason)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("content length: got %d, want 1", len(resp.Content))
	}
	tc, ok := resp.Content[0].(model.ToolCallPart)
	if !ok {
		t.Fatalf("expected ToolCallPart, got %T", resp.Content[0])
	}
	if tc.Name != "Bash" {
		t.Errorf("name: got %q", tc.Name)
	}
	// Verify mapper registered the pair
	if mapper.ToWire(tc.ID) != "toolu_abc" {
		t.Errorf("mapper: got %q, want toolu_abc", mapper.ToWire(tc.ID))
	}
}

func TestStreamThinkingAndText(t *testing.T) {
	events := []string{
		`{"type":"message_start","message":{"id":"msg_3","model":"claude-sonnet-4-20250514","content":[],"role":"assistant","stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Let me think"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig_123"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"The answer is 42"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":10,"output_tokens":30}}`,
		`{"type":"message_stop"}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseResponse(events))
	}))
	defer server.Close()

	p := newTestProvider(server.URL)
	mapper := NewIDMapper()

	stream := p.client.Messages.NewStreaming(context.Background(), sdk.MessageNewParams{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 1024,
		Messages:  []sdk.MessageParam{sdk.NewUserMessage(sdk.NewTextBlock("think"))},
	})

	ch := p.startStream(context.Background(), stream, mapper, p.bus, "trace", "span")

	resp, err := provider.AccumulateStream(ch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Content) != 2 {
		t.Fatalf("content length: got %d, want 2", len(resp.Content))
	}
	// Thinking first
	think, ok := resp.Content[0].(model.ThinkingPart)
	if !ok {
		t.Fatalf("content[0]: expected ThinkingPart, got %T", resp.Content[0])
	}
	if think.Text != "Let me think" {
		t.Errorf("thinking text: got %q", think.Text)
	}
	if think.Signature != "sig_123" {
		t.Errorf("signature: got %q", think.Signature)
	}
	// Then text
	text, ok := resp.Content[1].(model.TextPart)
	if !ok {
		t.Fatalf("content[1]: expected TextPart, got %T", resp.Content[1])
	}
	if text.Text != "The answer is 42" {
		t.Errorf("text: got %q", text.Text)
	}
}

func TestStreamContextCancellation(t *testing.T) {
	// Server that blocks forever
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `event: message
data: {"type":"message_start","message":{"id":"msg_4","model":"test","content":[],"role":"assistant","stop_reason":null,"usage":{"input_tokens":0,"output_tokens":0}}}

`)
		w.(http.Flusher).Flush()
		// Block until request is done
		<-r.Context().Done()
	}))
	defer server.Close()

	p := newTestProvider(server.URL)
	mapper := NewIDMapper()

	ctx, cancel := context.WithCancel(context.Background())

	stream := p.client.Messages.NewStreaming(ctx, sdk.MessageNewParams{
		Model:     "test",
		MaxTokens: 1024,
		Messages:  []sdk.MessageParam{sdk.NewUserMessage(sdk.NewTextBlock("hi"))},
	})

	ch := p.startStream(ctx, stream, mapper, p.bus, "trace", "span")

	// Cancel after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	// Drain channel — should get an error
	var gotError bool
	for chunk := range ch {
		if chunk.Error != nil {
			gotError = true
		}
	}
	if !gotError {
		t.Error("expected error after context cancellation")
	}
}
