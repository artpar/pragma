package query

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/shellrun"
)

const pragmaLoopSystemPrompt = `Pragma loop mode is a shell-action transport.

Follow the active task and persona instructions. This wrapper only defines how
to send shell actions.

When you need to inspect or change the system, respond with exactly one fenced
bash code block containing one command or one shell script, and no prose outside
the code block. A fenced bash block is a tool call and will be executed.

When no shell action is needed, or when you are ready to answer the user, respond
with the final user-facing answer and no fenced bash code block. A response with
no fenced bash block ends the turn.

Do not emit readiness echo commands. Do not use bash just to print the final
answer.

Use commands, command output, and required task artifacts for reasoning and
evidence.

Tool call format:

<format_example>
` + "```bash" + `
your_command_here
` + "```" + `
</format_example>

Failure to follow these rules will cause your response to be rejected.`

func PragmaLoopSystemPrompt() string {
	return pragmaLoopSystemPrompt
}

const pragmaLoopInstanceSuffix = `

<system_information>
Current working directory: %s
%s
</system_information>
`

const pragmaLoopFormatErrorTemplate = `Please provide at most one bash action in triple backticks. Found %d actions.
If you want to answer the user or end the task, write the final answer with no fenced bash block.
If you need a shell action, format your response exactly as follows and include no prose outside the code block:

<response_example>
` + "```bash" + `
<action>
` + "```" + `
</response_example>

Note: In rare cases, if you need to reference a similar format in your command, you might have
to proceed in two steps, first writing TRIPLEBACKTICKSBASH, then replacing them with ` + "```bash" + `.`

var pragmaLoopBashBlockRE = regexp.MustCompile("(?s)```bash\\s*\\n(.*?)\\n```")

var pragmaLoopCommandTimeout = 300 * time.Second
var pragmaLoopForegroundWait = 30 * time.Second

const pragmaLoopRunningOutputLines = 100

func (e *Engine) runPragmaLoop(ctx context.Context, userMessage string, ch chan<- LoopEvent) {
	snap := e.store.Snapshot()
	system := pragmaLoopSystemFromExisting(snap.Conversation.System)
	e.runPragmaLoopWithInitialPrompt(ctx, system, pragmaLoopInstancePrompt(userMessage, snap.CWD), ch)
}

// RunPragmaLoopWithSystem runs the shell-action loop with an explicit system
// prompt and first user message. It is used by orchestration states that need
// persona instructions to have system-message priority.
func (e *Engine) RunPragmaLoopWithSystem(ctx context.Context, system model.SystemPrompt, userMessage string) <-chan LoopEvent {
	ch := make(chan LoopEvent, 16)
	go func() {
		defer close(ch)
		defer func() {
			if r := recover(); r != nil {
				ch <- ErrorEvent{Err: fmt.Errorf("query loop panic: %v", r)}
			}
		}()
		e.runPragmaLoopWithInitialPrompt(ctx, system, userMessage, ch)
	}()
	return ch
}

