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

// CLK-001/CLK-002 gates: every user-role message the provider-tools loop
// appends carries its append wall-clock time, visible to the model on the
// request messages. The stamp rides as a first line on text-only appends
// (prompt, notices); the results message itself stays tool-results-only;
// the per-request clock rides in a dynamic system block, NEVER as a
// stamp-only user message (the CLK-001 companion is banned, CLK-002);
// assistant messages are unstamped; pragma loop mode carries no stamps and
// no clock block.

// wallClockStampMarkerText mirrors the production marker as a literal so the
// gate compiles on the pre-change baseline and fails at runtime, not at
// compile time.
const wallClockStampMarkerText = "[pragma wall-clock "

// wallClockSystemBlockBody mirrors the production per-request clock block
// body (CLK-002) as a literal so these gates compile on the pre-change
// baseline and fail at runtime, not at compile time. systemWithWallClock
// must keep its wording in sync with this literal or the gates fail.
const wallClockSystemBlockBody = "The current wall-clock time as this request was built. User-role messages appended by the harness carry the same stamp as the first line of their text at the time they were added; tool-results messages carry no text (a text part would serialize before the tool results and break pairing), so a results batch is timed by this block on the request that follows it."

// wallClockSystemBlockText builds the mirrored per-request clock block
// (CLK-002) for a given stamp time.
func wallClockSystemBlockText(now time.Time) string {
	return "# Wall clock\n\n" + wallClockStamp(now) + "\n\n" + wallClockSystemBlockBody
}

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

// systemWallClockStamps returns the wall-clock stamps carried by a
// request's system prompt blocks (the per-request clock block, CLK-002).
func systemWallClockStamps(system model.SystemPrompt) []time.Time {
	var stamps []time.Time
	for _, block := range system.Blocks {
		for _, line := range strings.Split(block.Text, "\n") {
			if ts, ok := parseWallClockStamp(line); ok {
				stamps = append(stamps, ts)
			}
		}
	}
	return stamps
}

// assertNoStampOnlyUserMessages enforces the CLK-002 ban: no user message
// whose entire text content is a bare wall-clock stamp — the operator
// directive forbids user messages that exist only to carry a timestamp.
func assertNoStampOnlyUserMessages(t *testing.T, msgs []model.Message, where string) {
	t.Helper()
	for i, msg := range msgs {
		if msg.Role != model.RoleUser {
			continue
		}
		var texts []string
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok {
				texts = append(texts, tp.Text)
			}
		}
		if len(texts) == 0 {
			continue
		}
		joined := strings.Join(texts, "\n")
		if _, stampOnly := parseWallClockStamp(joined); stampOnly {
			t.Fatalf("%s message %d is a stamp-only user message (CLK-002 ban): %q", where, i, joined)
		}
	}
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
			Content:    []model.ContentPart{model.TextPart{Text: "done"}},
			StopReason: model.StopEndTurn,
		},
	}
}

func TestProviderToolsLoopWallClockStampsTextAppendsAndRequestSystem(t *testing.T) {
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

	// CLK-002: every request's system carries the per-request clock block —
	// exactly one wall-clock stamp, carried as a block, never a user message.
	for i, rp := range prov.requests {
		sysStamps := systemWallClockStamps(rp.System)
		if len(sysStamps) != 1 {
			t.Fatalf("request %d system carries %d wall-clock stamps, want exactly 1 (the per-request clock block, CLK-002)", i, len(sysStamps))
		}
		want := wallClockSystemBlockText(sysStamps[0])
		var blockText string
		for _, block := range rp.System.Blocks {
			if strings.Contains(block.Text, wallClockStampMarkerText) {
				blockText = block.Text
				break
			}
		}
		if blockText != want {
			t.Fatalf("request %d clock block = %q, want %q", i, blockText, want)
		}
	}

	// Request 1: prompt, assistant(tool call), user(results-only) — and NO
	// stamp-only user message anywhere (the CLK-001 companion is banned).
	req1 := prov.requests[1].Messages
	if len(req1) != 3 {
		t.Fatalf("request 1 carries %d messages, want 3 (prompt, assistant, results) — no stamp-only companion (CLK-002)", len(req1))
	}
	if req1[1].Role != model.RoleAssistant {
		t.Fatalf("request 1 message 1 role = %q, want assistant", req1[1].Role)
	}
	if req1[2].Role != model.RoleUser {
		t.Fatalf("request 1 tail role = %q, want user", req1[2].Role)
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

	// No user message anywhere in the request — or in the persisted
	// conversation, which session files serialize 1:1 — is stamp-only.
	assertNoStampOnlyUserMessages(t, req1, "request 1")
	assertNoStampOnlyUserMessages(t, engine.store.Snapshot().Conversation.Messages, "persisted conversation")

	// Monotonic: the prompt stamp must not postdate the request-0 clock
	// block, and the clock must not run backwards across requests.
	sys0 := systemWallClockStamps(prov.requests[0].System)[0]
	sys1 := systemWallClockStamps(prov.requests[1].System)[0]
	if promptStamp.After(sys0) {
		t.Fatalf("prompt stamp %v is after request-0 clock %v", promptStamp, sys0)
	}
	if sys0.After(sys1) {
		t.Fatalf("clock ran backwards across requests: %v then %v", sys0, sys1)
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
	// only the prompt prefix (the per-turn clock rides in the system block).
	if n := countWallClockStamps(req1); n != 1 {
		t.Fatalf("request 1 carries %d stamps, want 1 (the prompt prefix; the clock block is not a user message)", n)
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
		if n := len(systemWallClockStamps(req.System)); n != 0 {
			t.Fatalf("pragma-mode request %d system carries %d wall-clock stamps, want 0 — the clock block lives in the provider-tools loop only", i, n)
		}
	}
}
