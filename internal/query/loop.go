package query

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/compact"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
)

// continuationPrompt is sent when the model returns StopPauseTurn,
// indicating it wants to continue but hit a turn-level limit.
const continuationPrompt = "Please continue."

// Run starts the agentic loop in a goroutine and returns a channel of LoopEvents.
// The channel is closed when the loop finishes.
// Implements SPEC.md §6.1.
func (e *Engine) Run(ctx context.Context, userMessage string) <-chan LoopEvent {
	ch := make(chan LoopEvent, 16)
	go func() {
		defer close(ch)
		defer func() {
			if r := recover(); r != nil {
				ch <- ErrorEvent{Err: fmt.Errorf("query loop panic: %v", r)}
			}
		}()
		e.runLoop(ctx, userMessage, ch)
	}()
	return ch
}

func (e *Engine) runLoop(ctx context.Context, userMessage string, ch chan<- LoopEvent) {
	maxTurns := e.config.MaxTurns
	if maxTurns <= 0 {
		maxTurns = DefaultMaxTurns
	}

	// 1. Create and append user message
	userMsg := model.Message{
		ID:        model.NewUUID(),
		Role:      model.RoleUser,
		Content:   []model.ContentPart{model.TextPart{Text: userMessage}},
		Timestamp: time.Now(),
	}
	e.store.Update(func(s *app.AppState) {
		s.Conversation.Append(userMsg)
	})

	// 2. LOOP
	for range maxTurns {
		if err := ctx.Err(); err != nil {
			ch <- ErrorEvent{Err: fmt.Errorf("context cancelled: %w", model.ErrContextCancelled)}
			return
		}

		snap := e.store.Snapshot()

		// a. Build RequestParams
		params := provider.RequestParams{
			Model:       e.config.Model,
			MaxTokens:   e.config.MaxTokens,
			Messages:    snap.Conversation.APIMessages(),
			System:      snap.Conversation.System,
			Tools:       e.registry.ToolDefs(),
			Temperature: e.config.Temperature,
			Thinking:    e.config.Thinking,
		}

		// b. Stream — provider emits its own APIRequestStarted/Completed/Failed events
		chunks, err := e.provider.Stream(ctx, params)
		if err != nil {
			ch <- ErrorEvent{Err: err}
			return
		}

		// c. Consume stream: emit deltas as they arrive, accumulate into Response
		response, err := e.consumeStream(chunks, ch)
		if err != nil {
			ch <- ErrorEvent{Err: err}
			return
		}

		// d+e. Build assistant message, append to conversation
		assistantMsg := model.Message{
			ID:        model.NewUUID(),
			Role:      model.RoleAssistant,
			Content:   response.Content,
			Timestamp: time.Now(),
		}
		e.store.Update(func(s *app.AppState) {
			s.Conversation.Append(assistantMsg)
		})

		// f. Record cost
		pricing, known := e.provider.Pricing(e.config.Model)
		if !known {
			e.bus.Emit(observe.ErrorOccurred{
				EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
				Severity:     "warn",
				Component:    "query",
				ErrorType:    "unknown_model_pricing",
				ErrorMessage: fmt.Sprintf("no pricing data for model %q, costs will be zero", e.config.Model),
			})
		}
		e.costTracker.Record(e.config.Model, e.provider.Name(), response.Usage, pricing)

		// Auto-compaction check (between cost recording and stop reason switch).
		// Only runs if compactor is set (nil for subagent engines, #27794).
		if e.compactor != nil && e.autoTracker != nil {
			compSnap := e.store.Snapshot()
			tokenCount := compact.EstimateConversationTokens(compSnap.Conversation.APIMessages())
			if e.autoTracker.ShouldAutoCompact(tokenCount, e.windowConfig) {
				compResult, compErr := e.compactor.Compact(ctx, compSnap.Conversation.APIMessages(), compSnap.Conversation.System, "")
				if compErr != nil && ctx.Err() == nil {
					// Only count real failures, not context cancellation (Ctrl+C)
					tripped := e.autoTracker.RecordFailure()
					if tripped {
						e.bus.Emit(observe.ErrorOccurred{
							EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
							Severity:     "warn",
							Component:    "compact",
							ErrorType:    "circuit_breaker_tripped",
							ErrorMessage: fmt.Sprintf("auto-compaction disabled after %d consecutive failures", compact.MaxConsecutiveFailures),
						})
					}
				} else {
					e.autoTracker.RecordSuccess()
					e.store.Update(func(s *app.AppState) {
						s.Conversation.Messages = compResult.ReplacementMessages
						s.Conversation.UpdatedAt = time.Now()
					})
					ch <- CompactionEvent{PreTokens: compResult.PreTokenCount, PostTokens: compResult.PostTokenCount}
				}
			}
			e.autoTracker.IncrementTurn()
		}

		// g+h. Switch on StopReason
		switch response.StopReason {
		case model.StopEndTurn, model.StopMaxTokens:
			ch <- TurnCompleteEvent{Response: response, StopReason: response.StopReason}
			return

		case model.StopPauseTurn:
			// Model wants to continue — send continuation message (ADR-010)
			contMsg := model.Message{
				ID:        model.NewUUID(),
				Role:      model.RoleUser,
				Content:   []model.ContentPart{model.TextPart{Text: continuationPrompt}},
				Timestamp: time.Now(),
			}
			e.store.Update(func(s *app.AppState) {
				s.Conversation.Append(contMsg)
			})
			continue

		case model.StopToolUse:
			toolCalls := extractToolCalls(response.Content)
			for _, tc := range toolCalls {
				ch <- ToolCallEvent{Call: tc}
			}

			execResult := e.orchestrator.Execute(ctx, toolCalls, snap)
			for _, r := range execResult.Results {
				ch <- ToolResultEvent{Result: r}
			}

			// Build user message with tool results + any supplements (e.g., PDF document blocks)
			resultParts := make([]model.ContentPart, 0, len(execResult.Results)+len(execResult.Supplements))
			for _, r := range execResult.Results {
				resultParts = append(resultParts, r)
			}
			resultParts = append(resultParts, execResult.Supplements...)
			resultMsg := model.Message{
				ID:        model.NewUUID(),
				Role:      model.RoleUser,
				Content:   resultParts,
				Timestamp: time.Now(),
			}
			e.store.Update(func(s *app.AppState) {
				s.Conversation.Append(resultMsg)
			})
			continue

		default:
			ch <- ErrorEvent{Err: fmt.Errorf("unknown stop reason: %s", response.StopReason)}
			return
		}
	}
	// Loop exhausted maxTurns without a terminal stop reason
	ch <- ErrorEvent{Err: fmt.Errorf("agentic loop exceeded maximum of %d turns", maxTurns)}
}

