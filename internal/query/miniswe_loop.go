package query

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/hook"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/provider"
)

const pragmaLoopSystemPrompt = `Pragma loop mode is a shell-action transport.

Follow the active task and persona instructions. This wrapper only defines how
to send the next shell action.

Your response must contain one fenced bash code block with one command or one shell script.
Do not write prose, analysis sections, headings, or bullets outside the bash code block.
Use commands, command output, and required task artifacts for reasoning and evidence.
When you have done your part of the persona job ` + "`echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT` to signal finish" + `.
Format your response as shown in <format_example>.

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

const pragmaLoopFormatErrorTemplate = `Please always provide EXACTLY ONE bash action in triple backticks and no prose outside the code block. Found %d actions.
If you want to end the task, please issue the following command: ` + "`echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT`" + `
without any other command. YOU HAVE TO PUT IT in triple backticks like any other command.
Else, please format your response exactly as follows:

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
		if e.hookMgr != nil {
			hookCtx, hookCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer hookCancel()
			e.hookMgr.Execute(hookCtx, hook.Stop, hook.HookInput{})
		}
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

		messagesForQuery := e.messagesForRequest(snap.Conversation)
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
		if actionCount != 1 {
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
		if submitted, message := pragmaLoopSubmitted(result); submitted {
			if err := e.appendPragmaLoopUserMessage(message); err != nil {
				ch <- ErrorEvent{Err: err}
				return
			}
			ch <- TurnCompleteEvent{Response: response, StopReason: model.StopEndTurn}
			return
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
	cmdCtx, cancel := context.WithTimeout(ctx, pragmaLoopCommandTimeout)
	defer cancel()
	shellCommand := commandForPragmaLoopShellRun(command)
	cmd := exec.CommandContext(cmdCtx, "bash", "-o", "pipefail", "-c", shellCommand)
	cmd.Dir = workDir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	files, err := newPragmaLoopCommandFiles(command)
	if err != nil {
		return pragmaLoopBashResult{ReturnCode: -1, Output: err.Error()}, false
	}
	consoleFile, err := os.OpenFile(files.console, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		cancel()
		return pragmaLoopBashResult{ReturnCode: -1, Output: err.Error()}, false
	}
	cmd.Stdout = consoleFile
	cmd.Stderr = consoleFile

	if err := cmd.Start(); err != nil {
		cancel()
		consoleFile.Close()
		return pragmaLoopBashResult{ReturnCode: -1, Output: err.Error()}, false
	}
	pid := cmd.Process.Pid
	_ = os.WriteFile(files.status, []byte(fmt.Sprintf("running\npid=%d\nstarted=%s\n", pid, time.Now().Format(time.RFC3339))), 0o644)

	done := make(chan error, 1)
	go func() {
		defer cancel()
		err := cmd.Wait()
		consoleFile.Close()
		writePragmaLoopCommandStatus(files.status, pid, cmdCtx, err)
		done <- err
	}()

	var waitErr error
	timedOut := false
	softTimer := time.NewTimer(pragmaLoopForegroundWait)
	defer softTimer.Stop()
	select {
	case waitErr = <-done:
		cancel()
	case <-softTimer.C:
		output := tailLines(readPragmaLoopCommandOutput(files), pragmaLoopRunningOutputLines)
		output = appendPragmaLoopRunningProcess(output, pid, files)
		return pragmaLoopBashResult{ReturnCode: -1, Output: output}, false
	case <-cmdCtx.Done():
		timedOut = true
		select {
		case waitErr = <-done:
		case <-time.After(100 * time.Millisecond):
		}
	}
	if timedOut {
		return pragmaLoopBashResult{ReturnCode: -1, Output: readPragmaLoopCommandOutput(files)}, true
	}

	output := readPragmaLoopCommandOutput(files)
	result := pragmaLoopBashResult{Output: output}
	if waitErr == nil {
		result.ReturnCode = 0
		return result, timedOut
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		result.ReturnCode = exitErr.ExitCode()
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			result.ReturnCode = -int(status.Signal())
		}
		return result, timedOut
	}
	result.ReturnCode = -1
	if result.Output != "" {
		result.Output += "\n"
	}
	result.Output += waitErr.Error()
	return result, timedOut
}

