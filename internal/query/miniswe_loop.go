package query

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/shellrun"
	"github.com/artpar/pragma/internal/tools/applypatch"
)

const pragmaLoopSystemPrompt = `Pragma loop mode is a shell-action transport.

Follow the active task and persona instructions. This wrapper only defines how
to send the next shell action.

When you need to inspect or change the system, your response must contain one
fenced bash code block with one command or one shell script. Do not write prose,
analysis sections, headings, or bullets outside the bash code block.
When you are done and want to answer the user, write the final answer directly
with no fenced bash code block unless the active runtime contract gives a
different completion signal. Do not use a bash block to end the turn unless the
active runtime contract requires it.
When you need to ask the user a question, ask it directly with no fenced bash
code block. Do not use echo or any other command to ask user questions.
Use commands, command output, and required task artifacts for reasoning and evidence.
Format your response as shown in <format_example>.

<format_example>
` + "```bash" + `
your_command_here
` + "```" + `
</format_example>

Failure to follow these rules will cause your response to be rejected.`

func PragmaLoopSystemPrompt() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: pragmaLoopSystemPrompt")
	return pragmaLoopSystemPrompt
}

const pragmaLoopInstanceSuffix = `

<system_information>
Current working directory: %s
%s
</system_information>
`

const pragmaLoopFormatErrorTemplate = `Please always provide EXACTLY ONE bash action in triple backticks and no prose outside the code block when you need a shell action. Found %d actions.
If you want to end the task, write the final answer directly with no fenced bash code block.
If you need to ask the user a question, ask it directly with no fenced bash code block; do not use echo.
Else, please format your response exactly as follows:

<response_example>
` + "```bash" + `
<action>
` + "```" + `
</response_example>

Note: In rare cases, if you need to reference a similar format in your command, you might have
to proceed in two steps, first writing TRIPLEBACKTICKSBASH, then replacing them with ` + "```bash" + `.`

var pragmaLoopCommandTimeout = 300 * time.Second
var pragmaLoopForegroundWait = 300 * time.Second

const pragmaLoopRunningOutputLines = 100

const pragmaLoopMaxNoActionRetries = 3

type pragmaLoopRunConfig struct {
	System              model.SystemPrompt
	UserMessage         string
	CompletionCheck     PragmaLoopCompletionCheck
	MessageStartIndexes []int
	MaxTurns            int
}

type pragmaLoopTurnRequest struct {
	Model  string
	Params provider.RequestParams
}

type pragmaLoopActionKind int

const (
	pragmaLoopActionFinal pragmaLoopActionKind = iota
	pragmaLoopActionBash
	pragmaLoopActionInvalid
	pragmaLoopActionNativeToolCall
)

type pragmaLoopAction struct {
	Kind  pragmaLoopActionKind
	Count int
	Bash  string
}

type pragmaLoopAssistantTurn struct {
	Response      model.Response
	Text          string
	ReplayContent []model.ContentPart
	Action        pragmaLoopAction
}

type pragmaLoopCommandResult struct {
	Command   string
	Result    pragmaLoopBashResult
	TimedOut  bool
	Submitted bool
}

type pragmaLoopObservation struct {
	Text     string
	Complete bool
}

type pragmaLoopCommandPolicy struct{}

type pragmaLoopCommandRejection struct {
	Reason string
}