func (e *Engine) runPragmaLoopWithInitialPrompt(ctx context.Context, system model.SystemPrompt, userMessage string, ch chan<- LoopEvent) {
	defer func() {
		e.runStopHook(ch)
	}()

	maxTurns := e.config.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 300
	}

	snap := e.store.Snapshot()
	userMsg := model.Message{
		ID:        model.NewUUID(),
		Role:      model.RoleUser,
		Content:   []model.ContentPart{model.TextPart{Text: userMessage}},
		Timestamp: time.Now(),
	}
	if err := e.appendConversationMessage(userMsg, func(s *app.AppState) {
		s.Conversation.System = system
	}); err != nil {
		ch <- ErrorEvent{Err: err}
		return
	}

	for turn := 0; turn < maxTurns; turn++ {
		if err := ctx.Err(); err != nil {
			ch <- ErrorEvent{Err: fmt.Errorf("context cancelled: %w", model.ErrContextCancelled)}
			return
		}

		snap = e.store.Snapshot()
		resolvedModel := e.config.Model
		if snap.Model != "" {
			resolvedModel = snap.Model
		}

		messagesForQuery, err := e.messagesForRequestChecked(snap.Conversation)
		if err != nil {
			ch <- ErrorEvent{Err: err}
			return
		}
		params := provider.RequestParams{
			Model:          resolvedModel,
			MaxTokens:      e.config.MaxTokens,
			Messages:       messagesForQuery,
			System:         system,
			Temperature:    e.config.Temperature,
			Thinking:       e.config.Thinking,
			ResponseSchema: e.config.ResponseSchema,
		}

		ch <- ModelRequestEvent{Model: resolvedModel, Attempt: 1}
		response, err := e.completePragmaLoopResponse(ctx, params, ch)
		if err != nil {
			ch <- ErrorEvent{Err: err}
			return
		}
		ch <- ModelResponseEvent{Model: resolvedModel, StopReason: response.StopReason}

		assistantMsg := model.Message{
			ID:        model.NewUUID(),
			Role:      model.RoleAssistant,
			Content:   pragmaLoopReplayContent(response.Content),
			Timestamp: time.Now(),
		}
		if err := e.appendConversationMessage(assistantMsg, nil); err != nil {
			ch <- ErrorEvent{Err: err}
			return
		}

		if e.autoTracker != nil {
			e.autoTracker.IncrementTurn()
		}

		assistantText := responseText(response)
		command, actionCount := extractPragmaLoopCommand(assistantText)
		if actionCount == 0 {
			if e.config.RequireStructuredOutput {
				ch <- ErrorEvent{Err: fmt.Errorf("structured output was not produced")}
				return
			}
			ch <- TurnCompleteEvent{Response: response, StopReason: model.StopEndTurn}
			return
		}
		if actionCount > 1 {
			if err := e.appendPragmaLoopUserMessage(fmt.Sprintf(pragmaLoopFormatErrorTemplate, actionCount)); err != nil {
				ch <- ErrorEvent{Err: err}
				return
			}
			continue
		}

		result, timedOut := runPragmaLoopBash(ctx, snap.CWD, command)
		if timedOut {
			if err := e.appendPragmaLoopUserMessage(formatPragmaLoopTimeout(command, result.Output)); err != nil {
				ch <- ErrorEvent{Err: err}
				return
			}
			continue
		}
		if err := e.appendPragmaLoopUserMessage(formatPragmaLoopObservation(result)); err != nil {
			ch <- ErrorEvent{Err: err}
			return
		}
	}

	ch <- ErrorEvent{Err: fmt.Errorf("agentic loop exceeded maximum of %d turns", maxTurns)}
}

func (e *Engine) completePragmaLoopResponse(ctx context.Context, params provider.RequestParams, ch chan<- LoopEvent) (model.Response, error) {
	const maxStreamRetries = 10
	const maxConsecutiveOverloaded = 3
	var consecutiveOverloaded int

	for attempt := range maxStreamRetries + 1 {
		response, err := e.provider.Complete(ctx, params)
		if err == nil {
			for _, part := range response.Content {
				if text, ok := part.(model.TextPart); ok && text.Text != "" {
					ch <- TextEvent{Text: text.Text}
				}
			}
			return response, nil
		}

		classified := ClassifyStreamError(err)
		if classified.Kind == ErrorKindOverloaded {
			consecutiveOverloaded++
			if consecutiveOverloaded >= maxConsecutiveOverloaded {
				return model.Response{}, fmt.Errorf("repeated overloaded errors")
			}
		} else {
			consecutiveOverloaded = 0
		}
		if !classified.Retryable || attempt >= maxStreamRetries {
			return model.Response{}, classified.Err
		}

		delay := classified.RetryAfter
		if delay == 0 {
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
		case <-ctx.Done():
			return model.Response{}, fmt.Errorf("context cancelled during retry: %w", model.ErrContextCancelled)
		}
	}

	return model.Response{}, errors.New("model request failed")
}

func (e *Engine) appendPragmaLoopUserMessage(text string) error {
	msg := model.Message{
		ID:        model.NewUUID(),
		Role:      model.RoleUser,
		Content:   []model.ContentPart{model.TextPart{Text: text}},
		Timestamp: time.Now(),
	}
	return e.appendConversationMessage(msg, nil)
}

