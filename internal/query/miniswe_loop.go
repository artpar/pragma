package query

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
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
		startIndex := len(e.store.Snapshot().Conversation.Messages)
		e.runPragmaLoopWithInitialPrompt(ctx, system, userMessage, ch, startIndex)
	}()
	return ch
}

func (e *Engine) runPragmaLoopWithInitialPrompt(ctx context.Context, system model.SystemPrompt, userMessage string, ch chan<- LoopEvent, messageStartIndexes ...int) {
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

	const maxNoActionRetries = 3
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

		messagesForQuery, err := e.messagesForRequestChecked(snap.Conversation, messageStartIndexes...)
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

		var response model.Response
		var assistantText string
		var command string
		var actionCount int
		for noActionRetries := 0; ; noActionRetries++ {
			ch <- ModelRequestEvent{Model: resolvedModel, Attempt: noActionRetries + 1}
			var err error
			response, err = e.completePragmaLoopResponse(ctx, params, ch)
			if err != nil {
				ch <- ErrorEvent{Err: err}
				return
			}
			assistantText = responseText(response)
			command, actionCount = extractPragmaLoopCommand(assistantText)
			if actionCount != 0 || strings.Contains(assistantText, "COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT") {
				break
			}
			if e.config.RequireStructuredOutput {
				ch <- ErrorEvent{Err: fmt.Errorf("structured output was not produced")}
				return
			}
			if noActionRetries >= maxNoActionRetries {
				ch <- ErrorEvent{Err: fmt.Errorf("model returned no bash action and no completion sentinel after %d retries", maxNoActionRetries)}
				return
			}
		}
		emitPragmaLoopResponseText(response, ch)
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

		if actionCount == 0 {
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
		if submitted, _ := pragmaLoopSubmitted(result); submitted {
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

func emitPragmaLoopResponseText(response model.Response, ch chan<- LoopEvent) {
	for _, part := range response.Content {
		if text, ok := part.(model.TextPart); ok && text.Text != "" {
			ch <- TextEvent{Text: text.Text}
		}
	}
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

type pragmaLoopPatchState struct {
	workDir string
}

func (s pragmaLoopPatchState) WorkDir() string {
	return s.workDir
}

func runPragmaLoopBash(ctx context.Context, workDir, command string) (pragmaLoopBashResult, bool) {
	if patch, patchWorkDir, ok, err := applypatch.ExtractShellApplyPatch(command); err != nil {
		return pragmaLoopBashResult{ReturnCode: 1, Output: err.Error()}, false
	} else if ok {
		applyWorkDir := workDir
		if patchWorkDir != "" {
			if filepath.IsAbs(patchWorkDir) {
				applyWorkDir = patchWorkDir
			} else {
				applyWorkDir = filepath.Join(workDir, patchWorkDir)
			}
		}
		result, err := applypatch.ApplyPatchText(ctx, patch, applyWorkDir, pragmaLoopPatchState{workDir: applyWorkDir})
		if err != nil {
			return pragmaLoopBashResult{ReturnCode: 1, Output: err.Error()}, false
		}
		return pragmaLoopBashResult{ReturnCode: 0, Output: result.Content}, false
	}
	if reason := pragmaLoopSourceMutationReason(workDir, command); reason != "" {
		return pragmaLoopBashResult{
			ReturnCode: 1,
			Output:     fmt.Sprintf("Bash rejected: %s. Use apply_patch <<'PATCH' for repository source edits; Bash remains available for read-only inspection, validation, and runtime artifact writes.", reason),
		}, false
	}
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

func pragmaLoopSourceMutationReason(workDir, command string) string {
	fields := strings.Fields(command)
	for i, field := range fields {
		base := filepath.Base(strings.Trim(field, `"'`))
		switch base {
		case "sed", "gsed", "perl":
			if i+1 < len(fields) && strings.HasPrefix(strings.Trim(fields[i+1], `"'`), "-i") && commandMentionsRepoSourcePath(workDir, strings.Join(fields[i+2:], " ")) {
				return base + " in-place edits to repository source are blocked"
			}
		case "tee":
			if firstRepoSourcePath(workDir, fields[i+1:]) != "" {
				return "tee writes to repository source are blocked"
			}
		case "python", "python3", "perl5":
			if commandContainsWriteIntent(command) && commandMentionsRepoSourcePath(workDir, command) {
				return base + " file-write snippets to repository source are blocked"
			}
		}
	}

	for i, field := range fields {
		switch field {
		case ">", ">>":
			if i+1 < len(fields) && isRepoSourcePath(workDir, cleanShellPathToken(fields[i+1])) {
				return "shell redirection writes to repository source are blocked"
			}
		default:
			if strings.HasPrefix(field, ">") {
				path := strings.TrimPrefix(strings.TrimPrefix(field, ">>"), ">")
				if isRepoSourcePath(workDir, cleanShellPathToken(path)) {
					return "shell redirection writes to repository source are blocked"
				}
			}
		}
	}
	return ""
}

func commandContainsWriteIntent(command string) bool {
	lower := strings.ToLower(command)
	for _, needle := range []string{
		"write_text(",
		"write_bytes(",
		"os.writefile(",
		"os.remove(",
		"os.rename(",
		".write(",
	} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func commandMentionsRepoSourcePath(workDir, command string) bool {
	for _, field := range strings.Fields(command) {
		if isRepoSourcePath(workDir, cleanShellPathToken(field)) {
			return true
		}
	}
	return false
}

func firstRepoSourcePath(workDir string, fields []string) string {
	for _, field := range fields {
		path := cleanShellPathToken(field)
		if strings.HasPrefix(path, "-") {
			continue
		}
		if isRepoSourcePath(workDir, path) {
			return path
		}
	}
	return ""
}

func cleanShellPathToken(token string) string {
	token = strings.TrimSpace(token)
	token = strings.Trim(token, `"'`)
	token = strings.TrimSuffix(token, ";")
	token = strings.TrimSuffix(token, `\`)
	return token
}

func isRepoSourcePath(workDir, path string) bool {
	if path == "" || strings.HasPrefix(path, "-") || strings.HasPrefix(path, "$") {
		return false
	}
	if strings.HasPrefix(path, "/tmp/pragma/") || path == "/tmp/pragma" {
		return false
	}
	clean := filepath.Clean(path)
	if filepath.IsAbs(clean) {
		if workDir == "" {
			return false
		}
		absWorkDir, err := filepath.Abs(workDir)
		if err != nil {
			absWorkDir = workDir
		}
		rel, err := filepath.Rel(absWorkDir, clean)
		if err != nil || strings.HasPrefix(rel, "..") || rel == "." {
			return strings.HasPrefix(clean, "/app/") && looksLikeSourcePath(strings.TrimPrefix(clean, "/app/"))
		}
		clean = rel
	}
	clean = strings.TrimPrefix(clean, "./")
	return looksLikeSourcePath(clean)
}

func looksLikeSourcePath(path string) bool {
	if path == "" || strings.HasPrefix(path, "..") || strings.Contains(path, "*") {
		return false
	}
	switch filepath.Ext(path) {
	case ".go", ".py", ".js", ".jsx", ".ts", ".tsx", ".java", ".rs", ".c", ".cc", ".cpp", ".h", ".hpp", ".proto", ".yaml", ".yml", ".json", ".toml", ".md":
	default:
		return false
	}
	first := path
	if idx := strings.IndexRune(path, filepath.Separator); idx >= 0 {
		first = path[:idx]
	}
	switch first {
	case "cmd", "internal", "pkg", "api", "src", "lib", "server", "client", "web", "config", "configs", "tools", "test", "tests":
		return true
	default:
		return !filepath.IsAbs(path) && !strings.HasPrefix(path, "/")
	}
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
	if result.ReturnCode != 0 {
		return false, ""
	}
	return strings.Contains(result.Output, "MINI_SWE_AGENT_FINAL_OUTPUT") ||
		strings.Contains(result.Output, "COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT"), ""
}
