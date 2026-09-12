package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/mcp"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/shellrun"
	"github.com/artpar/pragma/internal/tools/applypatch"
	"github.com/artpar/pragma/internal/tools/websearch"
)

func (engine *Engine) runProviderToolsLoop(ctx context.Context, userMessage string, ch chan<- LoopEvent) {
	observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "exit")
	defer engine.runStopHook(ch)

	maxTurns := engine.config.MaxTurns
	if maxTurns < 0 {
		observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "if: maxTurns < 0")
		maxTurns = 0
	}
	warnTurn := turnBudgetWarnTurn(maxTurns)

	promptAt := time.Now()
	if err := engine.appendConversationMessage(model.Message{
		ID:        model.NewUUID(),
		Role:      model.RoleUser,
		Content:   []model.ContentPart{model.TextPart{Text: wallClockStamp(promptAt) + "\n" + userMessage}},
		Timestamp: promptAt,
	}); err != nil {
		observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "if: err != nil")
		ch <- ErrorEvent{Err: err}
		return
	}

	for turn := 0; maxTurns <= 0 || turn < maxTurns; turn++ {
		observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "for: turn < maxTurns")
		if err := ctx.Err(); err != nil {
			observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "if: err != nil")
			ch <- ErrorEvent{Err: fmt.Errorf("context cancelled: %w", model.ErrContextCancelled)}
			return
		}
		if turn == warnTurn {
			observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "if: turn == warnTurn")
			noticeAt := time.Now()
			if err := engine.appendConversationMessage(model.Message{
				ID:        model.NewUUID(),
				Role:      model.RoleUser,
				Content:   []model.ContentPart{model.TextPart{Text: wallClockStamp(noticeAt) + "\n" + turnBudgetNotice(turn, maxTurns)}},
				Timestamp: noticeAt,
			}); err != nil {
				observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "if: err != nil")
				ch <- ErrorEvent{Err: err}
				return
			}
		}

		snap := engine.store.Snapshot()
		resolvedModel := firstNonEmpty(snap.Model, snap.Conversation.Model, engine.config.Model)
		system := engine.WithCustomSystemPrompt(snap.Conversation.System)
		system = engine.systemWithMCPStatus(system)
		tools := providerToolDefs()
		tools = engine.withWebSearchTool(tools)
		tools = engine.withSubAgentTool(tools)
		tools = engine.withMCPToolDefs(ctx, tools)
		system = engine.systemWithPatchGuidance(system, tools)
		messages, err := engine.messagesForRequestChecked(snap.Conversation)
		if err != nil {
			observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "if: err != nil")
			ch <- ErrorEvent{Err: err}
			return
		}

		params := provider.RequestParams{
			Model:          resolvedModel,
			MaxTokens:      engine.config.MaxTokens,
			Messages:       messages,
			System:         system,
			Tools:          tools,
			Temperature:    engine.config.Temperature,
			Thinking:       engine.config.Thinking,
			ResponseSchema: engine.config.ResponseSchema,
		}
		ch <- ModelRequestEvent{Model: resolvedModel, Attempt: 1}
		response, err := engine.completeProviderToolsResponse(ctx, params, ch)
		if err != nil {
			observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "if: err != nil")
			ch <- ErrorEvent{Err: err}
			return
		}
		ch <- ModelResponseEvent{Model: resolvedModel, StopReason: response.StopReason}
		emitProviderToolsContent(response, ch)

		if err := engine.appendConversationMessage(model.Message{
			ID:        model.NewUUID(),
			Role:      model.RoleAssistant,
			Content:   response.Content,
			Timestamp: time.Now(),
		}); err != nil {
			observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "if: err != nil")
			ch <- ErrorEvent{Err: err}
			return
		}

		toolCalls := responseToolCalls(response)
		if response.StopReason == model.StopMaxTokens && len(toolCalls) == 0 {
			observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "if: response.StopReason == model.StopMaxTokens && len(toolCalls) == 0")
			ch <- ErrorEvent{Err: errors.New("final response truncated by max_tokens output limit before completion; the conversation is preserved — continue with a new prompt or --resume")}
			return
		}
		if response.StopReason != model.StopToolUse && len(toolCalls) == 0 {
			observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "if: response.StopReason != model.StopToolUse && len(toolCalls) == 0")
			ch <- TurnCompleteEvent{Response: response, StopReason: response.StopReason}
			return
		}
		if len(toolCalls) == 0 {
			observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "if: len(toolCalls) == 0")
			ch <- ErrorEvent{Err: errors.New("provider requested tool use but returned no tool calls")}
			return
		}

		for _, call := range toolCalls {
			observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "range toolCalls")
			ch <- ToolCallEvent{Call: call}
		}
		results := make([]model.ContentPart, len(toolCalls))
		var pending sync.WaitGroup
		for i, call := range toolCalls {
			observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "range toolCalls")
			if call.Name == applypatch.ToolName {
				observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "if: call.Name == applypatch.ToolName")
				continue
			}
			pending.Add(1)
			go func(i int, call model.ToolCallPart) {
				defer pending.Done()
				result := engine.executeProviderToolCall(ctx, call)
				results[i] = result
				ch <- ToolResultEvent{Result: result}
			}(i, call)
		}
		for i, call := range toolCalls {
			observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "range toolCalls")
			if call.Name != applypatch.ToolName {
				observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "if: call.Name != applypatch.ToolName")
				continue
			}
			result := engine.executeProviderToolCall(ctx, call)
			results[i] = result
			ch <- ToolResultEvent{Result: result}
		}
		pending.Wait()
		if err := engine.appendConversationMessage(model.Message{
			ID:        model.NewUUID(),
			Role:      model.RoleUser,
			Content:   results,
			Timestamp: time.Now(),
		}); err != nil {
			observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "if: err != nil")
			ch <- ErrorEvent{Err: err}
			return
		}
		if err := engine.appendConversationMessage(model.Message{
			ID:        model.NewUUID(),
			Role:      model.RoleUser,
			Content:   []model.ContentPart{model.TextPart{Text: wallClockStamp(time.Now())}},
			Timestamp: time.Now(),
		}); err != nil {
			observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "if: err != nil")
			ch <- ErrorEvent{Err: err}
			return
		}

		engine.drainPendingUserInputs()

		if engine.autoTracker != nil {
			observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "if: engine.autoTracker != nil")
			engine.autoTracker.IncrementTurn()
		}
	}

	ch <- ErrorEvent{Err: fmt.Errorf("provider tools loop exceeded maximum of %d turns", maxTurns)}
}