func pragmaLoopInstancePrompt(task, cwd string) string {
	return "Please solve this task: " + task + fmt.Sprintf(pragmaLoopInstanceSuffix, cwd, pragmaLoopSystemInformation(cwd))
}

func pragmaLoopSystemFromExisting(existing model.SystemPrompt) model.SystemPrompt {
	text := pragmaLoopSystemPrompt
	for _, block := range existing.Blocks {
		if strings.Contains(block.Text, "## Server Commands") {
			text += "\n\n\n" + block.Text
			break
		}
	}
	return model.SystemPrompt{Blocks: []model.SystemBlock{{Text: text, Cacheable: false}}}
}

func pragmaLoopSystemInformation(cwd string) string {
	out, err := exec.Command("uname", "-srmv").Output()
	if err != nil {
		return cwd
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) < 4 {
		return strings.TrimSpace(string(out))
	}
	system := fields[0]
	release := fields[1]
	machine := fields[len(fields)-1]
	version := strings.Join(fields[2:len(fields)-1], " ")
	return fmt.Sprintf("%s %s %s %s", system, release, version, machine)
}

func responseText(response model.Response) string {
	var b strings.Builder
	for _, part := range response.Content {
		if text, ok := part.(model.TextPart); ok {
			b.WriteString(text.Text)
		}
	}
	return b.String()
}

func pragmaLoopReplayContent(content []model.ContentPart) []model.ContentPart {
	out := make([]model.ContentPart, 0, len(content))
	for _, part := range content {
		if _, ok := part.(model.ThinkingPart); ok {
			continue
		}
		out = append(out, part)
	}
	return out
}

func extractPragmaLoopCommand(text string) (string, int) {
	matches := pragmaLoopBashBlockRE.FindAllStringSubmatch(text, -1)
	if len(matches) != 1 {
		return "", len(matches)
	}
	return strings.TrimSpace(matches[0][1]), 1
}

type pragmaLoopBashResult struct {
	ReturnCode int
	Output     string
}

func runPragmaLoopBash(ctx context.Context, workDir, command string) (pragmaLoopBashResult, bool) {
	result, err := shellrun.Execute(ctx, shellrun.Options{
		Command:            command,
		WorkDir:            workDir,
		Timeout:            pragmaLoopCommandTimeout,
		ForegroundWait:     pragmaLoopForegroundWait,
		BaseDirName:        "pragma-loop-bash",
		UsePipefail:        true,
		RunningOutputLines: pragmaLoopRunningOutputLines,
	})
	if err != nil {
		return pragmaLoopBashResult{ReturnCode: -1, Output: err.Error()}, false
	}
	out := pragmaLoopBashResult{
		ReturnCode: result.ExitCode,
		Output:     result.Output,
	}
	if result.Err != nil && result.ExitCode == -1 && !result.TimedOut && !result.Running {
		if out.Output != "" {
			out.Output += "\n"
		}
		out.Output += result.Err.Error()
	}
	return out, result.TimedOut
}

func formatPragmaLoopObservation(result pragmaLoopBashResult) string {
	if len(result.Output) < 10_000 {
		return fmt.Sprintf("<returncode>%d</returncode>\n<output>\n%s</output>", result.ReturnCode, result.Output)
	}
	elidedChars := len(result.Output) - 10_000
	return fmt.Sprintf(`<returncode>%d</returncode>
<warning>
The output of your last command was too long.
Please try a different command that produces less output.
If you're inspecting a file, use a narrower file-reading command such as nl with sed.
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
	var body string
	if len(output) < 10_000 {
		body = fmt.Sprintf("<output>\n%s\n</output>", output)
	} else {
		body = fmt.Sprintf(`<warning>Output was too long and has been truncated.</warning>
<output_head>
%s
</output_head>
<elided_chars>%d characters elided</elided_chars>
<output_tail>
%s
</output_tail>`, output[:5000], len(output)-10_000, output[len(output)-5000:])
	}
	return fmt.Sprintf(`The last command <command>%s</command> timed out and has been killed.
The output of the command was:
%s
Please try another command and make sure to avoid those requiring interactive input.`, command, body)
}