func (engine *Engine) runPragmaLoop(ctx context.Context, userMessage string, ch chan<- LoopEvent) {
	observe.TraceCtx(ctx, "query", "Engine.runPragmaLoop", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.runPragmaLoop", "exit")
	snap := engine.store.Snapshot()
	system := model.SystemPrompt{Blocks: []model.SystemBlock{{Text: pragmaLoopSystemPrompt, Cacheable: false}}}
	engine.runPragmaLoopWithInitialPrompt(ctx, system, pragmaLoopInstancePrompt(userMessage, snap.CWD), nil, ch)
}

// PragmaLoopCompletionCheck can reject a submitted bash turn and keep the same
// loop running with a corrective user observation.
type PragmaLoopCompletionCheck func() (bool, string, error)

// RunPragmaLoopWithSystemCompletionCheck runs the shell-action loop with an
// optional completion check after a command emits the completion sentinel.
func (engine *Engine) RunPragmaLoopWithSystemCompletionCheck(ctx context.Context, system model.SystemPrompt, userMessage string, completionCheck PragmaLoopCompletionCheck) <-chan LoopEvent {
	observe.TraceCtx(ctx, "query", "Engine.RunPragmaLoopWithSystemCompletionCheck", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.RunPragmaLoopWithSystemCompletionCheck", "exit")
	ch := make(chan LoopEvent, 16)
	go func() {
		defer close(ch)
		defer func() {
			if r := recover(); r != nil {
				observe.TraceCtx(ctx, "query", "Engine.RunPragmaLoopWithSystemCompletionCheck", "if: r != nil")
				ch <- ErrorEvent{Err: fmt.Errorf("query loop panic: %v", r)}
			}
		}()
		startIndex := len(engine.store.Snapshot().Conversation.Messages)
		engine.runPragmaLoopWithInitialPrompt(ctx, system, userMessage, completionCheck, ch, startIndex)
	}()
	observe.TraceCtx(ctx, "query", "Engine.RunPragmaLoopWithSystemCompletionCheck", "return: ch")
	return ch
}

func (engine *Engine) runPragmaLoopWithInitialPrompt(ctx context.Context, system model.SystemPrompt, userMessage string, completionCheck PragmaLoopCompletionCheck, ch chan<- LoopEvent, messageStartIndexes ...int) {
	observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "exit")
	defer func() {
		engine.runStopHook(ch)
	}()

	run := engine.newPragmaLoopRunConfig(system, userMessage, completionCheck, messageStartIndexes...)
	engine.setConversationSystemPrompt(run.System)

	if err := engine.appendPragmaLoopInitialUserMessage(run); err != nil {
		observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: err != nil")
		ch <- ErrorEvent{Err: err}
		return
	}

	for turn := 0; turn < run.MaxTurns; turn++ {
		observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "for: turn < maxTurns")
		if err := ctx.Err(); err != nil {
			observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: err != nil")
			ch <- ErrorEvent{Err: fmt.Errorf("context cancelled: %w", model.ErrContextCancelled)}
			return
		}

		request, workDir, err := engine.buildPragmaLoopTurnRequest(run)
		if err != nil {
			observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: err != nil")
			ch <- ErrorEvent{Err: err}
			return
		}

		assistantTurn, err := engine.completePragmaLoopAssistantTurn(ctx, request, ch)
		if err != nil {
			observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: err != nil")
			ch <- ErrorEvent{Err: err}
			return
		}
		emitPragmaLoopResponseText(assistantTurn.Response, ch)
		ch <- ModelResponseEvent{Model: request.Model, StopReason: assistantTurn.Response.StopReason}

		if err := engine.appendPragmaLoopAssistantTurn(assistantTurn); err != nil {
			observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: err != nil")
			ch <- ErrorEvent{Err: err}
			return
		}

		if engine.autoTracker != nil {
			observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: engine.autoTracker != nil")
			engine.autoTracker.IncrementTurn()
		}

		if assistantTurn.Action.Kind == pragmaLoopActionFinal {
			observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: actionCount == 0")
			ch <- TurnCompleteEvent{Response: assistantTurn.Response, StopReason: model.StopEndTurn}
			return
		}
		if assistantTurn.Action.Kind == pragmaLoopActionInvalid {
			observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: actionCount > 1")
			if err := engine.appendPragmaLoopUserMessage(fmt.Sprintf(pragmaLoopFormatErrorTemplate, assistantTurn.Action.Count)); err != nil {
				observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: err != nil")
				ch <- ErrorEvent{Err: err}
				return
			}
			continue
		}

		commandResult := executePragmaLoopCommand(ctx, workDir, assistantTurn.Action.Bash)
		obs, err := commandResult.Observation(run.CompletionCheck)
		if err != nil {
			observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: submitted")
			ch <- ErrorEvent{Err: err}
			return
		}
		if obs.Complete {
			observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: obs.Complete")
			ch <- TurnCompleteEvent{Response: assistantTurn.Response, StopReason: model.StopEndTurn}
			return
		}
		if err := engine.appendPragmaLoopUserMessage(obs.Text); err != nil {
			observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: err != nil")
			ch <- ErrorEvent{Err: err}
			return
		}
	}

	ch <- ErrorEvent{Err: fmt.Errorf("agentic loop exceeded maximum of %d turns", run.MaxTurns)}
}

func (engine *Engine) newPragmaLoopRunConfig(system model.SystemPrompt, userMessage string, completionCheck PragmaLoopCompletionCheck, messageStartIndexes ...int) pragmaLoopRunConfig {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	maxTurns := engine.config.MaxTurns
	if maxTurns <= 0 {
		observe.GlobalTrace("if: maxTurns <= 0")
		maxTurns = 300
	}
	indexes := append([]int(nil), messageStartIndexes...)
	observe.GlobalTrace("return: pragmaLoopRunConfig")
	observe.GlobalTrace("return: pragmaLoopRunConfig{\n\tSystem:\t\t\tsystem,\n\tUserMessage:\t\tuserMessage,\n\tCompleti...")
	return pragmaLoopRunConfig{
		System:              system,
		UserMessage:         userMessage,
		CompletionCheck:     completionCheck,
		MessageStartIndexes: indexes,
		MaxTurns:            maxTurns,
	}
}

func (engine *Engine) appendPragmaLoopInitialUserMessage(run pragmaLoopRunConfig) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	userMsg := model.Message{
		ID:        model.NewUUID(),
		Role:      model.RoleUser,
		Content:   []model.ContentPart{model.TextPart{Text: run.UserMessage}},
		Timestamp: time.Now(),
	}
	observe.GlobalTrace("return: engine.appendConversationMessage(userMsg)")
	return engine.appendConversationMessage(userMsg)
}

