package provider

import (
	"encoding/json"
	"testing"

	"github.com/artpar/gogent/internal/model"
)

func sendChunks(chunks ...StreamChunk) <-chan StreamChunk {
	ch := make(chan StreamChunk, len(chunks))
	for _, c := range chunks {
		ch <- c
	}
	close(ch)
	return ch
}

func TestAccumulateStreamTextOnly(t *testing.T) {
	ch := sendChunks(
		StreamChunk{TextDelta: "Hello, "},
		StreamChunk{TextDelta: "world!"},
		StreamChunk{Done: &StreamDone{
			StopReason: model.StopEndTurn,
			Usage:      model.TokenUsage{InputTokens: 10, OutputTokens: 5},
		}},
	)

	resp, err := AccumulateStream(ch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StopReason != model.StopEndTurn {
		t.Errorf("stop reason: got %q, want %q", resp.StopReason, model.StopEndTurn)
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 5 {
		t.Errorf("usage: got %+v", resp.Usage)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("content length: got %d, want 1", len(resp.Content))
	}
	tp, ok := resp.Content[0].(model.TextPart)
	if !ok {
		t.Fatalf("content[0] type: got %T, want TextPart", resp.Content[0])
	}
	if tp.Text != "Hello, world!" {
		t.Errorf("text: got %q, want %q", tp.Text, "Hello, world!")
	}
}

func TestAccumulateStreamThinkingAndText(t *testing.T) {
	ch := sendChunks(
		StreamChunk{ThinkingDelta: "Let me "},
		StreamChunk{ThinkingDelta: "think..."},
		StreamChunk{ThinkingSignatureDelta: "sig_xyz"},
		StreamChunk{TextDelta: "The answer is 42."},
		StreamChunk{Done: &StreamDone{StopReason: model.StopEndTurn}},
	)

	resp, err := AccumulateStream(ch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Content) != 2 {
		t.Fatalf("content length: got %d, want 2", len(resp.Content))
	}

	// Thinking first
	think, ok := resp.Content[0].(model.ThinkingPart)
	if !ok {
		t.Fatalf("content[0]: got %T, want ThinkingPart", resp.Content[0])
	}
	if think.Text != "Let me think..." {
		t.Errorf("thinking text: got %q", think.Text)
	}
	if think.Signature != "sig_xyz" {
		t.Errorf("thinking signature: got %q, want %q", think.Signature, "sig_xyz")
	}

	// Then text
	text, ok := resp.Content[1].(model.TextPart)
	if !ok {
		t.Fatalf("content[1]: got %T, want TextPart", resp.Content[1])
	}
	if text.Text != "The answer is 42." {
		t.Errorf("text: got %q", text.Text)
	}
}

func TestAccumulateStreamMultiToolCall(t *testing.T) {
	ch := sendChunks(
		StreamChunk{ToolCallStart: &model.ToolCallPart{ID: "tc1", Name: "Bash"}},
		StreamChunk{ToolCallStart: &model.ToolCallPart{ID: "tc2", Name: "Read"}},
		StreamChunk{ToolCallInputDelta: &ToolCallDelta{ToolCallID: "tc1", JSONDelta: `{"cmd":`}},
		StreamChunk{ToolCallInputDelta: &ToolCallDelta{ToolCallID: "tc2", JSONDelta: `{"path":`}},
		StreamChunk{ToolCallInputDelta: &ToolCallDelta{ToolCallID: "tc1", JSONDelta: `"ls"}`}},
		StreamChunk{ToolCallInputDelta: &ToolCallDelta{ToolCallID: "tc2", JSONDelta: `"/tmp"}`}},
		StreamChunk{Done: &StreamDone{StopReason: model.StopToolUse}},
	)

	resp, err := AccumulateStream(ch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StopReason != model.StopToolUse {
		t.Errorf("stop reason: got %q, want %q", resp.StopReason, model.StopToolUse)
	}
	if len(resp.Content) != 2 {
		t.Fatalf("content length: got %d, want 2", len(resp.Content))
	}

	tc1 := resp.Content[0].(model.ToolCallPart)
	if tc1.ID != "tc1" || tc1.Name != "Bash" {
		t.Errorf("tc1: got %+v", tc1)
	}
	var input1 map[string]string
	if err := json.Unmarshal(tc1.Input, &input1); err != nil {
		t.Fatalf("tc1 input unmarshal: %v", err)
	}
	if input1["cmd"] != "ls" {
		t.Errorf("tc1 input: got %v", input1)
	}

	tc2 := resp.Content[1].(model.ToolCallPart)
	if tc2.ID != "tc2" || tc2.Name != "Read" {
		t.Errorf("tc2: got %+v", tc2)
	}
	var input2 map[string]string
	if err := json.Unmarshal(tc2.Input, &input2); err != nil {
		t.Fatalf("tc2 input unmarshal: %v", err)
	}
	if input2["path"] != "/tmp" {
		t.Errorf("tc2 input: got %v", input2)
	}
}

func TestAccumulateStreamError(t *testing.T) {
	ch := sendChunks(
		StreamChunk{TextDelta: "partial"},
		StreamChunk{Error: model.ErrStreamClosed},
	)

	_, err := AccumulateStream(ch)
	if err == nil {
		t.Fatal("expected error")
	}
	if err != model.ErrStreamClosed {
		t.Errorf("error: got %v, want ErrStreamClosed", err)
	}
}

func TestAccumulateStreamNoDone(t *testing.T) {
	ch := sendChunks(
		StreamChunk{TextDelta: "hello"},
	)

	_, err := AccumulateStream(ch)
	if err == nil {
		t.Fatal("expected error for missing Done")
	}
}

func TestAccumulateStreamEmptyWithDone(t *testing.T) {
	ch := sendChunks(
		StreamChunk{Done: &StreamDone{StopReason: model.StopEndTurn}},
	)

	resp, err := AccumulateStream(ch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Content) != 0 {
		t.Errorf("content length: got %d, want 0", len(resp.Content))
	}
	if resp.StopReason != model.StopEndTurn {
		t.Errorf("stop reason: got %q", resp.StopReason)
	}
}
