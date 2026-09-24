package compact

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

// CMP-001.2 F4 gates: the pending tail — the trailing unanswered user
// prompts that ApplyResult re-appends verbatim after the summary (an
// errored turn leaves its prompt unanswered and INT-131 mid-turn input
// appends further user messages behind it) — must be INSIDE the
// compaction acceptance guard and the reported post-compaction token
// count. Before F4 both were computed on [summary] alone, so a summary
// larger than the message prefix it replaces was accepted whenever the
// pending tail dwarfed the prefix (the guard compared the summary
// against prefix+tail), and PostTokenCount under-reported the real
// post-compaction conversation whenever any prompt was preserved.

func compactPendingTailMessages(prefixText, pendingA, pendingB string) []model.Message {
	msg := func(role model.Role, text string) model.Message {
		return model.Message{
			ID:        model.NewUUID(),
			Role:      role,
			Content:   []model.ContentPart{model.TextPart{Text: text}},
			Timestamp: time.Now(),
		}
	}
	return []model.Message{
		msg(model.RoleUser, prefixText),
		msg(model.RoleAssistant, prefixText),
		msg(model.RoleUser, prefixText),
		msg(model.RoleAssistant, prefixText),
		// Unanswered prompt: the turn errored (provider failure /
		// truncation) before any assistant reply was appended.
		msg(model.RoleUser, pendingA),
		// INT-131 mid-turn input queued behind it: also a trailing
		// user message no assistant has answered.
		msg(model.RoleUser, pendingB),
	}
}

// TestCompactGrewGuardCoversReappendedPendingTail: the summary here is
// far larger than the tiny prefix it would replace but far smaller than
// prefix+tail — the pre-F4 guard compared the summary alone against
// prefix+tail and accepted a "compaction" that leaves the conversation
// LARGER than the original (summary+tail > prefix+tail).
func TestCompactGrewGuardCoversReappendedPendingTail(t *testing.T) {
	prov := &replayProvider{
		response: model.Response{
			Content: []model.ContentPart{model.TextPart{Text: "<summary>\n" +
				strings.Repeat("summary word ", 40) + "\n</summary>"}},
			StopReason: model.StopEndTurn,
		},
	}
	bus := observe.NewEventBus(64)
	ct := model.NewCostTracker(0)
	svc := NewService(prov, bus, ct, "claude-haiku")

	msgs := compactPendingTailMessages(
		"tiny",
		strings.Repeat("tail ", 300),
		strings.Repeat("queued ", 300))

	_, err := svc.Compact(context.Background(), msgs, model.SystemPrompt{}, "")
	if !errors.Is(err, ErrCompactionGrew) {
		pending := PendingUnansweredUserPrompts(msgs)
		pre := EstimateConversationTokens(msgs)
		tail := EstimateConversationTokens(pending)
		t.Fatalf("expected ErrCompactionGrew: the summary replaces only a %d-token prefix while the re-appended pending tail adds %d tokens back, so the result grows the %d-token conversation — Compact accepted it (err=%v)",
			pre-tail, tail, pre, err)
	}
	bus.Drain()
}

// TestCompactPostTokensIncludeReappendedPendingTail: a genuinely
// shrinking compaction (summary far below the prefix) must still
// REPORT the post-compaction size including the re-appended tail —
// PostTokenCount is what CompactionEvent and the manual /compact
// display carry.
func TestCompactPostTokensIncludeReappendedPendingTail(t *testing.T) {
	prov := &replayProvider{
		response: model.Response{
			Content:    []model.ContentPart{model.TextPart{Text: "<summary>\nsmall summary of the work\n</summary>"}},
			StopReason: model.StopEndTurn,
		},
	}
	bus := observe.NewEventBus(64)
	ct := model.NewCostTracker(0)
	svc := NewService(prov, bus, ct, "claude-haiku")

	msgs := compactPendingTailMessages(
		strings.Repeat("history ", 400),
		strings.Repeat("pending ", 100),
		strings.Repeat("queued ", 100))

	result, err := svc.Compact(context.Background(), msgs, model.SystemPrompt{}, "")
	if err != nil {
		t.Fatalf("Compact() error: %v", err)
	}

	pending := PendingUnansweredUserPrompts(msgs)
	if len(pending) != 2 {
		t.Fatalf("pending tail = %d messages, want the 2 unanswered prompts", len(pending))
	}
	// The exact conversation ApplyResult will leave: the replacement
	// (summary) plus the re-appended pending tail.
	expected := EstimateConversationTokens(append(append([]model.Message{}, result.ReplacementMessages...), pending...))
	if result.PostTokenCount != expected {
		t.Fatalf("PostTokenCount = %d, want %d (summary + re-appended pending tail) — the count reports the summary alone (CMP-001.2 F4)",
			result.PostTokenCount, expected)
	}
	if result.PostTokenCount >= result.PreTokenCount {
		t.Fatalf("PostTokenCount (%d) must stay below PreTokenCount (%d): the compaction still shrinks the conversation",
			result.PostTokenCount, result.PreTokenCount)
	}
	bus.Drain()
}