func (engine *Engine) buildPragmaLoopTurnRequest(run pragmaLoopRunConfig) (pragmaLoopTurnRequest, string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	snap := engine.store.Snapshot()
	resolvedModel := engine.config.Model
	if snap.Model != "" {
		observe.GlobalTrace("if: snap.Model != \"\"")
		resolvedModel = snap.Model
	}

	messagesForQuery, err := engine.messagesForRequestChecked(snap.Conversation, run.MessageStartIndexes...)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: pragmaLoopTurnRequest{}, \"\", err")
		return pragmaLoopTurnRequest{}, "", err
	}
	params := provider.RequestParams{
		Model:          resolvedModel,
		MaxTokens:      engine.config.MaxTokens,
		Messages:       messagesForQuery,
		System:         run.System,
		Temperature:    engine.config.Temperature,
		Thinking:       engine.config.Thinking,
		ResponseSchema: engine.config.ResponseSchema,
	}
	observe.GlobalTrace("return: pragmaLoopTurnRequest, snap.CWD, nil")
	observe.GlobalTrace("return: pragmaLoopTurnRequest{\n\tModel:\tresolvedModel,\n\tParams:\tparams,\n}, snap.CWD, nil")
	return pragmaLoopTurnRequest{
		Model:  resolvedModel,
		Params: params,
	}, snap.CWD, nil
}

func (engine *Engine) completePragmaLoopAssistantTurn(ctx context.Context, request pragmaLoopTurnRequest, ch chan<- LoopEvent) (pragmaLoopAssistantTurn, error) {
	observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopAssistantTurn", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopAssistantTurn", "exit")
	for noActionRetries := 0; ; noActionRetries++ {
		observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopAssistantTurn", "for: true")
		ch <- ModelRequestEvent{Model: request.Model, Attempt: noActionRetries + 1}
		response, err := engine.completePragmaLoopResponse(ctx, request.Params, ch)
		if err != nil {
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopAssistantTurn", "if: err != nil")
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopAssistantTurn", "return: pragmaLoopAssistantTurn{}, err")
			return pragmaLoopAssistantTurn{}, err
		}

		turn := classifyPragmaLoopAssistantTurn(response)
		switch turn.Action.Kind {
		case pragmaLoopActionNativeToolCall:
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopAssistantTurn", "case: pragmaLoopActionNativeToolCall")
			if noActionRetries >= pragmaLoopMaxNoActionRetries {
				observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopAssistantTurn", "return: pragmaLoopAssistantTurn{}, fmt.Errorf(\"model returned native tool call in Pra...")
				return pragmaLoopAssistantTurn{}, fmt.Errorf("model returned native tool call in Pragma bash loop after %d retries", pragmaLoopMaxNoActionRetries)
			}
		case pragmaLoopActionFinal:
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopAssistantTurn", "case: pragmaLoopActionFinal")
			if noActionRetries >= pragmaLoopMaxNoActionRetries {
				observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopAssistantTurn", "return: pragmaLoopAssistantTurn{}, fmt.Errorf(\"model returned no bash action after %d...")
				return pragmaLoopAssistantTurn{}, fmt.Errorf("model returned no bash action after %d retries", pragmaLoopMaxNoActionRetries)
			}
		default:
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopAssistantTurn", "default")
			return turn, nil
		}
	}
}

func classifyPragmaLoopAssistantTurn(response model.Response) pragmaLoopAssistantTurn {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	text := responseText(response)
	action := classifyPragmaLoopAction(response.Content, text)
	observe.GlobalTrace("return: pragmaLoopAssistantTurn")
	observe.GlobalTrace("return: pragmaLoopAssistantTurn{\n\tResponse:\tresponse,\n\tText:\t\ttext,\n\tReplayContent:\tp...")
	return pragmaLoopAssistantTurn{
		Response:      response,
		Text:          text,
		ReplayContent: pragmaLoopReplayContent(response.Content),
		Action:        action,
	}
}

func classifyPragmaLoopAction(content []model.ContentPart, text string) pragmaLoopAction {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if pragmaLoopHasToolCall(content) {
		observe.GlobalTrace("if: pragmaLoopHasToolCall(content)")
		observe.GlobalTrace("return: pragmaLoopAction{Kind: pragmaLoopActionNativeToolCall}")
		return pragmaLoopAction{Kind: pragmaLoopActionNativeToolCall}
	}
	command, actionCount := extractPragmaLoopCommand(text)
	if actionCount == 0 {
		observe.GlobalTrace("if: actionCount == 0")
		observe.GlobalTrace("return: pragmaLoopAction{Kind: pragmaLoopActionFinal}")
		return pragmaLoopAction{Kind: pragmaLoopActionFinal}
	}
	if actionCount > 1 {
		observe.GlobalTrace("if: actionCount > 1")
		observe.GlobalTrace("return: pragmaLoopAction{Kind: pragmaLoopActionInvalid, Count: actionCount}")
		return pragmaLoopAction{Kind: pragmaLoopActionInvalid, Count: actionCount}
	}
	observe.GlobalTrace("return: pragmaLoopAction{Kind: pragmaLoopActionBash}")
	observe.GlobalTrace("return: pragmaLoopAction{Kind: pragmaLoopActionBash, Count: actionCount, Bash: command}")
	return pragmaLoopAction{Kind: pragmaLoopActionBash, Count: actionCount, Bash: command}
}

func (engine *Engine) appendPragmaLoopAssistantTurn(turn pragmaLoopAssistantTurn) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	assistantMsg := model.Message{
		ID:        model.NewUUID(),
		Role:      model.RoleAssistant,
		Content:   turn.ReplayContent,
		Timestamp: time.Now(),
	}
	observe.GlobalTrace("return: engine.appendConversationMessage(assistantMsg)")
	return engine.appendConversationMessage(assistantMsg)
}