// AppendUserInput delivers operator text submitted while a turn is running
// (INT-001): it is appended to the conversation as a regular stamped user
// message and the loop's next request folds it in. When the conversation
// tail is an assistant tool_use message still awaiting its results, the
// message parks instead — a user message between a tool call and its
// results would serialize before the tool results and break tool_result
// pairing. Parked messages flush at the loop's next safe point
// (drainPendingUserInputs, after each companion append).
func (engine *Engine) AppendUserInput(text string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	text = strings.TrimSpace(text)
	if text == "" {
		observe.GlobalTrace("return: errors.New(\"empty user input\")")
		return errors.New("empty user input")
	}
	now := time.Now()
	msg := model.Message{
		ID:        model.NewUUID(),
		Role:      model.RoleUser,
		Content:   []model.ContentPart{model.TextPart{Text: wallClockStamp(now) + "\n" + text}},
		Timestamp: now,
	}
	engine.pendingUserInputsMu.Lock()
	defer engine.pendingUserInputsMu.Unlock()
	appended := false
	engine.store.Update(func(s *app.AppState) {
		if conversationTailHasDanglingToolUse(s.Conversation) {
			engine.pendingUserInputs = append(engine.pendingUserInputs, msg)
			return
		}
		s.Conversation.Append(msg)
		appended = true
	})
	if !appended {
		observe.GlobalTrace("return: nil (parked for the next safe point)")
		observe.GlobalTrace("return: nil")
		return nil
	}
	engine.emitMessageAppended(msg)
	observe.GlobalTrace("return: engine.checkpointSession()")
	return engine.checkpointSession()
}

