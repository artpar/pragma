package query

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/compact"
	"github.com/artpar/pragma/internal/debug"
	"github.com/artpar/pragma/internal/hook"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/task"
	"github.com/artpar/pragma/internal/tool"
	"github.com/artpar/pragma/internal/toolresult"
)

// continuationPrompt is sent when the model returns StopPauseTurn,
// indicating it wants to continue but hit a turn-level limit.
const continuationPrompt = "Please continue."

const (
	stateHandoffMaxToolResultChars      = 24_000
	stateHandoffMaxToolResultBatchChars = 64_000
)

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
		if e.isStateHandoffMode() && s.HandoffState.IsZero() {
			s.HandoffState = model.NewHandoffState(userMessage)
		}
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

		if e.taskRegistry != nil && e.config.TaskID == "" {
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: e.taskRegistry != nil && e.config.TaskID == \"\"")
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
		if e.isStateHandoffMode() {
			tools = append([]model.ToolDef{handoffPatchToolDef(), certifyFactToolDef()}, tools...)
		}

		messagesForQuery := e.messagesForRequest(snap.Conversation)
		var budgetErr error
		messagesForQuery, budgetErr = e.applyToolResultBudget(messagesForQuery)
		if budgetErr != nil {
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "tool result budget failed, continuing with original messages")
		}

		systemForQuery := e.systemWithMCPStatus(snap.Conversation.System)
		systemForQuery = e.systemWithHandoffState(systemForQuery, snap.HandoffState)

		if compacted, ok := e.autoCompactBeforeRequest(ctx, ch, resolvedModel, messagesForQuery, systemForQuery, tools); compacted {
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: compacted")
			debug.Log("autoCompactBeforeRequest returned true for model %s", resolvedModel)
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: compacted")
			messagesForQuery = ok
			snap = e.store.Snapshot()
			systemForQuery = e.systemWithMCPStatus(snap.Conversation.System)
			systemForQuery = e.systemWithHandoffState(systemForQuery, snap.HandoffState)
		} else if e.isAtBlockingLimit(ctx, resolvedModel, messagesForQuery, systemForQuery, tools) {
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "else-if: e.isAtBlockingLimit(ctx, resolvedModel, messagesForQuery, systemForQuery, tools)")
			ch <- ErrorEvent{
				Err:      provider.ErrContextOverflow,
				Kind:     ErrorKindContextOverflow,
				Guidance: "Run /compact to reduce context size",
			}
			return
		}

		params := provider.RequestParams{
			Model:       resolvedModel,
			MaxTokens:   e.config.MaxTokens,
			Messages:    messagesForQuery,
			System:      systemForQuery,
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
		debug.Log("Assistant response: StopReason=%s, ContentParts=%d", response.StopReason, len(response.Content))
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

		if e.autoTracker != nil {
			observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: e.autoTracker != nil")
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

			execResult := e.executeToolBatch(ctx, toolCalls, snap, ch)

			for i, r := range execResult.Results {
				ch <- ToolResultEvent{Result: r, Display: execResult.Displays[i]}
			}

			resultParts := make([]model.ContentPart, 0, len(execResult.Results)+len(execResult.Supplements))
			for _, r := range execResult.Results {
				resultParts = append(resultParts, r)
			}
			resultParts = append(resultParts, execResult.Supplements...)

			remaining := maxTurns - turnCount - 1
			warningThreshold := maxTurns / 5
			if warningThreshold < 2 {
				warningThreshold = 2
			}
			if remaining == warningThreshold {
				resultParts = append(resultParts, model.TextPart{
					Text: fmt.Sprintf("[system: %d turns remaining out of %d. Wrap up your current task or summarize progress.]", remaining, maxTurns),
				})
			}

			resultMsg := model.Message{
				ID:        model.NewUUID(),
				Role:      model.RoleUser,
				Content:   resultParts,
				Timestamp: time.Now(),
			}
			e.store.Update(func(s *app.AppState) {
				s.Conversation.Append(resultMsg)
			})
			if e.config.StopAfterToolExec {
				observe.TraceCtx(ctx, "query", "Engine.runLoop", "if: e.config.StopAfterToolExec")
				ch <- TurnCompleteEvent{Response: response, StopReason: response.StopReason}
				return
			}
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

func (e *Engine) applyToolResultBudget(messages []model.Message) ([]model.Message, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	snap := e.store.Snapshot()
	skipToolNames := make(map[string]bool)
	for _, desc := range e.registry.List() {
		observe.GlobalTrace("range e.registry.List()")
		if desc.Flags().MaxResultSizeChars < 0 {
			observe.GlobalTrace("if: desc.Flags().MaxResultSizeChars < 0")
			skipToolNames[desc.Name()] = true
		}
	}
	out, records, err := toolresult.ApplyToolResultBudget(messages, e.contentReplacementState, snap.Conversation.ID, skipToolNames)
	if len(records) > 0 && e.config.RecordContentReplacements != nil {
		observe.GlobalTrace("if: len(records) > 0 && e.config.RecordContentReplacements != nil")
		e.config.RecordContentReplacements(records)
	}
	observe.GlobalTrace("return: out, err")
	return out, err
}

func (e *Engine) autoCompactBeforeRequest(
	ctx context.Context,
	ch chan<- LoopEvent,
	resolvedModel string,
	messages []model.Message,
	system model.SystemPrompt,
	tools []model.ToolDef,
) (bool, []model.Message) {
	observe.TraceCtx(ctx, "query", "Engine.autoCompactBeforeRequest", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.autoCompactBeforeRequest", "exit")
	if e.compactor == nil || e.autoTracker == nil {
		observe.TraceCtx(ctx, "query", "Engine.autoCompactBeforeRequest", "if: e.compactor == nil || e.autoTracker == nil")
		observe.TraceCtx(ctx, "query", "Engine.autoCompactBeforeRequest", "return: false, nil")
		return false, nil
	}
	tokenCount := e.requestTokenCount(ctx, resolvedModel, messages, system, tools)
	if !e.autoTracker.ShouldAutoCompact(tokenCount, e.windowConfig) {
		observe.TraceCtx(ctx, "query", "Engine.autoCompactBeforeRequest", "if: !e.autoTracker.ShouldAutoCompact(tokenCount, e.windowConfig)")
		observe.TraceCtx(ctx, "query", "Engine.autoCompactBeforeRequest", "return: false, nil")
		return false, nil
	}

	ch <- CompactionStartedEvent{}
	compResult, compErr := e.compactor.Compact(ctx, messages, system, "")
	if compErr != nil {
		observe.TraceCtx(ctx, "query", "Engine.autoCompactBeforeRequest", "if: compErr != nil")
		if ctx.Err() != nil {
			observe.TraceCtx(ctx, "query", "Engine.autoCompactBeforeRequest", "if: ctx.Err() != nil")
			observe.TraceCtx(ctx, "query", "Engine.autoCompactBeforeRequest", "return: false, nil")
			return false, nil
		}
		tripped := e.autoTracker.RecordFailure()
		ch <- CompactionFailedEvent{
			Attempt:  e.autoTracker.FailureCount(),
			MaxRetry: compact.MaxConsecutiveFailures,
			ErrorMsg: compErr.Error(),
		}
		if tripped {
			observe.TraceCtx(ctx, "query", "Engine.autoCompactBeforeRequest", "if: tripped")
			e.bus.Emit(observe.ErrorOccurred{
				EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
				Severity:     "warn",
				Component:    "compact",
				ErrorType:    "circuit_breaker_tripped",
				ErrorMessage: fmt.Sprintf("auto-compaction disabled after %d consecutive failures", compact.MaxConsecutiveFailures),
			})
			ch <- CompactionDisabledEvent{ConsecutiveFailures: compact.MaxConsecutiveFailures}
		}
		observe.TraceCtx(ctx, "query", "Engine.autoCompactBeforeRequest", "return: false, nil")
		return false, nil
	}

	e.autoTracker.RecordSuccess()
	e.store.Update(func(s *app.AppState) {
		s.Conversation.Messages = compResult.ReplacementMessages
		s.Conversation.UpdatedAt = time.Now()
	})
	ch <- CompactionEvent{PreTokens: compResult.PreTokenCount, PostTokens: compResult.PostTokenCount}
	observe.TraceCtx(ctx, "query", "Engine.autoCompactBeforeRequest", "return: true, compResult.ReplacementMessages")
	return true, compResult.ReplacementMessages
}

func (e *Engine) isAtBlockingLimit(ctx context.Context, resolvedModel string, messages []model.Message, system model.SystemPrompt, tools []model.ToolDef) bool {
	observe.TraceCtx(ctx, "query", "Engine.isAtBlockingLimit", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.isAtBlockingLimit", "exit")
	if compact.EffectiveWindow(e.windowConfig) == 0 {
		observe.TraceCtx(ctx, "query", "Engine.isAtBlockingLimit", "if: compact.EffectiveWindow(e.windowConfig) == 0")
		observe.TraceCtx(ctx, "query", "Engine.isAtBlockingLimit", "return: false")
		return false
	}
	tokenCount := e.requestTokenCount(ctx, resolvedModel, messages, system, tools)
	blockingLimit := compact.EffectiveWindow(e.windowConfig) - 3_000
	if blockingLimit < 0 {
		observe.TraceCtx(ctx, "query", "Engine.isAtBlockingLimit", "if: blockingLimit < 0")
		blockingLimit = 0
	}
	observe.TraceCtx(ctx, "query", "Engine.isAtBlockingLimit", "return: tokenCount >= blockingLimit")
	return tokenCount >= blockingLimit
}

func (e *Engine) requestTokenCount(ctx context.Context, resolvedModel string, messages []model.Message, system model.SystemPrompt, tools []model.ToolDef) int {
	observe.TraceCtx(ctx, "query", "Engine.requestTokenCount", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.requestTokenCount", "exit")
	if counter, ok := e.provider.(provider.TokenCounter); ok {
		observe.TraceCtx(ctx, "query", "Engine.requestTokenCount", "if: ok")
		countParams := provider.RequestParams{
			Model:    resolvedModel,
			Messages: messages,
			System:   system,
			Tools:    tools,
		}
		if precise, err := counter.CountTokens(ctx, countParams); err == nil {
			observe.TraceCtx(ctx, "query", "Engine.requestTokenCount", "if: err == nil")
			debug.Log("Precise token count for model %s: %d", resolvedModel, precise)
			observe.TraceCtx(ctx, "query", "Engine.requestTokenCount", "if: err == nil")
			observe.TraceCtx(ctx, "query", "Engine.requestTokenCount", "return: precise")
			return precise
		}
	}
	est := compact.EstimateConversationTokens(messages)
	debug.Log("Estimated token count for model %s: %d", resolvedModel, est)
	observe.TraceCtx(ctx, "query", "Engine.requestTokenCount", "return: compact.EstimateConversationTokens(messages)")
	observe.TraceCtx(ctx, "query", "Engine.requestTokenCount", "return: est")
	return est
}

func (e *Engine) systemWithMCPStatus(system model.SystemPrompt) model.SystemPrompt {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if e.config.MCPServerStatuses == nil {
		observe.GlobalTrace("if: e.config.MCPServerStatuses == nil")
		observe.GlobalTrace("return: system")
		return system
	}
	statuses := e.config.MCPServerStatuses()
	if len(statuses) == 0 {
		observe.GlobalTrace("if: len(statuses) == 0")
		observe.GlobalTrace("return: system")
		return system
	}

	var b strings.Builder
	b.WriteString("# MCP Servers\n\n")
	b.WriteString("mcp_servers:\n")
	for _, server := range statuses {
		observe.GlobalTrace("range statuses")
		if server.Name == "" {
			observe.GlobalTrace("if: server.Name == \"\"")
			continue
		}
		status := server.Status
		if status == "" {
			observe.GlobalTrace("if: status == \"\"")
			status = "disconnected"
		}
		fmt.Fprintf(&b, "- name: %s\n  status: %s\n", server.Name, status)
	}
	b.WriteString("\nUse this mcp_servers metadata as the authoritative MCP server configuration and connection status. When asked which MCP servers are configured, active, inactive, connected, failed, pending, or disabled, answer directly from mcp_servers without calling tools. ListMcpResourcesTool lists resources only and must not be used to infer MCP server status.")

	blocks := make([]model.SystemBlock, 0, len(system.Blocks)+1)
	blocks = append(blocks, system.Blocks...)
	blocks = append(blocks, model.SystemBlock{Text: b.String(), Cacheable: false})
	observe.GlobalTrace("return: model.SystemPrompt{Blocks: blocks}")
	return model.SystemPrompt{Blocks: blocks}
}

func (e *Engine) isStateHandoffMode() bool {
	return e.config.ContextMode == model.ContextModeStateHandoff
}

func (e *Engine) messagesForRequest(conv model.Conversation) []model.Message {
	if !e.isStateHandoffMode() {
		return conv.APIMessages()
	}
	api := conv.APIMessages()
	if len(api) < 2 {
		return api
	}
	last := api[len(api)-1]
	if last.Role == model.RoleUser && !messageHasToolResult(last) {
		if exchange, ok := latestToolExchange(api[:len(api)-1]); ok {
			return append(boundStateHandoffToolResults(exchange), last)
		}
		return []model.Message{last}
	}
	if exchange, ok := latestToolExchange(api); ok {
		return boundStateHandoffToolResults(exchange)
	}
	return api
}

func latestToolExchange(api []model.Message) ([]model.Message, bool) {
	for i := len(api) - 1; i >= 1; i-- {
		if api[i].Role != model.RoleUser || !messageHasToolResult(api[i]) {
			continue
		}
		for j := i - 1; j >= 0; j-- {
			if api[j].Role == model.RoleAssistant && messageHasToolCall(api[j]) {
				return []model.Message{api[j], api[i]}, true
			}
		}
	}
	return nil, false
}

func boundStateHandoffToolResults(messages []model.Message) []model.Message {
	out := make([]model.Message, len(messages))
	remaining := stateHandoffMaxToolResultBatchChars
	for i, msg := range messages {
		out[i] = msg
		if len(msg.Content) == 0 {
			continue
		}
		content := make([]model.ContentPart, len(msg.Content))
		for j, part := range msg.Content {
			result, ok := part.(model.ToolResultPart)
			if !ok {
				content[j] = part
				continue
			}
			limit := stateHandoffMaxToolResultChars
			if remaining < limit {
				limit = remaining
			}
			result.Content = boundToolResultContent(result.Content, limit)
			remaining -= len(result.Content)
			if remaining < 0 {
				remaining = 0
			}
			content[j] = result
		}
		out[i].Content = content
	}
	return out
}

func boundToolResultContent(content string, limit int) string {
	if limit <= 0 {
		return fmt.Sprintf("[tool result omitted from state-handoff prompt: original_size=%d chars; use focused read/search or CertifyFact for exact facts]", len(content))
	}
	if len(content) <= limit {
		return content
	}
	const suffixBudget = 180
	if limit <= suffixBudget {
		return fmt.Sprintf("[tool result truncated from %d chars]", len(content))
	}
	keep := limit - suffixBudget
	return content[:keep] + fmt.Sprintf("\n\n[tool result truncated for state-handoff prompt: original_size=%d chars, shown_prefix=%d chars; use focused read/search or CertifyFact for exact facts]", len(content), keep)
}

func (e *Engine) systemWithHandoffState(system model.SystemPrompt, state model.HandoffState) model.SystemPrompt {
	if !e.isStateHandoffMode() {
		return system
	}
	if state.IsZero() {
		state = model.NewHandoffState("")
	}
	block := `# Handoff State Protocol

You are running in state-handoff context mode. You do not receive older chat history.
Use current_handoff_state plus the latest assistant tool_call blocks and matching tool_result blocks as your continuity source.
PatchHandoffState is the required continuity mechanism in this mode.
Every tool-use response MUST call PatchHandoffState as the first tool call before any other tool.
The PatchHandoffState call must record durable task state: todos, recent_actions, completed work, files read or changed, evidence, risks, current_focus, and next_action as applicable.
CertifyFact is the only tool that may write current_handoff_state.certified_facts. PatchHandoffState may reference certified fact IDs, but MUST NOT write /certified_facts directly.
After a tool_result, interpret the concrete result into current_handoff_state before choosing the next tool.
Do not repeat the same real tool call or same search if the latest tool_result already answered it. Advance next_action to the next distinct step.
If Grep returns matching files, record those files/evidence and Read the most relevant file next instead of Grep again.
Do not end the turn with only a plan when the user's coding task still has pending implementation or verification work. Patch the plan into current_handoff_state and call the next real tool in the same response, or mark a concrete blocker.
Before calling any non-read-only tool such as Edit, Write, Bash, NotebookEdit, or other changing/destructive tools, you MUST first investigate with read-only tools, call CertifyFact to create runtime-verified facts, and patch:
- /investigation/certified_fact_refs/- with a fact ID returned by CertifyFact.
- /investigation/observed_contracts/- with the real source/evidence you are relying on, including fact_refs that reference certified facts.
- /investigation/acceptance_checks/- with the real acceptance command or expected real-input check that will prove the change, including fact_refs that reference certified facts.
- /investigation/ready_for_changes to true.
The runtime blocks non-read-only tools until those fields are present.
When calling any real tool, call PatchHandoffState and that real tool in the same response, with PatchHandoffState first.
If no durable task state changed yet, still call PatchHandoffState first with a minimal current_focus, next_action, or latest_tool_result_interpretation update explaining what you are about to do.
PatchHandoffState accepts arbitrary JSON paths and fields. Use whatever structure best preserves progress for the next turn.

Example patch-first response after reading files:
PatchHandoffState({"ops":[
  {"op":"replace","path":"/latest_tool_result_interpretation","value":"Read internal/query/loop.go and found the state-handoff prompt builder."},
  {"op":"add","path":"/files/read/-","value":"internal/query/loop.go"},
  {"op":"add","path":"/recent_actions/-","value":"Read internal/query/loop.go"},
  {"op":"add","path":"/todos/-","value":{"id":"inspect-query-loop","task":"Inspect query loop state-handoff request construction","status":"completed"}},
  {"op":"add","path":"/todos/-","value":{"id":"update-prompt","task":"Update the state-handoff protocol text","status":"in_progress"}},
  {"op":"replace","path":"/current_focus","value":"Update the handoff prompt requirements."},
  {"op":"replace","path":"/next_action","value":"Edit the state-handoff protocol text."}
]})
Edit(...)

current_handoff_state:
` + state.PrettyJSON()

	blocks := make([]model.SystemBlock, 0, len(system.Blocks)+1)
	blocks = append(blocks, system.Blocks...)
	blocks = append(blocks, model.SystemBlock{Text: block, Cacheable: false})
	return model.SystemPrompt{Blocks: blocks}
}

func handoffPatchToolDef() model.ToolDef {
	return model.ToolDef{
		Name:        "PatchHandoffState",
		Description: "Patch the persistent JSON handoff state. Use this to preserve tool-result interpretation, constraints, completed work, next action, risks, and evidence.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"ops":{
					"type":"array",
					"items":{
						"type":"object",
						"properties":{
							"op":{"type":"string"},
							"path":{"type":"string"},
							"value":{}
						}
					}
				}
			},
			"required":["ops"]
		}`),
	}
}

func certifyFactToolDef() model.ToolDef {
	return model.ToolDef{
		Name:        "CertifyFact",
		Description: "Ask Pragma to verify a concrete fact from runtime evidence. Valid kind values are file_contains, json_shape, jsonl_shape, and tool_result_contains. Only this tool can write certified_facts into handoff state.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"id":{"type":"string","description":"Stable fact ID to reference from current_handoff_state.investigation.certified_fact_refs and acceptance_checks.fact_refs."},
				"kind":{"type":"string","enum":["file_contains","json_shape","jsonl_shape","tool_result_contains"],"description":"file_contains verifies that a real file contains text; json_shape/jsonl_shape verifies matching JSONL records have required fields; tool_result_contains verifies a previous tool_result by tool_call_id contains text."},
				"path":{"type":"string","description":"Filesystem path for file_contains, json_shape, or jsonl_shape. Relative paths resolve against the current working directory."},
				"contains":{"type":"string","description":"Exact text that must be present for file_contains or tool_result_contains."},
				"claim":{"type":"string","description":"Human-readable claim this verified fact proves."},
				"required_fields":{"type":"array","items":{"type":"string"},"description":"Fields that must exist on at least one matching JSONL object for json_shape/jsonl_shape."},
				"selector":{"type":"object","properties":{"field":{"type":"string"},"equals":{"type":"string"}},"description":"Optional JSONL selector. Example: {\"field\":\"kind\",\"equals\":\"APIRequestStarted\"}."},
				"tool_call_id":{"type":"string","description":"Previous tool call ID to inspect for tool_result_contains."}
			},
			"required":["id","kind"]
		}`),
	}
}

func (e *Engine) executeToolBatch(ctx context.Context, calls []model.ToolCallPart, snap app.AppState, ch chan<- LoopEvent) tool.ExecuteResult {
	results := make([]model.ToolResultPart, len(calls))
	displays := make([]string, len(calls))
	realCalls := make([]model.ToolCallPart, 0, len(calls))
	realIndexes := make([]int, 0, len(calls))

	for i, call := range calls {
		if call.Name != "PatchHandoffState" && call.Name != "CertifyFact" {
			realCalls = append(realCalls, call)
			realIndexes = append(realIndexes, i)
			continue
		}
		if call.Name == "PatchHandoffState" {
			results[i] = e.executeHandoffPatch(call)
		} else {
			results[i] = e.executeCertifyFact(call)
		}
	}

	var supplements []model.ContentPart
	if len(realCalls) > 0 {
		gateState := e.store.Snapshot().HandoffState
		filteredCalls, filteredIndexes, blocked := e.filterBlockedChangeCalls(realCalls, realIndexes, gateState)
		for idx, result := range blocked {
			results[idx] = result
		}
		realCalls = filteredCalls
		realIndexes = filteredIndexes
	}

	if len(realCalls) > 0 {
		progressCh := make(chan tool.ProgressEvent, 16)
		wrappedSnap := &progressSnapshot{StateSnapshot: snap, progressCh: progressCh, fileState: e.fileState}

		type execDone struct {
			result tool.ExecuteResult
		}
		doneCh := make(chan execDone, 1)
		go func() {
			doneCh <- execDone{result: e.orchestrator.Execute(ctx, realCalls, wrappedSnap)}
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
			idx := realIndexes[i]
			results[idx] = r
			displays[idx] = execResult.Displays[i]
		}
		supplements = execResult.Supplements
	}

	return tool.ExecuteResult{
		Results:     results,
		Displays:    displays,
		Supplements: supplements,
	}
}

func (e *Engine) filterBlockedChangeCalls(calls []model.ToolCallPart, indexes []int, state model.HandoffState) ([]model.ToolCallPart, []int, map[int]model.ToolResultPart) {
	if !e.isStateHandoffMode() || state.AllowsChanges() {
		return calls, indexes, nil
	}
	missing := state.ChangeGateMissing()
	blocked := make(map[int]model.ToolResultPart)
	allowedCalls := make([]model.ToolCallPart, 0, len(calls))
	allowedIndexes := make([]int, 0, len(indexes))
	for i, call := range calls {
		desc, ok := e.registry.Get(call.Name)
		if ok && desc.Flags().ReadOnly {
			allowedCalls = append(allowedCalls, call)
			allowedIndexes = append(allowedIndexes, indexes[i])
			continue
		}
		blocked[indexes[i]] = model.ToolResultPart{
			ToolCallID: call.ID,
			Content:    "change blocked: investigate and patch " + strings.Join(missing, ", ") + " before calling non-read-only tools",
			IsError:    true,
		}
	}
	return allowedCalls, allowedIndexes, blocked
}

func (e *Engine) executeHandoffPatch(call model.ToolCallPart) model.ToolResultPart {
	snap := e.store.Snapshot()
	state := snap.HandoffState
	if state.IsZero() {
		state = model.NewHandoffState("")
	}
	next, err := model.ApplyHandoffPatch(state, call.Input)
	if err != nil {
		return model.ToolResultPart{
			ToolCallID: call.ID,
			Content:    err.Error(),
			IsError:    true,
		}
	}
	e.store.Update(func(s *app.AppState) {
		s.HandoffState = next
	})
	return model.ToolResultPart{
		ToolCallID: call.ID,
		Content:    "handoff state patched",
	}
}

type certifyFactInput struct {
	ID             string               `json:"id"`
	Kind           string               `json:"kind"`
	Path           string               `json:"path,omitempty"`
	Contains       string               `json:"contains,omitempty"`
	Claim          string               `json:"claim,omitempty"`
	RequiredFields []string             `json:"required_fields,omitempty"`
	Selector       *certifyFactSelector `json:"selector,omitempty"`
	ToolCallID     string               `json:"tool_call_id,omitempty"`
}

type certifyFactSelector struct {
	Field  string `json:"field,omitempty"`
	Equals string `json:"equals,omitempty"`
}

func (e *Engine) executeCertifyFact(call model.ToolCallPart) model.ToolResultPart {
	var in certifyFactInput
	if err := json.Unmarshal(call.Input, &in); err != nil {
		return model.ToolResultPart{ToolCallID: call.ID, Content: "certify fact: invalid input: " + err.Error(), IsError: true}
	}
	if strings.TrimSpace(in.ID) == "" {
		return model.ToolResultPart{ToolCallID: call.ID, Content: "certify fact: id is required", IsError: true}
	}
	fact, err := e.certifyFact(in)
	if err != nil {
		return model.ToolResultPart{ToolCallID: call.ID, Content: "certify fact: " + err.Error(), IsError: true}
	}
	e.store.Update(func(s *app.AppState) {
		if s.HandoffState.IsZero() {
			s.HandoffState = model.NewHandoffState("")
		}
		s.HandoffState.AddCertifiedFact(fact)
	})
	data, _ := json.Marshal(fact)
	return model.ToolResultPart{ToolCallID: call.ID, Content: string(data)}
}

func (e *Engine) certifyFact(in certifyFactInput) (model.CertifiedFact, error) {
	switch strings.ToLower(strings.TrimSpace(in.Kind)) {
	case "file_contains":
		return e.certifyFileContains(in)
	case "json_shape", "jsonl_shape":
		return e.certifyJSONShape(in)
	case "tool_result_contains":
		return e.certifyToolResultContains(in)
	default:
		return model.CertifiedFact{}, fmt.Errorf("unsupported kind %q; supported kinds: file_contains, json_shape, jsonl_shape, tool_result_contains", in.Kind)
	}
}

func (e *Engine) certifyFileContains(in certifyFactInput) (model.CertifiedFact, error) {
	if strings.TrimSpace(in.Path) == "" {
		return model.CertifiedFact{}, fmt.Errorf("path is required")
	}
	if in.Contains == "" {
		return model.CertifiedFact{}, fmt.Errorf("contains is required")
	}
	path := resolveCertifyPath(e.store.Snapshot().CWD, in.Path)
	data, err := os.ReadFile(path)
	if err != nil {
		return model.CertifiedFact{}, err
	}
	if !strings.Contains(string(data), in.Contains) {
		return model.CertifiedFact{}, fmt.Errorf("%s does not contain requested text", path)
	}
	return model.CertifiedFact{
		ID:         in.ID,
		Kind:       "file_contains",
		Source:     path,
		Claim:      in.Claim,
		Evidence:   in.Contains,
		SampleHash: sha256Hex([]byte(in.Contains)),
		Verified:   true,
	}, nil
}

func (e *Engine) certifyJSONShape(in certifyFactInput) (model.CertifiedFact, error) {
	if strings.TrimSpace(in.Path) == "" {
		return model.CertifiedFact{}, fmt.Errorf("path is required")
	}
	if len(in.RequiredFields) == 0 {
		return model.CertifiedFact{}, fmt.Errorf("required_fields is required")
	}
	path := resolveCertifyPath(e.store.Snapshot().CWD, in.Path)
	file, err := os.Open(path)
	if err != nil {
		return model.CertifiedFact{}, err
	}
	defer file.Close()

	required := make(map[string]bool, len(in.RequiredFields))
	for _, f := range in.RequiredFields {
		required[f] = false
	}
	observed := make(map[string]bool)
	var matching int
	var sample []byte
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var obj map[string]any
		if err := json.Unmarshal(line, &obj); err != nil {
			continue
		}
		if !matchesCertifySelector(obj, in.Selector) {
			continue
		}
		matching++
		if sample == nil {
			sample = append([]byte(nil), line...)
		}
		for k := range obj {
			observed[k] = true
		}
		for k := range required {
			if _, ok := obj[k]; ok {
				required[k] = true
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return model.CertifiedFact{}, err
	}
	if matching == 0 {
		return model.CertifiedFact{}, fmt.Errorf("no JSONL records matched selector")
	}
	var missing []string
	for k, ok := range required {
		if !ok {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return model.CertifiedFact{}, fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
	}
	fields := make([]string, 0, len(observed))
	for k := range observed {
		fields = append(fields, k)
	}
	sort.Strings(fields)
	return model.CertifiedFact{
		ID:              in.ID,
		Kind:            "json_shape",
		Source:          path,
		Claim:           in.Claim,
		Evidence:        fmt.Sprintf("%d matching records with required fields %s", matching, strings.Join(in.RequiredFields, ", ")),
		Fields:          fields,
		MatchingRecords: matching,
		SampleHash:      sha256Hex(sample),
		Verified:        true,
	}, nil
}

func (e *Engine) certifyToolResultContains(in certifyFactInput) (model.CertifiedFact, error) {
	if strings.TrimSpace(in.ToolCallID) == "" {
		return model.CertifiedFact{}, fmt.Errorf("tool_call_id is required")
	}
	if in.Contains == "" {
		return model.CertifiedFact{}, fmt.Errorf("contains is required")
	}
	snap := e.store.Snapshot()
	for _, msg := range snap.Conversation.Messages {
		for _, part := range msg.Content {
			result, ok := part.(model.ToolResultPart)
			if !ok || result.ToolCallID != in.ToolCallID {
				continue
			}
			if !strings.Contains(result.Content, in.Contains) {
				return model.CertifiedFact{}, fmt.Errorf("tool result %s does not contain requested text", in.ToolCallID)
			}
			return model.CertifiedFact{
				ID:         in.ID,
				Kind:       "tool_result_contains",
				Source:     "tool_result:" + in.ToolCallID,
				Claim:      in.Claim,
				Evidence:   in.Contains,
				ToolCallID: in.ToolCallID,
				SampleHash: sha256Hex([]byte(result.Content)),
				Verified:   true,
			}, nil
		}
	}
	return model.CertifiedFact{}, fmt.Errorf("tool result %s not found", in.ToolCallID)
}

func resolveCertifyPath(cwd, path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(cwd, path))
}

func matchesCertifySelector(obj map[string]any, selector *certifyFactSelector) bool {
	if selector == nil || selector.Field == "" {
		return true
	}
	got, ok := obj[selector.Field]
	if !ok {
		return false
	}
	return fmt.Sprint(got) == selector.Equals
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}

func messageHasToolCall(msg model.Message) bool {
	for _, part := range msg.Content {
		if _, ok := part.(model.ToolCallPart); ok {
			return true
		}
	}
	return false
}

func messageHasToolResult(msg model.Message) bool {
	for _, part := range msg.Content {
		if _, ok := part.(model.ToolResultPart); ok {
			return true
		}
	}
	return false
}

// progressSnapshot wraps a StateSnapshot with a ProgressReporter for tool progress.
// Implements tool.ProgressSource via optional interface pattern (ADR-042).
type progressSnapshot struct {
	tool.StateSnapshot
	progressCh chan tool.ProgressEvent
	fileState  *tool.FileStateCache
}

func (p *progressSnapshot) Progress() tool.ProgressReporter {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: p.progressCh")
	return p.progressCh
}

func (p *progressSnapshot) ReadFileState() *tool.FileStateCache {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: p.fileState")
	return p.fileState
}

func (p *progressSnapshot) SessionID() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if id, ok := tool.SessionIDFrom(p.StateSnapshot); ok {
		observe.GlobalTrace("if: ok")
		observe.GlobalTrace("return: id")
		return id
	}
	observe.GlobalTrace("return: \"\"")
	return ""
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
	var bufferedToolInput strings.Builder
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

			if bufferedToolInput.Len() > 0 {
				observe.GlobalTrace("if: bufferedToolInput.Len() > 0")
				toolCalls[tc.ID].inputBuf.WriteString(bufferedToolInput.String())
				bufferedToolInput.Reset()
			}
		}

		if chunk.ToolCallInputDelta != nil {
			observe.GlobalTrace("if: chunk.ToolCallInputDelta != nil")
			id := chunk.ToolCallInputDelta.ToolCallID
			acc, ok := toolCalls[id]

			if !ok && id == "" {
				observe.GlobalTrace("if: !ok && id == \"\"")

				if len(toolOrder) > 0 {
					observe.GlobalTrace("if: len(toolOrder) > 0")

					lastID := toolOrder[len(toolOrder)-1]
					acc = toolCalls[lastID]
				} else {
					observe.GlobalTrace("else: len(toolOrder) > 0")

					bufferedToolInput.WriteString(chunk.ToolCallInputDelta.JSONDelta)
					continue
				}
			}

			if acc != nil {
				observe.GlobalTrace("if: acc != nil")
				acc.inputBuf.WriteString(chunk.ToolCallInputDelta.JSONDelta)
			} else if id != "" {

				observe.GlobalTrace("if: !ok")
				observe.GlobalTrace("return: model.Response{}, fmt.Errorf(\"input delta for unknown tool call %q\", chunk.To...")
				return model.Response{}, fmt.Errorf("input delta for unknown tool call %q", id)
			}
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
				observe.GlobalTrace("else-if: !json.Valid(raw) — marking as malformed")

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