func executePragmaLoopCommand(ctx context.Context, workDir, command string) pragmaLoopCommandResult {
	observe.TraceCtx(ctx, "query", "executePragmaLoopCommand", "enter")
	defer observe.TraceCtx(ctx, "query", "executePragmaLoopCommand", "exit")
	result, timedOut := runPragmaLoopBash(ctx, workDir, command)
	submitted := false
	if !timedOut {
		observe.TraceCtx(ctx, "query", "executePragmaLoopCommand", "if: !timedOut")
		submitted, _ = pragmaLoopSubmitted(result)
	}
	observe.TraceCtx(ctx, "query", "executePragmaLoopCommand", "return: pragmaLoopCommandResult")
	observe.TraceCtx(ctx, "query", "executePragmaLoopCommand", "return: pragmaLoopCommandResult{\n\tCommand:\tcommand,\n\tResult:\t\tresult,\n\tTimedOut:\ttime...")
	return pragmaLoopCommandResult{
		Command:   command,
		Result:    result,
		TimedOut:  timedOut,
		Submitted: submitted,
	}
}

func (result pragmaLoopCommandResult) Observation(completionCheck PragmaLoopCompletionCheck) (pragmaLoopObservation, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if result.TimedOut {
		observe.GlobalTrace("if: result.TimedOut")
		observe.GlobalTrace("return: pragmaLoopObservation{Text: formatPragmaLoopTimeout(result.Command, result.Re...")
		return pragmaLoopObservation{Text: formatPragmaLoopTimeout(result.Command, result.Result.Output)}, nil
	}
	if !result.Submitted {
		observe.GlobalTrace("if: !result.Submitted")
		observe.GlobalTrace("return: pragmaLoopObservation{Text: formatPragmaLoopObservation(result.Result)}, nil")
		return pragmaLoopObservation{Text: formatPragmaLoopObservation(result.Result)}, nil
	}
	if completionCheck == nil {
		observe.GlobalTrace("if: completionCheck == nil")
		observe.GlobalTrace("return: pragmaLoopObservation{Complete: true}, nil")
		return pragmaLoopObservation{Complete: true}, nil
	}
	ok, message, err := completionCheck()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: pragmaLoopObservation{}, err")
		return pragmaLoopObservation{}, err
	}
	if ok {
		observe.GlobalTrace("if: ok")
		observe.GlobalTrace("return: pragmaLoopObservation{Complete: true}, nil")
		return pragmaLoopObservation{Complete: true}, nil
	}
	if strings.TrimSpace(message) == "" {
		observe.GlobalTrace("if: strings.TrimSpace(message) == \"\"")
		message = "Completion was rejected because required output artifacts are missing. Create the missing artifacts and echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT again."
	}
	observe.GlobalTrace("return: pragmaLoopObservation{Text: message}, nil")
	return pragmaLoopObservation{Text: message}, nil
}

func (engine *Engine) completePragmaLoopResponse(ctx context.Context, params provider.RequestParams, ch chan<- LoopEvent) (model.Response, error) {
	observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "exit")
	const maxStreamRetries = 10
	const maxConsecutiveOverloaded = 3
	var consecutiveOverloaded int

	for attempt := range maxStreamRetries + 1 {
		observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "range maxStreamRetries + 1")
		response, err := engine.provider.Complete(ctx, params)
		if err == nil {
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "if: err == nil")
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "return: response, nil")
			return response, nil
		}

		classified := ClassifyStreamError(err)
		if classified.Kind == ErrorKindOverloaded {
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "if: classified.Kind == ErrorKindOverloaded")
			consecutiveOverloaded++
			if consecutiveOverloaded >= maxConsecutiveOverloaded {
				observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "if: consecutiveOverloaded >= maxConsecutiveOverloaded")
				observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "return: model.Response{}, fmt.Errorf(\"repeated overloaded errors\")")
				return model.Response{}, fmt.Errorf("repeated overloaded errors")
			}
		} else {
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "else: classified.Kind == ErrorKindOverloaded")
			consecutiveOverloaded = 0
		}
		if !classified.Retryable || attempt >= maxStreamRetries {
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "if: !classified.Retryable || attempt >= maxStreamRetries")
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "return: model.Response{}, classified.Err")
			return model.Response{}, classified.Err
		}

		delay := classified.RetryAfter
		if delay == 0 {
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "if: delay == 0")
			delay = retryDelay(attempt)
		}
		ch <- RetryEvent{
			Attempt:     attempt + 1,
			MaxAttempts: maxStreamRetries + 1,
			Delay:       delay,
			Kind:        classified.Kind,
			ErrorMsg:    err.Error(),
		}
		select {
		case <-time.After(delay):
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "select: <-time.After(delay)")
		case <-ctx.Done():
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "select: <-ctx.Done()")
			return model.Response{}, fmt.Errorf("context cancelled during retry: %w", model.ErrContextCancelled)
		}
	}
	observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "return: model.Response{}, errors.New(\"model request failed\")")

	return model.Response{}, errors.New("model request failed")
}

func emitPragmaLoopResponseText(response model.Response, ch chan<- LoopEvent) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, part := range response.Content {
		observe.GlobalTrace("range response.Content")
		if text, ok := part.(model.TextPart); ok && text.Text != "" {
			observe.GlobalTrace("if: ok && text.Text != \"\"")
			ch <- TextEvent{Text: text.Text}
		}
	}
}

