package compact

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
)

// Sentinel errors for compaction failures.
var (
	ErrTooFewMessages = errors.New("not enough messages to compact (need at least 4)")
	ErrEmptySummary   = errors.New("compaction produced an empty summary")
	ErrCompactionGrew = errors.New("compaction result is larger than original — discarding")
)

// minMessagesToCompact is the minimum number of messages needed for meaningful compaction.
const minMessagesToCompact = 4

// Service performs conversation compaction via LLM summarization.
// It does NOT modify conversation state — callers apply the result atomically.
// This prevents the GitHub issue #40316 bug where retries corrupt state.
type Service struct {
	provider    provider.Provider
	bus         *observe.EventBus
	costTracker *model.CostTracker
	model       string // secondary/fast model for summarization
}

// NewService creates a compaction service.
func NewService(prov provider.Provider, bus *observe.EventBus, ct *model.CostTracker, modelName string) *Service {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &Service{\n\tprovider:\tprov,\n\tbus:\t\tbus,\n\tcostTracker:\tct,\n\tmodel:\t\tmodelName,\n}")
	observe.GlobalTrace("return: &Service{\n\tprovider:\tprovider.WithAccounting(prov, ct, bus),\n\tbus:\t\tbus,\n\tcos...")
	return &Service{
		provider:    provider.WithAccounting(prov, ct, bus),
		bus:         bus,
		costTracker: ct,
		model:       modelName,
	}
}

// CompactResult holds the output of a compaction.
type CompactResult struct {
	Summary             string          // formatted summary text
	ReplacementMessages []model.Message // summary msg to replace conversation
	PreTokenCount       int
	// PostTokenCount estimates the post-compaction conversation: the
	// replacement summary PLUS the pending unanswered prompts that
	// ApplyResult re-appends after it (CMP-001.2 F4 — previously the
	// summary alone, under-reporting whenever a prompt was preserved).
	PostTokenCount  int
	MessagesRemoved int
}

// ApplyResult atomically applies a compaction result to conversation state.
// The summary replaces the history, and the operator's pending unanswered
// prompts — the trailing user messages no assistant reply has answered yet —
// are re-appended verbatim after the summary so they still reach the model:
// the "never compact an unanswered prompt" semantics (CMP-001.2 F3), applied
// uniformly to manual /compact and auto-compaction since CMP-001.2.F3 (the
// manual path previously replaced ALL messages with the summary alone, so an
// operator prompt left unanswered by an errored turn was swallowed when the
// operator then ran /compact). The pending set is derived from the LIVE
// store state inside this single Update — StateStore.Update holds the store
// lock across the callback — so operator input delivered during the
// in-flight summary call survives the replacement (CMP-001.2.F1).
func ApplyResult(store *app.StateStore, result CompactResult) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	store.Update(func(s *app.AppState) {
		pending := PendingUnansweredUserPrompts(s.Conversation.Messages)
		repl := result.ReplacementMessages
		if len(pending) > 0 {
			repl = append(append([]model.Message{}, repl...), pending...)
		}
		s.Conversation.Messages = repl
		s.Conversation.UpdatedAt = time.Now()
	})
}