type pragmaLoopCommandFiles struct {
	dir     string
	command string
	console string
	status  string
}

func newPragmaLoopCommandFiles(command string) (pragmaLoopCommandFiles, error) {
	base := filepath.Join(os.TempDir(), "pragma-loop-bash")
	if err := os.MkdirAll(base, 0o755); err != nil {
		return pragmaLoopCommandFiles{}, fmt.Errorf("create pragma loop log directory: %w", err)
	}
	dir, err := os.MkdirTemp(base, "cmd-")
	if err != nil {
		return pragmaLoopCommandFiles{}, fmt.Errorf("create pragma loop command directory: %w", err)
	}
	files := pragmaLoopCommandFiles{
		dir:     dir,
		command: filepath.Join(dir, "command.sh"),
		console: filepath.Join(dir, "console.log"),
		status:  filepath.Join(dir, "status.txt"),
	}
	if err := os.WriteFile(files.command, []byte(command), 0o644); err != nil {
		return pragmaLoopCommandFiles{}, fmt.Errorf("write pragma loop command log: %w", err)
	}
	return files, nil
}

func readPragmaLoopCommandOutput(files pragmaLoopCommandFiles) string {
	output, err := os.ReadFile(files.console)
	if err != nil || len(output) == 0 {
		return ""
	}
	return string(output)
}

func writePragmaLoopCommandStatus(statusPath string, pid int, cmdCtx context.Context, err error) {
	status := "exited"
	detail := "exit_code=0"
	if cmdCtx.Err() == context.DeadlineExceeded {
		status = "timeout"
		detail = fmt.Sprintf("timeout_ms=%d", pragmaLoopCommandTimeout.Milliseconds())
	} else if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			detail = fmt.Sprintf("exit_code=%d", exitErr.ExitCode())
		} else {
			status = "error"
			detail = fmt.Sprintf("error=%s", err.Error())
		}
	}
	_ = os.WriteFile(statusPath, []byte(fmt.Sprintf("%s\npid=%d\n%s\nended=%s\n", status, pid, detail, time.Now().Format(time.RFC3339))), 0o644)
}

func appendPragmaLoopRunningProcess(output string, pid int, files pragmaLoopCommandFiles) string {
	if output != "" {
		output = fmt.Sprintf("Console tail (last %d lines):\n%s\n", pragmaLoopRunningOutputLines, output)
	}
	if output != "" {
		output += "\n"
	}
	return output + fmt.Sprintf(`Command is still running after %s.
PID: %d
Command: %s
Status: %s
Console: %s

Active processes:
%s

Poll with: cat %q
Inspect logs with: tail -100 %q`, pragmaLoopForegroundWait, pid, files.command, files.status, files.console, pragmaLoopProcessGroupSummary(pid), files.status, files.console)
}

func tailLines(output string, maxLines int) string {
	output = strings.TrimRight(output, "\n")
	if output == "" || maxLines <= 0 {
		return ""
	}
	lines := strings.Split(output, "\n")
	if len(lines) <= maxLines {
		return output
	}
	return strings.Join(lines[len(lines)-maxLines:], "\n")
}

func pragmaLoopProcessGroupSummary(pid int) string {
	out, err := exec.Command("ps", "-o", "pid,ppid,pgid,stat,etime,comm,args", "-g", strconv.Itoa(pid)).Output()
	if err != nil || len(out) == 0 {
		return fmt.Sprintf("pid=%d", pid)
	}
	return strings.TrimRight(string(out), "\n")
}

func commandForPragmaLoopShellRun(command string) string {
	if !strings.Contains(command, "&") {
		return command
	}
	return command + "\n__pragma_status=$?\ndisown -a 2>/dev/null || true\nexit $__pragma_status"
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

func pragmaLoopSubmitted(result pragmaLoopBashResult) (bool, string) {
	lines := strings.SplitAfter(strings.TrimLeft(result.Output, "\r\n\t "), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return false, ""
	}
	first := strings.TrimSpace(lines[0])
	if first != "MINI_SWE_AGENT_FINAL_OUTPUT" && first != "COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT" {
		return false, ""
	}
	if result.ReturnCode != 0 {
		return false, ""
	}
	if len(lines) == 1 {
		return true, ""
	}
	return true, strings.Join(lines[1:], "")
}