func (engine *Engine) appendPragmaLoopUserMessage(text string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	msg := model.Message{
		ID:        model.NewUUID(),
		Role:      model.RoleUser,
		Content:   []model.ContentPart{model.TextPart{Text: text}},
		Timestamp: time.Now(),
	}
	observe.GlobalTrace("return: engine.appendConversationMessage(msg)")
	return engine.appendConversationMessage(msg)
}

func pragmaLoopInstancePrompt(task, cwd string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Please solve this task: \" + task + fmt.Sprintf(pragmaLoopInstanceSuffix, cwd...")
	return "Please solve this task: " + task + fmt.Sprintf(pragmaLoopInstanceSuffix, cwd, pragmaLoopSystemInformation(cwd))
}

func pragmaLoopSystemInformation(cwd string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	out, err := exec.Command("uname", "-srmv").Output()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: cwd")
		return cwd
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) < 4 {
		observe.GlobalTrace("if: len(fields) < 4")
		observe.GlobalTrace("return: strings.TrimSpace(string(out))")
		return strings.TrimSpace(string(out))
	}
	system := fields[0]
	release := fields[1]
	machine := fields[len(fields)-1]
	version := strings.Join(fields[2:len(fields)-1], " ")
	observe.GlobalTrace("return: fmt.Sprintf(\"%s %s %s %s\", system, release, version, machine)")
	return fmt.Sprintf("%s %s %s %s", system, release, version, machine)
}

func responseText(response model.Response) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var b strings.Builder
	for _, part := range response.Content {
		observe.GlobalTrace("range response.Content")
		if text, ok := part.(model.TextPart); ok {
			observe.GlobalTrace("if: ok")
			b.WriteString(text.Text)
		}
	}
	observe.GlobalTrace("return: b.String()")
	return b.String()
}

func pragmaLoopReplayContent(content []model.ContentPart) []model.ContentPart {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	out := make([]model.ContentPart, 0, len(content))
	for _, part := range content {
		observe.GlobalTrace("range content")
		if _, ok := part.(model.ThinkingPart); ok {
			observe.GlobalTrace("if: ok")
			continue
		}
		if _, ok := part.(model.ToolCallPart); ok {
			observe.GlobalTrace("if: ok")
			continue
		}
		out = append(out, part)
	}
	observe.GlobalTrace("return: out")
	return out
}

func pragmaLoopHasToolCall(content []model.ContentPart) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, part := range content {
		observe.GlobalTrace("range content")
		if _, ok := part.(model.ToolCallPart); ok {
			observe.GlobalTrace("if: ok")
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func extractPragmaLoopCommand(text string) (string, int) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	matches := extractPragmaLoopBashBlocks(text)
	if len(matches) != 1 {
		observe.GlobalTrace("if: len(matches) != 1")
		observe.GlobalTrace("return: \"\", len(matches)")
		return "", len(matches)
	}
	observe.GlobalTrace("return: strings.TrimSpace(matches[0]), 1")
	return strings.TrimSpace(matches[0]), 1
}

type pragmaLoopHeredoc struct {
	delimiter string
	stripTabs bool
}

func extractPragmaLoopBashBlocks(text string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	lines := strings.Split(text, "\n")
	var blocks []string
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "```bash" {
			continue
		}

		start := i + 1
		var heredocs []pragmaLoopHeredoc
		for j := start; j < len(lines); j++ {
			line := lines[j]
			if len(heredocs) > 0 {
				if pragmaLoopHeredocEnds(line, heredocs[0]) {
					heredocs = heredocs[1:]
				}
				continue
			}

			if strings.TrimSpace(line) == "```" {
				blocks = append(blocks, strings.Join(lines[start:j], "\n"))
				i = j
				break
			}

			heredocs = append(heredocs, extractPragmaLoopHeredocs(line)...)
		}
	}
	observe.GlobalTrace("return: blocks")
	return blocks
}

func pragmaLoopHeredocEnds(line string, heredoc pragmaLoopHeredoc) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if heredoc.stripTabs {
		line = strings.TrimLeft(line, "\t")
	}
	observe.GlobalTrace("return: line == heredoc.delimiter")
	return line == heredoc.delimiter
}

func extractPragmaLoopHeredocs(line string) []pragmaLoopHeredoc {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var heredocs []pragmaLoopHeredoc
	var inSingle, inDouble, escaped bool
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && !inSingle {
			escaped = true
			continue
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if inSingle || inDouble {
			continue
		}
		if ch == '#' {
			break
		}
		if ch != '<' || i+1 >= len(line) || line[i+1] != '<' {
			continue
		}

		pos := i + 2
		stripTabs := false
		if pos < len(line) && line[pos] == '-' {
			stripTabs = true
			pos++
		}
		delimiter, next, ok := readPragmaLoopShellWord(line, pos)
		if ok {
			heredocs = append(heredocs, pragmaLoopHeredoc{
				delimiter: delimiter,
				stripTabs: stripTabs,
			})
			i = next - 1
		}
	}
	observe.GlobalTrace("return: heredocs")
	return heredocs
}

