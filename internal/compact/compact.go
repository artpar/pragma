package compact

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
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
	return &Service{
		provider:    prov,
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

// Compact summarizes the conversation and returns a CompactResult.
// The caller is responsible for replacing messages in the conversation.
func (s *Service) Compact(ctx context.Context, messages []model.Message, system model.SystemPrompt, customInstructions string) (CompactResult, error) {
	if len(messages) < minMessagesToCompact {
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

	// Step 1: Microcompact — lossless trim (50-70% reduction, #27293)
	trimmed := Microcompact(messages)

	// Step 2: Serialize to text
	conversationText := SerializeForCompaction(trimmed)

	// Step 3: Build compaction prompt
	prompt := CompactPrompt(customInstructions)
	userContent := prompt + "\n\nHere is the conversation to summarize:\n\n" + conversationText

	// Step 4: Call LLM for summarization
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
		s.emitFailed("api_error", err.Error())
		return CompactResult{}, fmt.Errorf("compaction API call: %w", err)
	}

	// Step 5: Record cost (#43945: compaction calls must be tracked)
	if pricing, known := s.provider.Pricing(s.model); known {
		s.costTracker.Record(s.model, s.provider.Name(), response.Usage, pricing)
	}

	// Step 6: Extract summary text
	summaryText := extractText(response.Content)
	if summaryText == "" {
		s.emitFailed("empty_summary", "model returned no text content")
		return CompactResult{}, ErrEmptySummary
	}

	// Step 7: Format summary — strips <analysis>, extracts <summary>
	formatted := FormatCompactSummary(summaryText)
	if formatted == "" {
		s.emitFailed("empty_summary", "formatted summary is empty after stripping analysis")
		return CompactResult{}, ErrEmptySummary
	}

	// Step 8: Build replacement message
	summaryMsg := model.Message{
		ID:   model.NewUUID(),
		Role: model.RoleUser,
		Content: []model.ContentPart{
			model.TextPart{Text: CompactUserMessage(formatted, false)},
		},
		Timestamp: time.Now(),
		Flags:     model.MessageFlags{IsCompactSummary: true},
	}

	// Step 9: Strip empty text parts (ADR-018 defense, #41992)
	summaryMsg.Content = stripEmptyTextParts(summaryMsg.Content)

	// Step 10: Validate — post-compact must be smaller than pre-compact
	replacements := []model.Message{summaryMsg}
	postTokens := EstimateConversationTokens(replacements)
	if postTokens >= preTokens {
		s.emitFailed("compaction_grew", fmt.Sprintf("post=%d >= pre=%d tokens", postTokens, preTokens))
		return CompactResult{}, ErrCompactionGrew
	}

	durationMs := time.Since(start).Milliseconds()
	s.bus.Emit(observe.CompactionCompleted{
		EventHeader:     observe.NewEventHeader("CompactionCompleted", "", "", ""),
		PostTokenCount:  postTokens,
		SummarizedCount: len(messages),
		DurationMs:      durationMs,
	})

	return CompactResult{
		Summary:             formatted,
		ReplacementMessages: replacements,
		PreTokenCount:       preTokens,
		PostTokenCount:      postTokens,
		MessagesRemoved:     len(messages),
	}, nil
}

func (s *Service) emitFailed(errorType, errorMsg string) {
	s.bus.Emit(observe.CompactionFailed{
		EventHeader:  observe.NewEventHeader("CompactionFailed", "", "", ""),
		ErrorType:    errorType,
		ErrorMessage: errorMsg,
	})
}

// extractText concatenates all TextPart values from content parts.
func extractText(parts []model.ContentPart) string {
	var b []byte
	for _, part := range parts {
		if tp, ok := part.(model.TextPart); ok {
			b = append(b, tp.Text...)
		}
	}
	return string(b)
}

// stripEmptyTextParts removes TextParts with empty text from content.
// Defense against #41992 where empty text blocks corrupt sessions.
func stripEmptyTextParts(parts []model.ContentPart) []model.ContentPart {
	result := make([]model.ContentPart, 0, len(parts))
	for _, part := range parts {
		if tp, ok := part.(model.TextPart); ok && tp.Text == "" {
			continue
		}
		result = append(result, part)
	}
	return result
}