// Compact summarizes the conversation and returns a CompactResult.
// Use ApplyResult to replace messages in the conversation.
func (s *Service) Compact(ctx context.Context, messages []model.Message, system model.SystemPrompt, customInstructions string) (CompactResult, error) {
	observe.TraceCtx(ctx, "compact", "Service.Compact", "enter")
	defer observe.TraceCtx(ctx, "compact", "Service.Compact", "exit")
	if len(messages) < minMessagesToCompact {
		observe.TraceCtx(ctx, "compact", "Service.Compact", "if: len(messages) < minMessagesToCompact")
		observe.TraceCtx(ctx, "compact", "Service.Compact", "return: CompactResult{}, ErrTooFewMessages")
		return CompactResult{}, ErrTooFewMessages
	}

	start := time.Now()
	preTokens := EstimateConversationTokens(messages)

	s.bus.Emit(observe.CompactionStarted{
		EventHeader:   observe.NewEventHeader("CompactionStarted", "", "", ""),
		PreTokenCount: preTokens,
		BudgetTokens:  MaxOutputTokensForSummary,
		MessageCount:  len(messages),
	})

	trimmed := Microcompact(messages)

	conversationText := SerializeForCompaction(trimmed)

	prompt := CompactPrompt(customInstructions)
	userContent := prompt + "\n\nHere is the conversation to summarize:\n\n" + conversationText

	params := provider.RequestParams{
		Model:     s.model,
		MaxTokens: MaxOutputTokensForSummary,
		Messages: []model.Message{
			{
				ID:        model.NewUUID(),
				Role:      model.RoleUser,
				Content:   []model.ContentPart{model.TextPart{Text: userContent}},
				Timestamp: time.Now(),
			},
		},
		System: model.SystemPrompt{
			Blocks: []model.SystemBlock{{Text: CompactSystemPrompt}},
		},
	}

	response, err := s.provider.Complete(ctx, params)
	if err != nil {
		observe.TraceCtx(ctx, "compact", "Service.Compact", "if: err != nil")
		s.emitFailed("api_error", err.Error())
		observe.TraceCtx(ctx, "compact", "Service.Compact", "return: CompactResult{}, fmt.Errorf(\"compaction API call: %w\", err)")
		return CompactResult{}, fmt.Errorf("compaction API call: %w", err)
	}

	summaryText := extractText(response.Content)
	if summaryText == "" {
		observe.TraceCtx(ctx, "compact", "Service.Compact", "if: summaryText == \"\"")
		s.emitFailed("empty_summary", "model returned no text content")
		observe.TraceCtx(ctx, "compact", "Service.Compact", "return: CompactResult{}, ErrEmptySummary")
		return CompactResult{}, ErrEmptySummary
	}

	formatted := FormatCompactSummary(summaryText)
	if formatted == "" {
		observe.TraceCtx(ctx, "compact", "Service.Compact", "if: formatted == \"\"")
		s.emitFailed("empty_summary", "formatted summary is empty after stripping analysis")
		observe.TraceCtx(ctx, "compact", "Service.Compact", "return: CompactResult{}, ErrEmptySummary")
		return CompactResult{}, ErrEmptySummary
	}

	summaryMsg := model.Message{
		ID:   model.NewUUID(),
		Role: model.RoleUser,
		Content: []model.ContentPart{
			model.TextPart{Text: CompactUserMessage(formatted, false)},
		},
		Timestamp: time.Now(),
		Flags:     model.MessageFlags{IsCompactSummary: true},
	}

	summaryMsg.Content = stripEmptyTextParts(summaryMsg.Content)

	replacements := []model.Message{summaryMsg}
	// CMP-001.2 F4: the acceptance guard and PostTokenCount must measure
	// the conversation that will exist after ApplyResult re-appends the
	// pending unanswered prompts behind the summary — not the summary
	// alone. The pre-F4 guard compared the summary against prefix+tail,
	// so with a multi-prompt unanswered tail (an errored turn plus
	// queued INT-131 input — every trailing user message is re-appended
	// verbatim) a summary larger than the prefix it replaces was
	// accepted, leaving the post-compaction conversation LARGER than
	// the original, and PostTokenCount under-reported the real post-
	// compaction request. Counted from this input snapshot, the tail is
	// the lower bound: ApplyResult re-derives the set from LIVE store
	// state, so input delivered during the summary call is re-appended
	// (and reaches the model) but is not in this count.
	guarded := replacements
	if pending := PendingUnansweredUserPrompts(messages); len(pending) > 0 {
		guarded = append(append([]model.Message{}, replacements...), pending...)
	}
	postTokens := EstimateConversationTokens(guarded)
	if postTokens >= preTokens {
		observe.TraceCtx(ctx, "compact", "Service.Compact", "if: postTokens >= preTokens")
		s.emitFailed("compaction_grew", fmt.Sprintf("post=%d >= pre=%d tokens", postTokens, preTokens))
		observe.TraceCtx(ctx, "compact", "Service.Compact", "return: CompactResult{}, ErrCompactionGrew")
		return CompactResult{}, ErrCompactionGrew
	}

	durationMs := time.Since(start).Milliseconds()
	s.bus.Emit(observe.CompactionCompleted{
		EventHeader:     observe.NewEventHeader("CompactionCompleted", "", "", ""),
		PostTokenCount:  postTokens,
		SummarizedCount: len(messages),
		DurationMs:      durationMs,
	})
	observe.TraceCtx(ctx, "compact", "Service.Compact", "return: CompactResult{\n\tSummary:\t\tformatted,\n\tReplacementMessages:\treplacements,\n\tPre...")

	return CompactResult{
		Summary:             formatted,
		ReplacementMessages: replacements,
		PreTokenCount:       preTokens,
		PostTokenCount:      postTokens,
		MessagesRemoved:     len(messages),
	}, nil
}

func (s *Service) emitFailed(errorType, errorMsg string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	s.bus.Emit(observe.CompactionFailed{
		EventHeader:  observe.NewEventHeader("CompactionFailed", "", "", ""),
		ErrorType:    errorType,
		ErrorMessage: errorMsg,
	})
}

// extractText concatenates all TextPart values from content parts.
func extractText(parts []model.ContentPart) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var b []byte
	for _, part := range parts {
		observe.GlobalTrace("range parts")
		if tp, ok := part.(model.TextPart); ok {
			observe.GlobalTrace("if: ok")
			b = append(b, tp.Text...)
		}
	}
	observe.GlobalTrace("return: string(b)")
	return string(b)
}

// stripEmptyTextParts removes TextParts with empty text from content.
// Defense against #41992 where empty text blocks corrupt sessions.
func stripEmptyTextParts(parts []model.ContentPart) []model.ContentPart {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	result := make([]model.ContentPart, 0, len(parts))
	for _, part := range parts {
		observe.GlobalTrace("range parts")
		if tp, ok := part.(model.TextPart); ok && tp.Text == "" {
			observe.GlobalTrace("if: ok && tp.Text == \"\"")
			continue
		}
		result = append(result, part)
	}
	observe.GlobalTrace("return: result")
	return result
}