func readPragmaLoopShellWord(line string, pos int) (string, int, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for pos < len(line) && (line[pos] == ' ' || line[pos] == '\t') {
		pos++
	}

	var b strings.Builder
	var inSingle, inDouble, escaped bool
	for pos < len(line) {
		ch := line[pos]
		if escaped {
			b.WriteByte(ch)
			escaped = false
			pos++
			continue
		}
		if ch == '\\' && !inSingle {
			escaped = true
			pos++
			continue
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			pos++
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			pos++
			continue
		}
		if !inSingle && !inDouble && isPragmaLoopShellWordTerminator(ch) {
			break
		}
		b.WriteByte(ch)
		pos++
	}

	if b.Len() == 0 || inSingle || inDouble || escaped {
		observe.GlobalTrace("return: \"\", pos, false")
		return "", pos, false
	}
	observe.GlobalTrace("return: b.String(), pos, true")
	return b.String(), pos, true
}

func isPragmaLoopShellWordTerminator(ch byte) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: ch == ' ' || ch == '\\t' || strings.ContainsRune(\";|&<>()\", rune(ch))")
	return ch == ' ' || ch == '\t' || strings.ContainsRune(";|&<>()", rune(ch))
}

type pragmaLoopBashResult struct {
	ReturnCode int
	Output     string
}

func runPragmaLoopBash(ctx context.Context, workDir, command string) (pragmaLoopBashResult, bool) {
	observe.TraceCtx(ctx, "query", "runPragmaLoopBash", "enter")
	defer observe.TraceCtx(ctx, "query", "runPragmaLoopBash", "exit")
	if patch, patchWorkDir, ok, err := applypatch.ExtractShellApplyPatch(command); err != nil {
		observe.TraceCtx(ctx, "query", "runPragmaLoopBash", "if: err != nil")
		observe.TraceCtx(ctx, "query", "runPragmaLoopBash", "return: pragmaLoopBashResult{ReturnCode: 1, Output: err.Error()}, false")
		return pragmaLoopBashResult{ReturnCode: 1, Output: err.Error()}, false
	} else if ok {
		observe.TraceCtx(ctx, "query", "runPragmaLoopBash", "else-if: ok")
		applyWorkDir := workDir
		if patchWorkDir != "" {
			if filepath.IsAbs(patchWorkDir) {
				applyWorkDir = patchWorkDir
			} else {
				applyWorkDir = filepath.Join(workDir, patchWorkDir)
			}
		}
		result, err := applypatch.ApplyPatchText(ctx, patch, applyWorkDir)
		if err != nil {
			return pragmaLoopBashResult{ReturnCode: 1, Output: err.Error()}, false
		}
		return pragmaLoopBashResult{ReturnCode: 0, Output: result.Content}, false
	}
	if rejection := (pragmaLoopCommandPolicy{}).Evaluate(workDir, command); rejection.Reason != "" {
		observe.TraceCtx(ctx, "query", "runPragmaLoopBash", "if: reason != \"\"")
		observe.TraceCtx(ctx, "query", "runPragmaLoopBash", "return: pragmaLoopBashResult{\n\tReturnCode:\t1,\n\tOutput:\t\tfmt.Sprintf(\"Bash rejected: %...")
		return pragmaLoopBashResult{
			ReturnCode: 1,
			Output:     fmt.Sprintf("Bash rejected: %s. Use apply_patch <<'PATCH' for repository source edits; Bash remains available for read-only inspection, validation, and runtime artifact writes.", rejection.Reason),
		}, false
	}
	result, err := shellrun.Execute(ctx, shellrun.Options{
		Command:            command,
		WorkDir:            workDir,
		Timeout:            pragmaLoopCommandTimeout,
		ForegroundWait:     pragmaLoopForegroundWait,
		BaseDirName:        "pragma-loop-bash",
		UsePipefail:        true,
		UseErrexit:         true,
		RunningOutputLines: pragmaLoopRunningOutputLines,
	})
	if err != nil {
		observe.TraceCtx(ctx, "query", "runPragmaLoopBash", "if: err != nil")
		observe.TraceCtx(ctx, "query", "runPragmaLoopBash", "return: pragmaLoopBashResult{ReturnCode: -1, Output: err.Error()}, false")
		return pragmaLoopBashResult{ReturnCode: -1, Output: err.Error()}, false
	}
	out := pragmaLoopBashResult{
		ReturnCode: result.ExitCode,
		Output:     result.Output,
	}
	if result.Err != nil && result.ExitCode == -1 && !result.TimedOut && !result.Running {
		observe.TraceCtx(ctx, "query", "runPragmaLoopBash", "if: result.Err != nil && result.ExitCode == -1 && !result.TimedOut && !result.Run...")
		if out.Output != "" {
			observe.TraceCtx(ctx, "query", "runPragmaLoopBash", "if: out.Output != \"\"")
			out.Output += "\n"
		}
		out.Output += result.Err.Error()
	}
	observe.TraceCtx(ctx, "query", "runPragmaLoopBash", "return: out, result.TimedOut")
	return out, result.TimedOut
}

func (pragmaLoopCommandPolicy) Evaluate(workDir, command string) pragmaLoopCommandRejection {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	reason := pragmaLoopSourceMutationReason(workDir, command)
	if reason == "" {
		observe.GlobalTrace("if: reason == \"\"")
		observe.GlobalTrace("return: pragmaLoopCommandRejection{}")
		return pragmaLoopCommandRejection{}
	}
	observe.GlobalTrace("return: pragmaLoopCommandRejection{Reason: reason}")
	return pragmaLoopCommandRejection{Reason: reason}
}