// toolAccumulator collects streaming fragments for a single tool call.
type toolAccumulator struct {
	id       string
	name     string
	inputBuf strings.Builder
}

// consumeStream reads all chunks from a streaming response, emitting TextEvent
// and ThinkingEvent deltas as they arrive, and returns the accumulated Response.
func (e *Engine) consumeStream(
	chunks <-chan provider.StreamChunk,
	ch chan<- LoopEvent,
) (model.Response, error) {
	var textBuf strings.Builder
	var thinkBuf strings.Builder
	var thinkSigBuf strings.Builder
	var redactedThinkingParts []model.ThinkingPart
	toolCalls := make(map[string]*toolAccumulator)
	var toolOrder []string
	var done *provider.StreamDone

	for chunk := range chunks {
		if chunk.Error != nil {
			return model.Response{}, chunk.Error
		}

		if chunk.TextDelta != "" {
			textBuf.WriteString(chunk.TextDelta)
			ch <- TextEvent{Text: chunk.TextDelta}
		}

		if chunk.ThinkingDelta != "" {
			thinkBuf.WriteString(chunk.ThinkingDelta)
			ch <- ThinkingEvent{Text: chunk.ThinkingDelta}
		}

		if chunk.ThinkingSignatureDelta != "" {
			thinkSigBuf.WriteString(chunk.ThinkingSignatureDelta)
		}

		if chunk.RedactedThinkingBlock != nil {
			redactedThinkingParts = append(redactedThinkingParts, model.ThinkingPart{
				Redacted:     true,
				RedactedData: chunk.RedactedThinkingBlock.Data,
			})
		}

		if chunk.ToolCallStart != nil {
			tc := chunk.ToolCallStart
			if _, exists := toolCalls[tc.ID]; exists {
				return model.Response{}, fmt.Errorf("duplicate tool call ID %q", tc.ID)
			}
			toolCalls[tc.ID] = &toolAccumulator{id: tc.ID, name: tc.Name}
			toolOrder = append(toolOrder, tc.ID)
		}

		if chunk.ToolCallInputDelta != nil {
			acc, ok := toolCalls[chunk.ToolCallInputDelta.ToolCallID]
			if !ok {
				return model.Response{}, fmt.Errorf("input delta for unknown tool call %q", chunk.ToolCallInputDelta.ToolCallID)
			}
			acc.inputBuf.WriteString(chunk.ToolCallInputDelta.JSONDelta)
		}

		if chunk.Done != nil {
			done = chunk.Done
		}
	}

	if done == nil {
		return model.Response{}, fmt.Errorf("stream ended without Done: %w", model.ErrStreamClosed)
	}

	// Build content parts: thinking → redacted thinking → text → tool calls
	var parts []model.ContentPart
	if thinkBuf.Len() > 0 {
		parts = append(parts, model.ThinkingPart{
			Text:      thinkBuf.String(),
			Signature: thinkSigBuf.String(),
		})
	}
	for _, rtp := range redactedThinkingParts {
		parts = append(parts, rtp)
	}
	if textBuf.Len() > 0 {
		parts = append(parts, model.TextPart{Text: textBuf.String()})
	}
	for _, id := range toolOrder {
		acc := toolCalls[id]
		raw := json.RawMessage(acc.inputBuf.String())
		if len(raw) == 0 {
			raw = json.RawMessage("{}")
		} else if !json.Valid(raw) {
			return model.Response{}, fmt.Errorf("invalid tool input JSON for %q", acc.name)
		}
		parts = append(parts, model.ToolCallPart{
			ID:    acc.id,
			Name:  acc.name,
			Input: raw,
		})
	}

	return model.Response{
		Model:      done.Model,
		Content:    parts,
		StopReason: done.StopReason,
		Usage:      done.Usage,
	}, nil
}

// extractToolCalls filters ToolCallPart values from a slice of ContentParts.
func extractToolCalls(parts []model.ContentPart) []model.ToolCallPart {
	var calls []model.ToolCallPart
	for _, p := range parts {
		if tc, ok := p.(model.ToolCallPart); ok {
			calls = append(calls, tc)
		}
	}
	return calls
}
