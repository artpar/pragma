package query

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/compact"
	"github.com/artpar/pragma/internal/hook"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/task"
	"github.com/artpar/pragma/internal/tool"
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

	defer func() {
		if e.hookMgr != nil {
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: e.hookMgr != nil")
			hookCtx, hookCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer hookCancel()
			e.hookMgr.Execute(hookCtx, hook.Stop, hook.HookInput{})
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

	malformedRetries := 0
	const maxMalformedRetries = 3
	turnCount := 0
	for turnCount < maxTurns {
		observe.TraceCtx(ctx, "query", "Engine.runLoop", fmt.Sprintf("turn %d/%d", turnCount+1, maxTurns))
		if err := ctx.Err(); err != nil {
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: err != nil")
			ch <- ErrorEvent{Err: fmt.Errorf("context cancelled: %w", model.ErrContextCancelled)}
			return
		}

		// Main session: sweep dead sub-agents each turn so TaskList stays accurate.
		if e.taskRegistry != nil && e.config.TaskID == "" {
			e.taskRegistry.ReapDead(task.DeadAgentTimeout)
		}

		if e.taskRegistry != nil && e.config.TaskID != "" {
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: e.taskRegistry != nil && e.config.TaskID != \"\"")
			e.taskRegistry.Heartbeat(e.config.TaskID)
			if msgs := e.taskRegistry.DrainPendingMessages(e.config.TaskID); len(msgs) > 0 {
				observe.TraceCtx(ctx, "query", "Engine.runLoop", fmt.Sprintf("draining %d pending messages", len(msgs)))
				injectedMsg := model.Message{
					ID:        model.NewUUID(),
					Role:      model.RoleUser,
					Content:   []model.ContentPart{model.TextPart{Text: "Messages from teammates:\n" + strings.Join(msgs, "\n")}},
					Timestamp: time.Now(),
				}
				e.store.Update(func(s *app.AppState) {
					s.Conversation.Append(injectedMsg)
				})
			}
		}

		snap := e.store.Snapshot()

		resolvedModel := e.config.Model
		if snap.Model != "" {
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: snap.Model != \"\"")
			resolvedModel = snap.Model
		}

		tools := e.registry.ToolDefs()
		if snap.PlanMode {
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: snap.PlanMode")
			tools = e.filterReadOnlyTools(tools)
		}

		params := provider.RequestParams{
			Model:       resolvedModel,
			MaxTokens:   e.config.MaxTokens,
			Messages:    snap.Conversation.APIMessages(),
			System:      snap.Conversation.System,
			Tools:       tools,
			Temperature: e.config.Temperature,
			Thinking:    e.config.Thinking,
		}

		// Stream with retry: exponential backoff for retryable errors.
		// Matches TS withRetry: DEFAULT_MAX_RETRIES=10, MAX_529_RETRIES=3, BASE_DELAY_MS=500.
		// Addresses GitHub #2047 (exponential backoff), #26699 (session stuck on rate limit),
		// #23976 (input unresponsive), #3633/#35487 (repeated 529 errors).
		const maxStreamRetries = 10
		const maxConsecutiveOverloaded = 3
		var consecutiveOverloaded int
		var response model.Response

		for attempt := range maxStreamRetries + 1 {
			observe.TraceCtx(ctx, "query", "Engine.runLoop", fmt.Sprintf("stream attempt %d/%d", attempt+1, maxStreamRetries+1))

			var streamErr error
			chunks, err := e.provider.Stream(ctx, params)
			if err != nil {
				observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: err != nil")
				streamErr = err
			} else {
				observe.TraceCtx(ctx, "query", "Engine.runLoop", "else: err != nil")
				response, streamErr = e.consumeStream(chunks, ch)
			}

			if streamErr == nil {
				observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: streamErr == nil")
				break
			}

			classified := ClassifyStreamError(streamErr)

			if classified.Kind == ErrorKindOverloaded {
				observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: classified.Kind == ErrorKindOverloaded")
				consecutiveOverloaded++
				if consecutiveOverloaded >= maxConsecutiveOverloaded {
					observe.TraceCtx(ctx, "query", "Engine.runLoop", "consecutive overloaded limit reached")
					ch <- ErrorEvent{Err: fmt.Errorf("repeated overloaded errors"), Kind: classified.Kind}
					return
				}
			} else {
				observe.TraceCtx(ctx, "query", "Engine.runLoop", "else: classified.Kind == ErrorKindOverloaded")
				consecutiveOverloaded = 0
			}

			if !classified.Retryable || attempt >= maxStreamRetries {
				observe.TraceCtx(ctx, "query", "Engine.runLoop", fmt.Sprintf("non-retryable or exhausted: %s", classified.Kind))
				ch <- ErrorEvent{Err: classified.Err, Kind: classified.Kind, Guidance: classified.Guidance}
				return
			}

			delay := classified.RetryAfter
			if delay == 0 {
				observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: delay == 0")
				delay = retryDelay(attempt)
			}

			ch <- RetryEvent{
				Attempt:     attempt + 1,
				MaxAttempts: maxStreamRetries + 1,
				Delay:       delay,
				Kind:        classified.Kind,
				ErrorMsg:    streamErr.Error(),
			}

			select {
			case <-time.After(delay):
				observe.TraceCtx(ctx, "query", "Engine.runLoop", "select: <-time.After(delay)")
				continue
			case <-ctx.Done():
				observe.TraceCtx(ctx, "query", "Engine.runLoop", "select: <-ctx.Done()")
				ch <- ErrorEvent{Err: fmt.Errorf("context cancelled during retry: %w", model.ErrContextCancelled)}
				return
			}
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

		pricing, known := e.provider.Pricing(resolvedModel)
		if !known {
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: !known")
			e.bus.Emit(observe.ErrorOccurred{
				EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
				Severity:     "warn",
				Component:    "query",
				ErrorType:    "unknown_model_pricing",
				ErrorMessage: fmt.Sprintf("no pricing data for model %q, costs will be zero", resolvedModel),
			})
		}
		e.costTracker.Record(resolvedModel, e.provider.Name(), response.Usage, pricing)

		if e.compactor != nil && e.autoTracker != nil {
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: e.compactor != nil && e.autoTracker != nil")
			compSnap := e.store.Snapshot()
			var tokenCount int
			if counter, ok := e.provider.(provider.TokenCounter); ok {
				observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: provider implements TokenCounter")
				countParams := provider.RequestParams{
					Model:    resolvedModel,
					Messages: compSnap.Conversation.APIMessages(),
					System:   compSnap.Conversation.System,
					Tools:    tools,
				}
				precise, countErr := counter.CountTokens(ctx, countParams)
				if countErr != nil {
					observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: countErr != nil, falling back to heuristic")
					tokenCount = compact.EstimateConversationTokens(compSnap.Conversation.APIMessages())
				} else {
					observe.TraceCtx(ctx, "query", "Engine.runLoop", "else: countErr != nil")
					tokenCount = precise
				}
			} else {
				observe.TraceCtx(ctx, "query", "Engine.runLoop", "else: ok")
				tokenCount = compact.EstimateConversationTokens(compSnap.Conversation.APIMessages())
			}
			if e.autoTracker.ShouldAutoCompact(tokenCount, e.windowConfig) {
				observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: e.autoTracker.ShouldAutoCompact(tokenCount, e.windowConfig)")
				ch <- CompactionStartedEvent{}
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
				ErrorMessage: fmt.Sprintf("model %q hit max_tokens limit — response was truncated", resolvedModel),
			})
			ch <- TurnCompleteEvent{Response: response, StopReason: response.StopReason}
			return

		case model.StopMalformedToolCall:
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "case: model.StopMalformedToolCall")
			malformedRetries++
			if malformedRetries > maxMalformedRetries {
				observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: malformedRetries > maxMalformedRetries")
				ch <- ErrorEvent{Err: fmt.Errorf("exceeded %d malformed tool call retries", maxMalformedRetries)}
				return
			}
			e.bus.Emit(observe.ErrorOccurred{
				EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
				Severity:     "warn",
				Component:    "query",
				ErrorType:    "malformed_tool_call",
				ErrorMessage: fmt.Sprintf("provider returned malformed tool call, retrying (%d/%d)", malformedRetries, maxMalformedRetries),
			})

			correctionMsg := model.Message{
				ID:        model.NewUUID(),
				Role:      model.RoleUser,
				Content:   []model.ContentPart{model.TextPart{Text: fmt.Sprintf("Your previous tool call had malformed arguments and was discarded. Please retry with valid JSON arguments. (attempt %d/%d)", malformedRetries, maxMalformedRetries)}},
				Timestamp: time.Now(),
			}
			e.store.Update(func(s *app.AppState) {
				s.Conversation.Append(correctionMsg)
			})
			turnCount++
			continue

		case model.StopContentFiltered:
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "case: model.StopContentFiltered")
			e.bus.Emit(observe.ErrorOccurred{
				EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
				Severity:     "warn",
				Component:    "query",
				ErrorType:    "content_filtered",
				ErrorMessage: fmt.Sprintf("model %q response was blocked by content filter — text preserved, tool calls dropped", resolvedModel),
			})
			ch <- TurnCompleteEvent{Response: response, StopReason: response.StopReason}
			return

		case model.StopPauseTurn:
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "case: model.StopPauseTurn")
			turnCount++

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

			progressCh := make(chan tool.ProgressEvent, 16)
			wrappedSnap := &progressSnapshot{StateSnapshot: snap, progressCh: progressCh}

			type execDone struct {
				result tool.ExecuteResult
			}
			doneCh := make(chan execDone, 1)
			go func() {
				doneCh <- execDone{result: e.orchestrator.Execute(ctx, toolCalls, wrappedSnap)}
			}()

			var execResult tool.ExecuteResult
		drainLoop:
			for {
				select {
				case pe := <-progressCh:
					ch <- progressToLoopEvent(pe)
				case d := <-doneCh:
					execResult = d.result

					for {
						select {
						case pe := <-progressCh:
							ch <- progressToLoopEvent(pe)
						default:
							break drainLoop
						}
					}
				}
			}

			for i, r := range execResult.Results {
				ch <- ToolResultEvent{Result: r, Display: execResult.Displays[i]}
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
			turnCount++
			continue

		case model.StopError:
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "case: model.StopError")
			e.bus.Emit(observe.ErrorOccurred{
				EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
				Severity:     "error",
				Component:    "query",
				ErrorType:    "provider_error",
				ErrorMessage: fmt.Sprintf("model %q returned an error stop reason", resolvedModel),
			})
			ch <- TurnCompleteEvent{Response: response, StopReason: response.StopReason}
			return

		default:
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "default")
			ch <- ErrorEvent{Err: fmt.Errorf("unknown stop reason: %s", response.StopReason)}
			return
		}
	}

	ch <- ErrorEvent{Err: fmt.Errorf("agentic loop exceeded maximum of %d turns", maxTurns)}
}