func pragmaLoopSourceMutationReason(workDir, command string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	fields := strings.Fields(command)
	for i, field := range fields {
		observe.GlobalTrace("range fields")
		base := filepath.Base(strings.Trim(field, `"'`))
		switch base {
		case "sed", "gsed", "perl":
			observe.GlobalTrace("case: \"sed\", \"gsed\", \"perl\"")
			if i+1 < len(fields) && strings.HasPrefix(strings.Trim(fields[i+1], `"'`), "-i") && commandMentionsRepoSourcePath(workDir, strings.Join(fields[i+2:], " ")) {
				observe.GlobalTrace("return: base + \" in-place edits to repository source are blocked\"")
				return base + " in-place edits to repository source are blocked"
			}
		case "tee":
			observe.GlobalTrace("case: \"tee\"")
			if firstRepoSourcePath(workDir, fields[i+1:]) != "" {
				observe.GlobalTrace("return: \"tee writes to repository source are blocked\"")
				return "tee writes to repository source are blocked"
			}
		case "python", "python3", "perl5":
			observe.GlobalTrace("case: \"python\", \"python3\", \"perl5\"")
			if commandContainsWriteIntent(command) && commandMentionsRepoSourcePath(workDir, command) {
				observe.GlobalTrace("return: base + \" file-write snippets to repository source are blocked\"")
				return base + " file-write snippets to repository source are blocked"
			}
		}
	}

	for i, field := range fields {
		observe.GlobalTrace("range fields")
		switch field {
		case ">", ">>":
			observe.GlobalTrace("case: \">\", \">>\"")
			if i+1 < len(fields) && isRepoSourcePath(workDir, cleanShellPathToken(fields[i+1])) {
				observe.GlobalTrace("return: \"shell redirection writes to repository source are blocked\"")
				return "shell redirection writes to repository source are blocked"
			}
		default:
			observe.GlobalTrace("default")
			if strings.HasPrefix(field, ">") {
				path := strings.TrimPrefix(strings.TrimPrefix(field, ">>"), ">")
				if isRepoSourcePath(workDir, cleanShellPathToken(path)) {
					observe.GlobalTrace("if: isRepoSourcePath(workDir, cleanShellPathToken(path))")
					observe.GlobalTrace("return: \"shell redirection writes to repository source are blocked\"")
					return "shell redirection writes to repository source are blocked"
				}
			}
		}
	}
	observe.GlobalTrace("return: \"\"")
	return ""
}

