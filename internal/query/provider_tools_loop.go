package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/shellrun"
	"github.com/artpar/pragma/internal/tools/applypatch"
)

func (engine *Engine) runProviderToolsLoop(ctx context.Context, userMessage string, ch chan<- LoopEvent) {
	observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "exit")
	defer engine.runStopHook(ch)

	maxTurns := engine.config.MaxTurns
	if maxTurns <= 0 {
		observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "if: maxTurns <= 0")
		maxTurns = DefaultMaxTurns
	}

	if err := engine.appendConversationMessage(model.Message{
		ID:        model.NewUUID(),
		Role:      model.RoleUser,
		Content:   []model.ContentPart{model.TextPart{Text: userMessage}},
		Timestamp: time.Now(),
	}); err != nil {
		observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "if: err != nil")
		ch <- ErrorEvent{Err: err}
		return
	}

	for turn := 0; turn < maxTurns; turn++ {
		observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "for: turn < maxTurns")
		if err := ctx.Err(); err != nil {
			observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "if: err != nil")
			ch <- ErrorEvent{Err: fmt.Errorf("context cancelled: %w", model.ErrContextCancelled)}
			return
		}

		snap := engine.store.Snapshot()
		resolvedModel := firstNonEmpty(snap.Model, snap.Conversation.Model, engine.config.Model)
		system := engine.WithCustomSystemPrompt(snap.Conversation.System)
		system = engine.systemWithMCPStatus(system)
		tools := providerToolDefs()
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

		results := make([]model.ContentPart, 0, len(toolCalls))
		for _, call := range toolCalls {
			observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "range toolCalls")
			ch <- ToolCallEvent{Call: call}
			result := engine.executeProviderToolCall(ctx, call)
			ch <- ToolResultEvent{Result: result}
			results = append(results, result)
		}
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

		if engine.autoTracker != nil {
			observe.TraceCtx(ctx, "query", "Engine.runProviderToolsLoop", "if: engine.autoTracker != nil")
			engine.autoTracker.IncrementTurn()
		}
	}

	ch <- ErrorEvent{Err: fmt.Errorf("provider tools loop exceeded maximum of %d turns", maxTurns)}
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
	switch call.Name {
	case "Bash":
		observe.TraceCtx(ctx, "query", "Engine.executeProviderToolCall", "case: \"Bash\"")
		return engine.executeProviderBashTool(ctx, call)
	case applypatch.ToolName:
		observe.TraceCtx(ctx, "query", "Engine.executeProviderToolCall", "case: applypatch.ToolName")
		return engine.executeProviderApplyPatchTool(ctx, call)
	default:
		observe.TraceCtx(ctx, "query", "Engine.executeProviderToolCall", "default")
		return model.ToolResultPart{ToolCallID: call.ID, Content: fmt.Sprintf("unknown tool %q", call.Name), IsError: true}
	}
}

func (engine *Engine) executeProviderBashTool(ctx context.Context, call model.ToolCallPart) model.ToolResultPart {
	observe.TraceCtx(ctx, "query", "Engine.executeProviderBashTool", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.executeProviderBashTool", "exit")
	var input struct {
		Cmd string `json:"cmd"`
	}
	if err := json.Unmarshal(call.Input, &input); err != nil {
		observe.TraceCtx(ctx, "query", "Engine.executeProviderBashTool", "if: err != nil")
		observe.TraceCtx(ctx, "query", "Engine.executeProviderBashTool", "return: model.ToolResultPart{ToolCallID: call.ID, Content: fmt.Sprintf(\"invalid Bash ...")
		return model.ToolResultPart{ToolCallID: call.ID, Content: fmt.Sprintf("invalid Bash input: %v", err), IsError: true}
	}
	if strings.TrimSpace(input.Cmd) == "" {
		observe.TraceCtx(ctx, "query", "Engine.executeProviderBashTool", "if: strings.TrimSpace(input.Cmd) == \"\"")
		observe.TraceCtx(ctx, "query", "Engine.executeProviderBashTool", "return: model.ToolResultPart{ToolCallID: call.ID, Content: \"Bash input requires non-e...")
		return model.ToolResultPart{ToolCallID: call.ID, Content: "Bash input requires non-empty cmd", IsError: true}
	}
	snap := engine.store.Snapshot()
	result, err := shellrun.Execute(ctx, shellrun.Options{
		Command:            input.Cmd,
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