// progressSnapshot wraps a StateSnapshot with a ProgressReporter for tool progress.
// Implements tool.ProgressSource via optional interface pattern (ADR-042).
type progressSnapshot struct {
	tool.StateSnapshot
	progressCh chan tool.ProgressEvent
}

func (p *progressSnapshot) Progress() tool.ProgressReporter {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: p.progressCh")
	return p.progressCh
}

// progressToLoopEvent converts a tool.ProgressEvent to a typed LoopEvent.
// Dispatches on Kind: "agent" → AgentProgressEvent, default → LifecycleProgressEvent.
func progressToLoopEvent(pe tool.ProgressEvent) LoopEvent {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if pe.Kind == "agent" {
		observe.GlobalTrace("if: pe.Kind == \"agent\"")
		observe.GlobalTrace("return: AgentProgressEvent{\n\tAgentID:\tpe.AgentID,\n\tDescription:\tpe.Description,\n\tTool...")
		return AgentProgressEvent{
			AgentID:     pe.AgentID,
			Description: pe.Description,
			ToolCount:   pe.ToolCount,
			TokenCount:  pe.TokenCount,
			LastTool:    pe.LastTool,
			Status:      pe.Status,
			Background:  pe.Background,
			Error:       pe.Error,
		}
	}
	observe.GlobalTrace("return: LifecycleProgressEvent{\n\tStep:\t\tpe.Step,\n\tNode:\t\tpe.Node,\n\tNodes:\t\tpe.Nodes,\n...")
	return LifecycleProgressEvent{
		Step:     pe.Step,
		Node:     pe.Node,
		Nodes:    pe.Nodes,
		Status:   pe.Status,
		Duration: pe.Duration,
		Error:    pe.Error,
		FromNode: pe.FromNode,
		ToNode:   pe.ToNode,
		RouteKey: pe.RouteKey,
	}
}

// toolAccumulator collects streaming fragments for a single tool call.
type toolAccumulator struct {
	id        string
	name      string
	signature string
	inputBuf  strings.Builder
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
			toolCalls[tc.ID] = &toolAccumulator{id: tc.ID, name: tc.Name, signature: tc.Signature}
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

	if done.StopReason == model.StopMalformedToolCall || done.StopReason == model.StopContentFiltered {
		observe.GlobalTrace("if: done.StopReason == model.StopMalformedToolCall || StopContentFiltered — dropping tool calls")
	} else {
		observe.GlobalTrace("else: done.StopReason == model.StopMalformedToolCall || done.StopReason == model.St...")
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
				ID:        acc.id,
				Name:      acc.name,
				Input:     raw,
				Signature: acc.signature,
			})
		}
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