func commandContainsWriteIntent(command string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	lower := strings.ToLower(command)
	for _, needle := range []string{
		"write_text(",
		"write_bytes(",
		"os.writefile(",
		"os.remove(",
		"os.rename(",
		".write(",
	} {
		observe.GlobalTrace("range []string{\n\t\"write_text(\",\n\t\"write_bytes(\",\n\t\"os.writefile(\",\n\t\"os.remove(\",\n\t...")
		if strings.Contains(lower, needle) {
			observe.GlobalTrace("if: strings.Contains(lower, needle)")
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func commandMentionsRepoSourcePath(workDir, command string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, field := range strings.Fields(command) {
		observe.GlobalTrace("range strings.Fields(command)")
		if isRepoSourcePath(workDir, cleanShellPathToken(field)) {
			observe.GlobalTrace("if: isRepoSourcePath(workDir, cleanShellPathToken(field))")
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func firstRepoSourcePath(workDir string, fields []string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, field := range fields {
		observe.GlobalTrace("range fields")
		path := cleanShellPathToken(field)
		if strings.HasPrefix(path, "-") {
			observe.GlobalTrace("if: strings.HasPrefix(path, \"-\")")
			continue
		}
		if isRepoSourcePath(workDir, path) {
			observe.GlobalTrace("if: isRepoSourcePath(workDir, path)")
			observe.GlobalTrace("return: path")
			return path
		}
	}
	observe.GlobalTrace("return: \"\"")
	return ""
}

func cleanShellPathToken(token string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	token = strings.TrimSpace(token)
	token = strings.Trim(token, `"'`)
	token = strings.TrimSuffix(token, ";")
	token = strings.TrimSuffix(token, `\`)
	observe.GlobalTrace("return: token")
	return token
}

func isRepoSourcePath(workDir, path string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if path == "" || strings.HasPrefix(path, "-") || strings.HasPrefix(path, "$") {
		observe.GlobalTrace("if: path == \"\" || strings.HasPrefix(path, \"-\") || strings.HasPrefix(path, \"$\")")
		observe.GlobalTrace("return: false")
		return false
	}
	if strings.HasPrefix(path, "/tmp/pragma/") || path == "/tmp/pragma" {
		observe.GlobalTrace("if: strings.HasPrefix(path, \"/tmp/pragma/\") || path == \"/tmp/pragma\"")
		observe.GlobalTrace("return: false")
		return false
	}
	clean := filepath.Clean(path)
	if filepath.IsAbs(clean) {
		observe.GlobalTrace("if: filepath.IsAbs(clean)")
		if workDir == "" {
			observe.GlobalTrace("if: workDir == \"\"")
			observe.GlobalTrace("return: false")
			return false
		}
		absWorkDir, err := filepath.Abs(workDir)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			absWorkDir = workDir
		}
		rel, err := filepath.Rel(absWorkDir, clean)
		if err != nil || strings.HasPrefix(rel, "..") || rel == "." {
			observe.GlobalTrace("if: err != nil || strings.HasPrefix(rel, \"..\") || rel == \".\"")
			observe.GlobalTrace("return: strings.HasPrefix(clean, \"/app/\") && looksLikeSourcePath(strings.TrimPrefix(c...")
			return strings.HasPrefix(clean, "/app/") && looksLikeSourcePath(strings.TrimPrefix(clean, "/app/"))
		}
		clean = rel
	}
	clean = strings.TrimPrefix(clean, "./")
	observe.GlobalTrace("return: looksLikeSourcePath(clean)")
	return looksLikeSourcePath(clean)
}

func looksLikeSourcePath(path string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if path == "" || strings.HasPrefix(path, "..") || strings.Contains(path, "*") {
		observe.GlobalTrace("if: path == \"\" || strings.HasPrefix(path, \"..\") || strings.Contains(path, \"*\")")
		observe.GlobalTrace("return: false")
		return false
	}
	switch filepath.Ext(path) {
	case ".go", ".py", ".js", ".jsx", ".ts", ".tsx", ".java", ".rs", ".c", ".cc", ".cpp", ".h", ".hpp", ".proto", ".yaml", ".yml", ".json", ".toml", ".md":
		observe.GlobalTrace("case: \".go\", \".py\", \".js\", \".jsx\", \".ts\", \".tsx\", \".java\", \".rs\", \".c\", \".cc\", \".cp...")
	default:
		observe.GlobalTrace("default")
		return false
	}
	first := path
	if idx := strings.IndexRune(path, filepath.Separator); idx >= 0 {
		observe.GlobalTrace("if: idx >= 0")
		first = path[:idx]
	}
	switch first {
	case "cmd", "internal", "pkg", "api", "src", "lib", "server", "client", "web", "config", "configs", "tools", "test", "tests":
		observe.GlobalTrace("case: \"cmd\", \"internal\", \"pkg\", \"api\", \"src\", \"lib\", \"server\", \"client\", \"web\", \"co...")
		return true
	default:
		observe.GlobalTrace("default")
		return !filepath.IsAbs(path) && !strings.HasPrefix(path, "/")
	}
}

func formatPragmaLoopObservation(result pragmaLoopBashResult) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(result.Output) < 10_000 {
		observe.GlobalTrace("if: len(result.Output) < 10_000")
		observe.GlobalTrace("return: fmt.Sprintf(\"<returncode>%d</returncode>\\n<output>\\n%s</output>\", result.Retu...")
		return fmt.Sprintf("<returncode>%d</returncode>\n<output>\n%s</output>", result.ReturnCode, result.Output)
	}
	elidedChars := len(result.Output) - 10_000
	observe.GlobalTrace("return: fmt.Sprintf(`<returncode>%d</returncode>\n<warning>\nThe output of your last co...")
	return fmt.Sprintf(`<returncode>%d</returncode>
<warning>
The output of your last command was too long.
Please try a different command that produces less output.
If you're inspecting a file for patch context, use unnumbered sed -n '<start>,<end>p' path/to/file.
Do not use nl -ba output as patch context because its first column is display-only line numbers.
If you're using grep or find and it produced too much output, use a more selective search pattern.
Do not pipe validation commands such as tests or builds to head or tail as proof of success.
For large validation output, redirect full output to a log, preserve rc=$?, print useful log lines, and exit with the original rc.
</warning>
<output_head>
%s
</output_head>
<elided_chars>
%d characters elided
</elided_chars>
<output_tail>
%s
</output_tail>`, result.ReturnCode, result.Output[:5000], elidedChars, result.Output[len(result.Output)-5000:])
}

func formatPragmaLoopTimeout(command, output string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var body string
	if len(output) < 10_000 {
		observe.GlobalTrace("if: len(output) < 10_000")
		body = fmt.Sprintf("<output>\n%s\n</output>", output)
	} else {
		observe.GlobalTrace("else: len(output) < 10_000")
		body = fmt.Sprintf(`<warning>Output was too long and has been truncated.</warning>
<output_head>
%s
</output_head>
<elided_chars>%d characters elided</elided_chars>
<output_tail>
%s
</output_tail>`, output[:5000], len(output)-10_000, output[len(output)-5000:])
	}
	observe.GlobalTrace("return: fmt.Sprintf(`The last command <command>%s</command> timed out and has been ki...")
	return fmt.Sprintf(`The last command <command>%s</command> timed out and has been killed.
The output of the command was:
%s
Please try another command and make sure to avoid those requiring interactive input.`, command, body)
}

func pragmaLoopSubmitted(result pragmaLoopBashResult) (bool, string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if result.ReturnCode != 0 {
		observe.GlobalTrace("if: result.ReturnCode != 0")
		observe.GlobalTrace("return: false, \"\"")
		return false, ""
	}
	observe.GlobalTrace("return: strings.Contains(result.Output, \"MINI_SWE_AGENT_FINAL_OUTPUT\") ||\n\tstrings.Co...")
	return strings.Contains(result.Output, "MINI_SWE_AGENT_FINAL_OUTPUT") ||
		strings.Contains(result.Output, "COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT"), ""
}
