package query

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/compact"
	"github.com/artpar/gogent/internal/hook"
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
	observe.TraceCtx(ctx, "query", "Engine.Run", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.Run", "exit")
	ch := make(chan LoopEvent, 16)
	go func() {
		defer close(ch)
		defer func() {
			if r := recover(); r != nil {
				observe.TraceCtx(ctx, "query", "Engine.Run", "if: r != nil")
				ch <- ErrorEvent{Err: fmt.Errorf("query loop panic: %v", r)}
			}
		}()
		e.runLoop(ctx, userMessage, ch)
	}()
	observe.TraceCtx(ctx, "query", "Engine.Run", "return: ch")
	return ch
}

func (e *Engine) runLoop(ctx context.Context, userMessage string, ch chan<- LoopEvent) {
	observe.TraceCtx(ctx, "query", "Engine.runLoop", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.runLoop", "exit")

	// Stop hook — fires when the query loop ends for any reason
	defer func() {
		if e.hookMgr != nil {
			e.hookMgr.Execute(ctx, hook.Stop, hook.HookInput{})
		}
	}()

	maxTurns := e.config.MaxTurns
	if maxTurns <= 0 {
		observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: maxTurns <= 0")
		maxTurns = DefaultMaxTurns
	}

	userMsg := model.Message{
		ID:        model.NewUUID(),
		Role:      model.RoleUser,
		Content:   []model.ContentPart{model.TextPart{Text: userMessage}},
		Timestamp: time.Now(),
	}
	e.store.Update(func(s *app.AppState) {
		s.Conversation.Append(userMsg)
	})

	for range maxTurns {
		observe.TraceCtx(ctx, "query", "Engine.runLoop", "range maxTurns")
		if err := ctx.Err(); err != nil {
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: err != nil")
			ch <- ErrorEvent{Err: fmt.Errorf("context cancelled: %w", model.ErrContextCancelled)}
			return
		}

		snap := e.store.Snapshot()

		tools := e.registry.ToolDefs()
		if snap.PlanMode {
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: snap.PlanMode")
			tools = e.filterReadOnlyTools(tools)
		}

		params := provider.RequestParams{
			Model:       e.config.Model,
			MaxTokens:   e.config.MaxTokens,
			Messages:    snap.Conversation.APIMessages(),
			System:      snap.Conversation.System,
			Tools:       tools,
			Temperature: e.config.Temperature,
			Thinking:    e.config.Thinking,
		}

		chunks, err := e.provider.Stream(ctx, params)
		if err != nil {
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: err != nil")
			ch <- ErrorEvent{Err: err}
			return
		}

		response, err := e.consumeStream(chunks, ch)
		if err != nil {
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: err != nil")
			ch <- ErrorEvent{Err: err}
			return
		}

		assistantMsg := model.Message{
			ID:        model.NewUUID(),
			Role:      model.RoleAssistant,
			Content:   response.Content,
			Timestamp: time.Now(),
		}
		e.store.Update(func(s *app.AppState) {
			s.Conversation.Append(assistantMsg)
		})

		pricing, known := e.provider.Pricing(e.config.Model)
		if !known {
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: !known")
			e.bus.Emit(observe.ErrorOccurred{
				EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
				Severity:     "warn",
				Component:    "query",
				ErrorType:    "unknown_model_pricing",
				ErrorMessage: fmt.Sprintf("no pricing data for model %q, costs will be zero", e.config.Model),
			})
		}
		e.costTracker.Record(e.config.Model, e.provider.Name(), response.Usage, pricing)

		if e.compactor != nil && e.autoTracker != nil {
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: e.compactor != nil && e.autoTracker != nil")
			compSnap := e.store.Snapshot()
			tokenCount := compact.EstimateConversationTokens(compSnap.Conversation.APIMessages())
			if e.autoTracker.ShouldAutoCompact(tokenCount, e.windowConfig) {
				observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: e.autoTracker.ShouldAutoCompact(tokenCount, e.windowConfig)")
				compResult, compErr := e.compactor.Compact(ctx, compSnap.Conversation.APIMessages(), compSnap.Conversation.System, "")
				if compErr != nil && ctx.Err() == nil {
					observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: compErr != nil && ctx.Err() == nil")

					tripped := e.autoTracker.RecordFailure()
					if tripped {
						observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: tripped")
						e.bus.Emit(observe.ErrorOccurred{
							EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
							Severity:     "warn",
							Component:    "compact",
							ErrorType:    "circuit_breaker_tripped",
							ErrorMessage: fmt.Sprintf("auto-compaction disabled after %d consecutive failures", compact.MaxConsecutiveFailures),
						})
						ch <- CompactionDisabledEvent{ConsecutiveFailures: compact.MaxConsecutiveFailures}
					}
				} else {
					observe.TraceCtx(ctx, "query", "Engine.runLoop", "else: compErr != nil && ctx.Err() == nil")
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

		switch response.StopReason {
		case model.StopEndTurn:
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "case: model.StopEndTurn")
			ch <- TurnCompleteEvent{Response: response, StopReason: response.StopReason}
			return

		case model.StopMaxTokens:
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "case: model.StopMaxTokens")
			e.bus.Emit(observe.ErrorOccurred{
				EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
				Severity:     "warn",
				Component:    "query",
				ErrorType:    "response_truncated",
				ErrorMessage: fmt.Sprintf("model %q hit max_tokens limit — response was truncated", e.config.Model),
			})
			ch <- TurnCompleteEvent{Response: response, StopReason: response.StopReason}
			return

		case model.StopPauseTurn:
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "case: model.StopPauseTurn")

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
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "case: model.StopToolUse")
			toolCalls := extractToolCalls(response.Content)
			for _, tc := range toolCalls {
				ch <- ToolCallEvent{Call: tc}
			}

			execResult := e.orchestrator.Execute(ctx, toolCalls, snap)
			for _, r := range execResult.Results {
				ch <- ToolResultEvent{Result: r}
			}

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
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "default")
			ch <- ErrorEvent{Err: fmt.Errorf("unknown stop reason: %s", response.StopReason)}
			return
		}
	}

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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var textBuf strings.Builder
	var thinkBuf strings.Builder
	var thinkSigBuf strings.Builder
	var redactedThinkingParts []model.ThinkingPart
	toolCalls := make(map[string]*toolAccumulator)
	var toolOrder []string
	var done *provider.StreamDone

	for chunk := range chunks {
		observe.GlobalTrace("range chunks")
		if chunk.Error != nil {
			observe.GlobalTrace("if: chunk.Error != nil")
			observe.GlobalTrace("return: model.Response{}, chunk.Error")
			return model.Response{}, chunk.Error
		}

		if chunk.TextDelta != "" {
			observe.GlobalTrace("if: chunk.TextDelta != \"\"")
			textBuf.WriteString(chunk.TextDelta)
			ch <- TextEvent{Text: chunk.TextDelta}
		}

		if chunk.ThinkingDelta != "" {
			observe.GlobalTrace("if: chunk.ThinkingDelta != \"\"")
			thinkBuf.WriteString(chunk.ThinkingDelta)
			ch <- ThinkingEvent{Text: chunk.ThinkingDelta}
		}

		if chunk.ThinkingSignatureDelta != "" {
			observe.GlobalTrace("if: chunk.ThinkingSignatureDelta != \"\"")
			thinkSigBuf.WriteString(chunk.ThinkingSignatureDelta)
		}

		if chunk.RedactedThinkingBlock != nil {
			observe.GlobalTrace("if: chunk.RedactedThinkingBlock != nil")
			redactedThinkingParts = append(redactedThinkingParts, model.ThinkingPart{
				Redacted:     true,
				RedactedData: chunk.RedactedThinkingBlock.Data,
			})
		}

		if chunk.ToolCallStart != nil {
			observe.GlobalTrace("if: chunk.ToolCallStart != nil")
			tc := chunk.ToolCallStart
			if _, exists := toolCalls[tc.ID]; exists {
				observe.GlobalTrace("if: exists")
				observe.GlobalTrace("return: model.Response{}, fmt.Errorf(\"duplicate tool call ID %q\", tc.ID)")
				return model.Response{}, fmt.Errorf("duplicate tool call ID %q", tc.ID)
			}
			toolCalls[tc.ID] = &toolAccumulator{id: tc.ID, name: tc.Name}
			toolOrder = append(toolOrder, tc.ID)
		}

		if chunk.ToolCallInputDelta != nil {
			observe.GlobalTrace("if: chunk.ToolCallInputDelta != nil")
			acc, ok := toolCalls[chunk.ToolCallInputDelta.ToolCallID]
			if !ok {
				observe.GlobalTrace("if: !ok")
				observe.GlobalTrace("return: model.Response{}, fmt.Errorf(\"input delta for unknown tool call %q\", chunk.To...")
				return model.Response{}, fmt.Errorf("input delta for unknown tool call %q", chunk.ToolCallInputDelta.ToolCallID)
			}
			acc.inputBuf.WriteString(chunk.ToolCallInputDelta.JSONDelta)
		}

		if chunk.Done != nil {
			observe.GlobalTrace("if: chunk.Done != nil")
			done = chunk.Done
		}
	}

	if done == nil {
		observe.GlobalTrace("if: done == nil")
		observe.GlobalTrace("return: model.Response{}, fmt.Errorf(\"stream ended without Done: %w\", model.ErrStream...")
		return model.Response{}, fmt.Errorf("stream ended without Done: %w", model.ErrStreamClosed)
	}

	// Build content parts: thinking → redacted thinking → text → tool calls
	var parts []model.ContentPart
	if thinkBuf.Len() > 0 {
		observe.GlobalTrace("if: thinkBuf.Len() > 0")
		parts = append(parts, model.ThinkingPart{
			Text:      thinkBuf.String(),
			Signature: thinkSigBuf.String(),
		})
	}
	for _, rtp := range redactedThinkingParts {
		observe.GlobalTrace("range redactedThinkingParts")
		parts = append(parts, rtp)
	}
	if textBuf.Len() > 0 {
		observe.GlobalTrace("if: textBuf.Len() > 0")
		parts = append(parts, model.TextPart{Text: textBuf.String()})
	}
	for _, id := range toolOrder {
		observe.GlobalTrace("range toolOrder")
		acc := toolCalls[id]
		raw := json.RawMessage(acc.inputBuf.String())
		if len(raw) == 0 {
			observe.GlobalTrace("if: len(raw) == 0")
			raw = json.RawMessage("{}")
		} else if !json.Valid(raw) {
			observe.GlobalTrace("else-if: !json.Valid(raw)")
			return model.Response{}, fmt.Errorf("invalid tool input JSON for %q", acc.name)
		}
		parts = append(parts, model.ToolCallPart{
			ID:    acc.id,
			Name:  acc.name,
			Input: raw,
		})
	}
	observe.GlobalTrace("return: model.Response{\n\tModel:\t\tdone.Model,\n\tContent:\tparts,\n\tStopReason:\tdone.StopR...")

	return model.Response{
		Model:      done.Model,
		Content:    parts,
		StopReason: done.StopReason,
		Usage:      done.Usage,
	}, nil
}

// filterReadOnlyTools returns only tool defs whose Flags().ReadOnly is true.
// Used in plan mode to restrict the LLM to non-mutating tools.
func (e *Engine) filterReadOnlyTools(tools []model.ToolDef) []model.ToolDef {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	filtered := make([]model.ToolDef, 0, len(tools))
	for _, td := range tools {
		observe.GlobalTrace("range tools")
		desc, ok := e.registry.Get(td.Name)
		if ok && desc.Flags().ReadOnly {
			observe.GlobalTrace("if: ok && desc.Flags().ReadOnly")
			filtered = append(filtered, td)
		}
	}
	observe.GlobalTrace("return: filtered")
	return filtered
}

// extractToolCalls filters ToolCallPart values from a slice of ContentParts.
func extractToolCalls(parts []model.ContentPart) []model.ToolCallPart {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var calls []model.ToolCallPart
	for _, p := range parts {
		observe.GlobalTrace("range parts")
		if tc, ok := p.(model.ToolCallPart); ok {
			observe.GlobalTrace("if: ok")
			calls = append(calls, tc)
		}
	}
	observe.GlobalTrace("return: calls")
	return calls
}
