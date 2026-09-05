package openrouter_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/provider/openrouter"
	"github.com/artpar/pragma/internal/session"
)

// Replay one authentic response, then stop at the changed request boundary.
// No recorded downstream model answer is treated as a counterfactual result.
func TestRecordedReasoningSurvivesSessionReplay(t *testing.T) {
	raw, err := os.ReadFile("testdata/recorded-reasoning-response.json")
	if err != nil {
		t.Fatal(err)
	}
	var recorded struct {
		Choices []struct{ Message map[string]json.RawMessage }
	}
	if err := json.Unmarshal(raw, &recorded); err != nil {
		t.Fatal(err)
	}
	ctxRaw, err := os.ReadFile("testdata/recorded-reasoning-context.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Model      string
		User       struct{ Content string }
		ToolResult struct {
			Content    string
			ToolCallID string `json:"tool_call_id"`
		} `json:"tool_result"`
	}
	if err := json.Unmarshal(ctxRaw, &fixture); err != nil {
		t.Fatal(err)
	}
	var requests []struct{ Messages []map[string]json.RawMessage }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Messages []map[string]json.RawMessage }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		requests = append(requests, req)
		if len(requests) == 1 {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(raw)
			return
		}
		// Boundary marker, not a simulated successful provider response. 400 is nonretryable.
		http.Error(w, "recorded replay ends at outgoing request", http.StatusBadRequest)
	}))
	defer server.Close()
	p, err := openrouter.New("replay-no-credential", observe.NewEventBus(64), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	params := provider.RequestParams{Model: fixture.Model, MaxTokens: 16384,
		Messages: []model.Message{{ID: "user", Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: fixture.User.Content}}}},
	}
	response, err := p.Complete(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if response.StopReason != model.StopToolUse {
		t.Fatalf("stop reason = %s", response.StopReason)
	}
	params.Messages = append(params.Messages,
		model.Message{ID: "assistant", Role: model.RoleAssistant, Content: response.Content},
		model.Message{ID: "tool", Role: model.RoleUser, Content: []model.ContentPart{model.ToolResultPart{ToolCallID: fixture.ToolResult.ToolCallID, Content: fixture.ToolResult.Content}}},
	)
	path := filepath.Join(t.TempDir(), "replay.jsonl")
	writer, err := session.NewWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if err := writer.WriteHeader(session.HeaderData{SessionID: "replay", Model: fixture.Model, Provider: "openrouter"}); err != nil {
		t.Fatal(err)
	}
	for _, m := range params.Messages {
		if err := writer.WriteMessage(m); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := (&session.Store{}).LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	copied := restored.Conversation.DeepCopy()
	params.Messages = copied.Messages
	if _, err := p.Complete(context.Background(), params); err == nil {
		t.Fatal("replay must stop at request boundary")
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
	messages := requests[1].Messages
	if len(messages) != 3 {
		t.Fatalf("messages = %d, want 3", len(messages))
	}
	for _, field := range []string{"reasoning", "reasoning_details"} {
		wantRaw := recorded.Choices[0].Message[field]
		if len(wantRaw) == 0 {
			t.Fatalf("fixture missing %s", field)
		}
		gotRaw := messages[1][field]
		if len(gotRaw) == 0 {
			t.Errorf("recorded %s lost after response -> JSONL -> reload -> next request", field)
			continue
		}
		var want, got any
		if err := json.Unmarshal(wantRaw, &want); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(gotRaw, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(want, got) {
			t.Errorf("%s changed: got %s want %s", field, gotRaw, wantRaw)
		}
		for _, index := range []int{0, 2} {
			if len(messages[index][field]) != 0 {
				t.Errorf("%s leaked to message %d", field, index)
			}
		}
	}
	var calls []struct {
		ID       string
		Function struct{ Name, Arguments string }
	}
	if err := json.Unmarshal(messages[1]["tool_calls"], &calls); err != nil {
		t.Fatal(err)
	}
	var originalCalls []struct {
		ID       string
		Function struct{ Name, Arguments string }
	}
	if err := json.Unmarshal(recorded.Choices[0].Message["tool_calls"], &originalCalls); err != nil {
		t.Fatal(err)
	}
	if len(calls) != len(originalCalls) {
		t.Fatal("tool call count changed")
	}
	for i, call := range calls {
		original := originalCalls[i]
		if call.ID != original.ID || call.Function.Name != original.Function.Name {
			t.Fatal("tool call identity changed")
		}
		var gotArgs, wantArgs any
		if err := json.Unmarshal([]byte(call.Function.Arguments), &gotArgs); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(original.Function.Arguments), &wantArgs); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(gotArgs, wantArgs) {
			t.Fatal("tool call arguments changed")
		}
	}
	var toolID, toolText string
	if err := json.Unmarshal(messages[2]["tool_call_id"], &toolID); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(messages[2]["content"], &toolText); err != nil {
		t.Fatal(err)
	}
	if toolID != fixture.ToolResult.ToolCallID || toolText != fixture.ToolResult.Content {
		t.Fatal("tool result changed")
	}
}
