package query

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

// CLK-001 gates: every user-role message the provider-tools loop appends
// carries its append wall-clock time, visible to the model on the request
// messages. The stamp rides as a first line on text-only appends (prompt,
// turn-budget notice) and as a companion user message after a
// tool-results batch; the results message itself stays tool-results-only;
// assistant messages are unstamped; pragma loop mode carries no stamps.

// wallClockStampMarkerText mirrors the production marker as a literal so the
// gate compiles on the pre-change baseline and fails at runtime, not at
// compile time.
const wallClockStampMarkerText = "[pragma wall-clock "

var wallClockStampLine = regexp.MustCompile(`^\[pragma wall-clock (\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2}))\]$`)

// parseWallClockStamp parses a stamp-only text ("[pragma wall-clock <RFC3339>]").
func parseWallClockStamp(text string) (time.Time, bool) {
	m := wallClockStampLine.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return time.Time{}, false
	}
	ts, err := time.Parse(time.RFC3339, m[1])
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
}

// splitWallClockPrefix splits "<stamp>\n<body>" text (prompt/notice shape).
func splitWallClockPrefix(text string) (time.Time, string, bool) {
	line, rest, found := strings.Cut(text, "\n")
	ts, ok := parseWallClockStamp(line)
	if !ok || !found {
		return time.Time{}, text, false
	}
	return ts, rest, true
}

func wallClockStampedResponses() []model.Response {
	bashInput, err := json.Marshal(map[string]string{"cmd": "true"})
	if err != nil {
		panic(err)
	}
	return []model.Response{
		{
			Content: []model.ContentPart{
				model.ToolCallPart{ID: "call-wallclock-a", Name: "Bash", Input: bashInput},
			},
			StopReason: model.StopToolUse,
		},
		{
			Content: []model.ContentPart{model.TextPart{Text: "done"}},
			StopReason: model.StopEndTurn,
		},
	}
}

func TestProviderToolsLoopWallClockStampsOnAppendedUserMessages(t *testing.T) {
	prov := &pragmaLoopTestProvider{responses: wallClockStampedResponses()}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          t.TempDir(),
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	engine := NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(64), EngineConfig{
		Model:     "test-model",
		LoopMode:  LoopModeProviderTools,
		MaxTokens: 4096,
		MaxTurns:  5,
	})

	collectPragmaLoopEvents(engine.Run(t.Context(), "work"))

	// Sanity: the loop must complete both turns — a failure below must be
	// about the stamps, not a broken loop.
	if prov.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", prov.calls)
	}

	// Request 0 carries the prompt: a single user message whose text part
	// begins with the stamp line, followed by the operator text.
	req0 := prov.requests[0].Messages
	if len(req0) != 1 || req0[0].Role != model.RoleUser {
		t.Fatalf("request 0 carries %d messages (first role %q), want a single user prompt", len(req0), req0[0].Role)
	}
	tp, ok := req0[0].Content[0].(model.TextPart)
	if !ok {
		t.Fatalf("prompt content[0] is %T, want model.TextPart", req0[0].Content[0])
	}
	promptStamp, rest, ok := splitWallClockPrefix(tp.Text)
	if !ok {
		t.Fatalf("prompt text = %q, want wall-clock stamp first line", tp.Text)
	}
	if rest != "work" {
		t.Fatalf("prompt body = %q, want %q", rest, "work")
	}

	// Request 1: prompt, assistant(tool call), user(results-only),
	// user(stamp companion).
	req1 := prov.requests[1].Messages
	if len(req1) != 4 {
		t.Fatalf("request 1 carries %d messages, want 4 (prompt, assistant, results, companion)", len(req1))
	}
	if req1[1].Role != model.RoleAssistant {
		t.Fatalf("request 1 message 1 role = %q, want assistant", req1[1].Role)
	}
	if req1[2].Role != model.RoleUser || req1[3].Role != model.RoleUser {
		t.Fatalf("request 1 tail roles = %q,%q, want user,user", req1[2].Role, req1[3].Role)
	}

	// The results message stays tool-results-only: every part is a tool
	// result paired to the outstanding call, no text part mixed in.
	resultsMsg := req1[2]
	if len(resultsMsg.Content) != 1 {
		t.Fatalf("results message carries %d parts, want 1", len(resultsMsg.Content))
	}
	rp, ok := resultsMsg.Content[0].(model.ToolResultPart)
	if !ok {
		t.Fatalf("results content[0] is %T, want model.ToolResultPart", resultsMsg.Content[0])
	}
	if rp.ToolCallID != "call-wallclock-a" {
		t.Fatalf("results ToolCallID = %q, want call-wallclock-a", rp.ToolCallID)
	}

	// The companion carries exactly the stamp.
	companion := req1[3]
	if len(companion.Content) != 1 {
		t.Fatalf("companion carries %d parts, want 1", len(companion.Content))
	}
	ctp, ok := companion.Content[0].(model.TextPart)
	if !ok {
		t.Fatalf("companion content[0] is %T, want model.TextPart", companion.Content[0])
	}
	companionStamp, ok := parseWallClockStamp(ctp.Text)
	if !ok {
		t.Fatalf("companion text = %q, want wall-clock stamp only", ctp.Text)
	}

	// Monotonic: the prompt stamp must not postdate the results companion.
	if promptStamp.After(companionStamp) {
		t.Fatalf("prompt stamp %v is after companion stamp %v", promptStamp, companionStamp)
	}

	// Assistant messages carry no stamps.
	for _, msg := range req1 {
		if msg.Role != model.RoleAssistant {
			continue
		}
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok && strings.Contains(tp.Text, wallClockStampMarkerText) {
				t.Fatalf("assistant message carries a stamp: %q", tp.Text)
			}
		}
	}

	// Exactly one stamp per engine-appended user message: request 1 carries
	// the prompt prefix and the companion.
	if n := countWallClockStamps(req1); n != 2 {
		t.Fatalf("request 1 carries %d stamps, want 2 (prompt + companion)", n)
	}
}

func countWallClockStamps(msgs []model.Message) int {
	count := 0
	for _, msg := range msgs {
		if msg.Role != model.RoleUser {
			continue
		}
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok && strings.Contains(tp.Text, wallClockStampMarkerText) {
				count++
			}
		}
	}
	return count
}

// Adjacent mode check: the pragma loop appends its own messages without
// stamps — the mechanism lives in the provider-tools loop only.
func TestPragmaLoopModeCarriesNoWallClockStamps(t *testing.T) {
	responses := []model.Response{
		{
			ID:         "resp-pragma-wallclock",
			Model:      "test-model",
			StopReason: model.StopEndTurn,
			Content: []model.ContentPart{
				model.TextPart{Text: "```bash\necho COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT\n```"},
			},
		},
	}
	prov := &pragmaLoopTestProvider{responses: responses}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          conv.WorkDir,
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	engine := NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(16), EngineConfig{
		Model:     "test-model",
		MaxTokens: 4096,
		MaxTurns:  5,
	})

	events := collectPragmaLoopEvents(engine.RunPragmaLoopWithSystemCompletionCheck(
		t.Context(),
		model.SystemPrompt{},
		"write the report",
		func() (bool, string, error) { return true, "", nil },
	))
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			t.Fatalf("unexpected ErrorEvent: %v", e.Err)
		}
	}
	if prov.calls == 0 {
		t.Fatal("pragma loop made no provider calls; absence check is vacuous")
	}
	for i, req := range prov.requests {
		if n := countWallClockStamps(req.Messages); n != 0 {
			t.Fatalf("pragma-mode request %d carries %d stamps, want 0", i, n)
		}
	}
}