// drainPendingUserInputs flushes parked operator messages into the
// conversation at a pairing-safe point (after the companion append). It
// holds pendingUserInputsMu for the whole flush so a concurrent
// AppendUserInput cannot interleave: parked messages land in submission
// order, before any later direct append.
func (engine *Engine) drainPendingUserInputs() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	engine.pendingUserInputsMu.Lock()
	defer engine.pendingUserInputsMu.Unlock()
	for len(engine.pendingUserInputs) > 0 {
		observe.GlobalTrace("for: len(engine.pendingUserInputs) > 0")
		msg := engine.pendingUserInputs[0]
		if err := engine.appendConversationMessage(msg); err != nil {
			observe.GlobalTrace("if: err != nil")

			return
		}
		engine.pendingUserInputs = engine.pendingUserInputs[1:]
	}
}

// conversationTailHasDanglingToolUse reports whether the conversation's
// last message is an assistant message carrying tool calls whose results
// have not been appended yet (the mid-tool-execution window).
func conversationTailHasDanglingToolUse(conv model.Conversation) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	n := len(conv.Messages)
	if n == 0 {
		observe.GlobalTrace("return: false")
		return false
	}
	tail := conv.Messages[n-1]
	if tail.Role != model.RoleAssistant {
		observe.GlobalTrace("return: false")
		return false
	}
	for _, part := range tail.Content {
		observe.GlobalTrace("range tail.Content")
		if _, ok := part.(model.ToolCallPart); ok {
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

// turnBudgetNoticeMarker prefixes the in-conversation turn-budget warning.
const turnBudgetNoticeMarker = "[pragma turn budget]"

// wallClockStampMarker prefixes the model-visible wall-clock append stamp
// (CLK-001): every user-role message this loop appends carries the time it
// was added, so the model can perceive blocking, latency, and session age —
// the store's Message.Timestamp never reaches the wire on its own.
const wallClockStampMarker = "[pragma wall-clock "

// wallClockStamp returns the append-time stamp line for engine-appended
// user messages (CLK-001). RFC3339 keeps the timezone explicit. The stamp
// rides as a first line inside text-only appends (prompt, turn-budget
// notice); after a tool-results batch it rides as a companion user message
// — the position and wire shape the TURN-001 notice proved live — because a
// text part inside the results message would serialize before its tool
// results (an unproven user-before-tools order on this route).
func wallClockStamp(now time.Time) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: marker + now.Format(time.RFC3339) + \"]\"")
	observe.GlobalTrace("return: wallClockStampMarker + now.Format(time.RFC3339) + \"]\"")
	return wallClockStampMarker + now.Format(time.RFC3339) + "]"
}

// turnBudgetWarnTurn returns the 0-based loop iteration at which the
// turn-budget notice is appended, or -1 when no warning window exists
// (TURN-001). The window is maxTurns/5 turns remaining — 20 at the default
// 100 (TURN-002: the first live run showed 10 was too small to complete a
// wrap-up) — with at least half the budget for small budgets (window < 5),
// so the model always has room to act before the cap.
func turnBudgetWarnTurn(maxTurns int) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if maxTurns <= 1 {
		observe.GlobalTrace("if: maxTurns <= 1")
		observe.GlobalTrace("return: -1")
		return -1
	}
	window := maxTurns / 5
	if window < 5 {
		observe.GlobalTrace("if: window < 5")
		window = maxTurns / 2
	}
	observe.GlobalTrace("return: maxTurns - window")
	return maxTurns - window
}

// turnBudgetNotice is the warning shown to the model in-conversation as the
// turn budget approaches exhaustion (TURN-001). The cap itself is unchanged;
// continuation already exists (a new prompt resumes the preserved
// conversation) and the explicit override exists (--max-turns).
func turnBudgetNotice(used, maxTurns int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	remaining := maxTurns - used
	observe.GlobalTrace("return: notice")
	observe.GlobalTrace("return: fmt.Sprintf(\"%s %d of %d provider-tools turns used; %d remain. The loop stops...")
	return fmt.Sprintf("%s %d of %d provider-tools turns used; %d remain. The loop stops when the budget is exhausted; the conversation is preserved and a new prompt continues it. Prioritize now: finish the current step, then either complete the task or write a concise handoff (state, decisions, evidence locations, next action) so a continued session can resume without redoing work.",
		turnBudgetNoticeMarker, used, maxTurns, remaining)
}

