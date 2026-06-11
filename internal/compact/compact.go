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
	PostTokenCount      int
	MessagesRemoved     int
}

// ApplyResult atomically applies a compaction result to conversation state.
func ApplyResult(store *app.StateStore, result CompactResult) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	store.Update(func(s *app.AppState) {
		s.Conversation.Messages = result.ReplacementMessages
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
	postTokens := EstimateConversationTokens(replacements)
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