func (engine *Engine) completeProviderToolsResponse(ctx context.Context, params provider.RequestParams, ch chan<- LoopEvent) (model.Response, error) {
	observe.TraceCtx(ctx, "query", "Engine.completeProviderToolsResponse", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.completeProviderToolsResponse", "exit")
	const maxRetries = 10
	const maxConsecutiveOverloaded = 3
	var consecutiveOverloaded int

	for attempt := range maxRetries + 1 {
		observe.TraceCtx(ctx, "query", "Engine.completeProviderToolsResponse", "range maxRetries + 1")
		if attempt > 0 {
			observe.TraceCtx(ctx, "query", "Engine.completeProviderToolsResponse", "if: attempt > 0")
			ch <- ModelRequestEvent{Model: params.Model, Attempt: attempt + 1}
		}
		response, err := engine.provider.Complete(ctx, params)
		if err == nil {
			observe.TraceCtx(ctx, "query", "Engine.completeProviderToolsResponse", "if: err == nil")
			observe.TraceCtx(ctx, "query", "Engine.completeProviderToolsResponse", "return: response, nil")
			return response, nil
		}
		classified := ClassifyStreamError(err)
		if classified.Kind == ErrorKindOverloaded {
			observe.TraceCtx(ctx, "query", "Engine.completeProviderToolsResponse", "if: classified.Kind == ErrorKindOverloaded")
			consecutiveOverloaded++
			if consecutiveOverloaded >= maxConsecutiveOverloaded {
				observe.TraceCtx(ctx, "query", "Engine.completeProviderToolsResponse", "if: consecutiveOverloaded >= maxConsecutiveOverloaded")
				observe.TraceCtx(ctx, "query", "Engine.completeProviderToolsResponse", "return: model.Response{}, fmt.Errorf(\"repeated overloaded errors\")")
				return model.Response{}, fmt.Errorf("repeated overloaded errors")
			}
		} else {
			observe.TraceCtx(ctx, "query", "Engine.completeProviderToolsResponse", "else: classified.Kind == ErrorKindOverloaded")
			consecutiveOverloaded = 0
		}
		if !classified.Retryable || attempt >= maxRetries {
			observe.TraceCtx(ctx, "query", "Engine.completeProviderToolsResponse", "if: !classified.Retryable || attempt >= maxRetries")
			observe.TraceCtx(ctx, "query", "Engine.completeProviderToolsResponse", "return: model.Response{}, classified.Err")
			return model.Response{}, classified.Err
		}
		delay := classified.RetryAfter
		if delay == 0 {
			observe.TraceCtx(ctx, "query", "Engine.completeProviderToolsResponse", "if: delay == 0")
			delay = retryDelay(attempt)
		}
		ch <- RetryEvent{
			Attempt:     attempt + 1,
			MaxAttempts: maxRetries + 1,
			Delay:       delay,
			Kind:        classified.Kind,
			ErrorMsg:    err.Error(),
		}
		select {
		case <-time.After(delay):
			observe.TraceCtx(ctx, "query", "Engine.completeProviderToolsResponse", "select: <-time.After(delay)")
		case <-ctx.Done():
			observe.TraceCtx(ctx, "query", "Engine.completeProviderToolsResponse", "select: <-ctx.Done()")
			return model.Response{}, fmt.Errorf("context cancelled during retry: %w", model.ErrContextCancelled)
		}
	}
	observe.TraceCtx(ctx, "query", "Engine.completeProviderToolsResponse", "return: model.Response{}, errors.New(\"model request failed\")")
	return model.Response{}, errors.New("model request failed")
}

func providerToolDefs() []model.ToolDef {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: []model.ToolDef{\n\t{\n\t\tName:\t\t\"Bash\",\n\t\tDescription:\t\"Run a bash command in th...")
	return []model.ToolDef{
		{
			Name:        "Bash",
			Description: "Run a bash command in the current workspace. Use for inspection, tests, builds, and shell actions.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"cmd":{"type":"string","description":"The bash command to run."}},"required":["cmd"],"additionalProperties":false}`),
		},
		{
			Name:        applypatch.ToolName,
			Description: "Apply a structured patch to files in the current workspace.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"patch":{"type":"string","description":"The apply_patch body, starting with *** Begin Patch and ending with *** End Patch."}},"required":["patch"],"additionalProperties":false}`),
		},
	}
}

func emitProviderToolsContent(response model.Response, ch chan<- LoopEvent) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, part := range response.Content {
		observe.GlobalTrace("range response.Content")
		switch p := part.(type) {
		case model.TextPart:
			observe.GlobalTrace("typecase: model.TextPart")
			if p.Text != "" {
				ch <- TextEvent{Text: p.Text}
			}
		case model.ThinkingPart:
			observe.GlobalTrace("typecase: model.ThinkingPart")
			if p.Text != "" {
				ch <- ThinkingEvent{Text: p.Text}
			}
		}
	}
}

func responseToolCalls(response model.Response) []model.ToolCallPart {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var calls []model.ToolCallPart
	for _, part := range response.Content {
		observe.GlobalTrace("range response.Content")
		if call, ok := part.(model.ToolCallPart); ok {
			observe.GlobalTrace("if: ok")
			calls = append(calls, call)
		}
	}
	observe.GlobalTrace("return: calls")
	return calls
}

func (engine *Engine) executeProviderToolCall(ctx context.Context, call model.ToolCallPart) model.ToolResultPart {
	observe.TraceCtx(ctx, "query", "Engine.executeProviderToolCall", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.executeProviderToolCall", "exit")
	if isMCPToolCall(engine, call.Name) {
		observe.TraceCtx(ctx, "query", "Engine.executeProviderToolCall", "if: isMCPToolCall(engine, call.Name)")
		observe.TraceCtx(ctx, "query", "Engine.executeProviderToolCall", "return: engine.executeMCPToolCall(ctx, call)")
		return engine.executeMCPToolCall(ctx, call)
	}
	switch call.Name {
	case "Bash":
		observe.TraceCtx(ctx, "query", "Engine.executeProviderToolCall", "case: \"Bash\"")
		return engine.executeProviderBashTool(ctx, call)
	case websearch.ToolName:
		observe.TraceCtx(ctx, "query", "Engine.executeProviderToolCall", "case: websearch.ToolName")
		if engine.config.WebSearch == nil {
			observe.TraceCtx(ctx, "query", "Engine.executeProviderToolCall", "if: engine.config.WebSearch == nil")
			observe.TraceCtx(ctx, "query", "Engine.executeProviderToolCall", "return: model.ToolResultPart{ToolCallID: call.ID, Content: fmt.Sprintf(\"unknown tool ...")
			return model.ToolResultPart{ToolCallID: call.ID, Content: fmt.Sprintf("unknown tool %q", call.Name), IsError: true}
		}
		return engine.executeWebSearchTool(ctx, call)
	case AgentToolName:
		observe.TraceCtx(ctx, "query", "Engine.executeProviderToolCall", "case: AgentToolName")
		if engine.config.DisableSubAgents {
			observe.TraceCtx(ctx, "query", "Engine.executeProviderToolCall", "if: engine.config.DisableSubAgents")
			observe.TraceCtx(ctx, "query", "Engine.executeProviderToolCall", "return: model.ToolResultPart{ToolCallID: call.ID, Content: fmt.Sprintf(\"unknown tool ...")
			return model.ToolResultPart{ToolCallID: call.ID, Content: fmt.Sprintf("unknown tool %q", call.Name), IsError: true}
		}
		return engine.executeSubAgentTool(ctx, call)
	case applypatch.ToolName:
		observe.TraceCtx(ctx, "query", "Engine.executeProviderToolCall", "case: applypatch.ToolName")
		return engine.executeProviderApplyPatchTool(ctx, call)
	default:
		observe.TraceCtx(ctx, "query", "Engine.executeProviderToolCall", "default")
		return model.ToolResultPart{ToolCallID: call.ID, Content: fmt.Sprintf("unknown tool %q", call.Name), IsError: true}
	}
}

// isMCPToolCall reports whether a tool name is an injected MCP tool the
// engine can route ("mcp__" prefix and a routing hook configured).
func isMCPToolCall(engine *Engine, name string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: mcp.IsMCPTool(name) && engine.config.MCPCallTool != nil")
	return mcp.IsMCPTool(name) && engine.config.MCPCallTool != nil
}

// withWebSearchTool appends the WebSearch tool definition when its executor
// is configured. Built-in names win on collision.
func (engine *Engine) withWebSearchTool(tools []model.ToolDef) []model.ToolDef {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if engine.config.WebSearch == nil {
		observe.GlobalTrace("if: engine.config.WebSearch == nil")
		observe.GlobalTrace("return: tools")
		return tools
	}
	for _, tool := range tools {
		observe.GlobalTrace("range tools")
		if tool.Name == websearch.ToolName {
			observe.GlobalTrace("if: tool.Name == websearch.ToolName")
			observe.GlobalTrace("return: tools")
			return tools
		}
	}
	observe.GlobalTrace("return: append(tools, websearch.ToolDef())")
	return append(tools, websearch.ToolDef())
}

// executeWebSearchTool routes a WebSearch call through the configured
// executor; the result pairs with the call like any built-in tool.
func (engine *Engine) executeWebSearchTool(ctx context.Context, call model.ToolCallPart) model.ToolResultPart {
	observe.TraceCtx(ctx, "query", "Engine.executeWebSearchTool", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.executeWebSearchTool", "exit")
	output, err := engine.config.WebSearch(ctx, call.Input)
	if err != nil {
		observe.TraceCtx(ctx, "query", "Engine.executeWebSearchTool", "if: err != nil")
		observe.TraceCtx(ctx, "query", "Engine.executeWebSearchTool", "return: model.ToolResultPart{ ... IsError: true }")
		observe.TraceCtx(ctx, "query", "Engine.executeWebSearchTool", "return: model.ToolResultPart{ToolCallID: call.ID, Content: fmt.Sprintf(\"WebSearch fai...")
		return model.ToolResultPart{ToolCallID: call.ID, Content: fmt.Sprintf("WebSearch failed: %v", err), IsError: true}
	}
	observe.TraceCtx(ctx, "query", "Engine.executeWebSearchTool", "return: model.ToolResultPart{ToolCallID: call.ID, Content: output}")
	return model.ToolResultPart{ToolCallID: call.ID, Content: output}
}

// withMCPToolDefs appends the injected MCP tool definitions to the built-in
// tool list. Built-in names win on collision (an MCP tool cannot shadow
// Bash or apply_patch); duplicate MCP names are skipped (first wins).
func (engine *Engine) withMCPToolDefs(ctx context.Context, tools []model.ToolDef) []model.ToolDef {
	observe.TraceCtx(ctx, "query", "Engine.withMCPToolDefs", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.withMCPToolDefs", "exit")
	if engine.config.MCPToolDefs == nil {
		observe.TraceCtx(ctx, "query", "Engine.withMCPToolDefs", "if: engine.config.MCPToolDefs == nil")
		observe.TraceCtx(ctx, "query", "Engine.withMCPToolDefs", "return: tools")
		return tools
	}
	mcpDefs := engine.config.MCPToolDefs(ctx)
	if len(mcpDefs) == 0 {
		observe.TraceCtx(ctx, "query", "Engine.withMCPToolDefs", "if: len(mcpDefs) == 0")
		observe.TraceCtx(ctx, "query", "Engine.withMCPToolDefs", "return: tools")
		return tools
	}
	seen := make(map[string]bool, len(tools)+len(mcpDefs))
	for _, tool := range tools {
		observe.TraceCtx(ctx, "query", "Engine.withMCPToolDefs", "range tools")
		seen[tool.Name] = true
	}
	for _, def := range mcpDefs {
		observe.TraceCtx(ctx, "query", "Engine.withMCPToolDefs", "range mcpDefs")
		if seen[def.Name] {
			observe.TraceCtx(ctx, "query", "Engine.withMCPToolDefs", "if: seen[def.Name]")
			continue
		}
		seen[def.Name] = true
		tools = append(tools, def)
	}
	observe.TraceCtx(ctx, "query", "Engine.withMCPToolDefs", "return: tools")
	return tools
}

// executeMCPToolCall routes an injected MCP tool call to its server through
// the configured hook. The result pairs with the call (ToolCallID) like any
// built-in tool result.
func (engine *Engine) executeMCPToolCall(ctx context.Context, call model.ToolCallPart) model.ToolResultPart {
	observe.TraceCtx(ctx, "query", "Engine.executeMCPToolCall", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.executeMCPToolCall", "exit")
	output, err := engine.config.MCPCallTool(ctx, call.Name, call.Input)
	if err != nil {
		observe.TraceCtx(ctx, "query", "Engine.executeMCPToolCall", "if: err != nil")
		observe.TraceCtx(ctx, "query", "Engine.executeMCPToolCall", "return: model.ToolResultPart{ ... IsError: true }")
		observe.TraceCtx(ctx, "query", "Engine.executeMCPToolCall", "return: model.ToolResultPart{ToolCallID: call.ID, Content: fmt.Sprintf(\"mcp tool %s f...")
		return model.ToolResultPart{ToolCallID: call.ID, Content: fmt.Sprintf("mcp tool %s failed: %v", call.Name, err), IsError: true}
	}
	observe.TraceCtx(ctx, "query", "Engine.executeMCPToolCall", "return: model.ToolResultPart{ToolCallID: call.ID, Content: output}")
	return model.ToolResultPart{ToolCallID: call.ID, Content: output}
}

func (engine *Engine) executeProviderBashTool(ctx context.Context, call model.ToolCallPart) model.ToolResultPart {
	observe.TraceCtx(ctx, "query", "Engine.executeProviderBashTool", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.executeProviderBashTool", "exit")
	var input struct {
		Cmd     string `json:"cmd"`
		Command string `json:"command"` // INT-002: observed emission alias
	}
	if err := json.Unmarshal(call.Input, &input); err != nil {
		observe.TraceCtx(ctx, "query", "Engine.executeProviderBashTool", "if: err != nil")
		observe.TraceCtx(ctx, "query", "Engine.executeProviderBashTool", "return: model.ToolResultPart{ToolCallID: call.ID, Content: fmt.Sprintf(\"invalid Bash ...")
		return model.ToolResultPart{ToolCallID: call.ID, Content: fmt.Sprintf("invalid Bash input: %v", err), IsError: true}
	}
	cmd := input.Cmd
	if strings.TrimSpace(cmd) == "" {
		observe.TraceCtx(ctx, "query", "Engine.executeProviderBashTool", "if: strings.TrimSpace(cmd) == \"\"")
		cmd = input.Command
	}
	if strings.TrimSpace(cmd) == "" {
		observe.TraceCtx(ctx, "query", "Engine.executeProviderBashTool", "if: strings.TrimSpace(cmd) == \"\"")
		// INT-002: echo the received keys so the model can self-correct in
		// one turn instead of rediscovering the cause from a bare error.
		var received []string
		var keys map[string]json.RawMessage
		if json.Unmarshal(call.Input, &keys) == nil {
			observe.TraceCtx(ctx, "query", "Engine.executeProviderBashTool", "if: json.Unmarshal(call.Input, &keys) == nil")
			for k := range keys {
				observe.TraceCtx(ctx, "query", "Engine.executeProviderBashTool", "range keys")
				received = append(received, k)
			}
		}
		observe.TraceCtx(ctx, "query", "Engine.executeProviderBashTool", "return: model.ToolResultPart{ToolCallID: call.ID, Content: \"Bash input requires a non-e...")
		observe.TraceCtx(ctx, "query", "Engine.executeProviderBashTool", "return: model.ToolResultPart{ToolCallID: call.ID, Content: fmt.Sprintf(\"Bash input re...")
		return model.ToolResultPart{ToolCallID: call.ID, Content: fmt.Sprintf("Bash input requires a non-empty command under key \"cmd\"; received keys: %v", received), IsError: true}
	}
	snap := engine.store.Snapshot()
	result, err := shellrun.Execute(ctx, shellrun.Options{
		Command:            cmd,
		WorkDir:            snap.CWD,
		Timeout:            pragmaLoopCommandTimeout,
		ForegroundWait:     pragmaLoopForegroundWait,
		BaseDirName:        "pragma-provider-tools-bash",
		UsePipefail:        true,
		UseErrexit:         true,
		RunningOutputLines: pragmaLoopRunningOutputLines,
	})
	if err != nil {
		observe.TraceCtx(ctx, "query", "Engine.executeProviderBashTool", "if: err != nil")
		observe.TraceCtx(ctx, "query", "Engine.executeProviderBashTool", "return: model.ToolResultPart{ToolCallID: call.ID, Content: err.Error(), IsError: true}")
		return model.ToolResultPart{ToolCallID: call.ID, Content: err.Error(), IsError: true}
	}
	content := fmt.Sprintf("Exit code: %d\n%s", result.ExitCode, result.Output)
	if result.Running {
		observe.TraceCtx(ctx, "query", "Engine.executeProviderBashTool", "if: result.Running")
		content = result.Output
	}
	observe.TraceCtx(ctx, "query", "Engine.executeProviderBashTool", "return: model.ToolResultPart{ToolCallID: call.ID, Content: content, IsError: result.E...")
	return model.ToolResultPart{ToolCallID: call.ID, Content: content, IsError: result.ExitCode != 0 && !result.Running}
}

func (engine *Engine) executeProviderApplyPatchTool(ctx context.Context, call model.ToolCallPart) model.ToolResultPart {
	observe.TraceCtx(ctx, "query", "Engine.executeProviderApplyPatchTool", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.executeProviderApplyPatchTool", "exit")
	var input applypatch.Input
	if err := json.Unmarshal(call.Input, &input); err != nil {
		observe.TraceCtx(ctx, "query", "Engine.executeProviderApplyPatchTool", "if: err != nil")
		observe.TraceCtx(ctx, "query", "Engine.executeProviderApplyPatchTool", "return: model.ToolResultPart{ToolCallID: call.ID, Content: fmt.Sprintf(\"invalid apply...")
		return model.ToolResultPart{ToolCallID: call.ID, Content: fmt.Sprintf("invalid apply_patch input: %v", err), IsError: true}
	}
	if strings.TrimSpace(input.Patch) == "" {
		observe.TraceCtx(ctx, "query", "Engine.executeProviderApplyPatchTool", "if: strings.TrimSpace(input.Patch) == \"\"")
		observe.TraceCtx(ctx, "query", "Engine.executeProviderApplyPatchTool", "return: model.ToolResultPart{ToolCallID: call.ID, Content: \"apply_patch input require...")
		return model.ToolResultPart{ToolCallID: call.ID, Content: "apply_patch input requires non-empty patch", IsError: true}
	}
	snap := engine.store.Snapshot()
	result, err := applypatch.ApplyPatchText(ctx, input.Patch, snap.CWD)
	if err != nil {
		observe.TraceCtx(ctx, "query", "Engine.executeProviderApplyPatchTool", "if: err != nil")
		observe.TraceCtx(ctx, "query", "Engine.executeProviderApplyPatchTool", "return: model.ToolResultPart{ToolCallID: call.ID, Content: err.Error(), IsError: true}")
		return model.ToolResultPart{ToolCallID: call.ID, Content: err.Error(), IsError: true}
	}
	observe.TraceCtx(ctx, "query", "Engine.executeProviderApplyPatchTool", "return: model.ToolResultPart{ToolCallID: call.ID, Content: result.Content}")
	return model.ToolResultPart{ToolCallID: call.ID, Content: result.Content}
}
