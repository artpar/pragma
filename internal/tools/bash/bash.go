package bash

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
	"github.com/artpar/pragma/internal/tools/applypatch"
)

const (
	defaultTimeoutMs = 120_000 // 2 minutes
	maxTimeoutMs     = 600_000 // 10 minutes
)

// BashInput defines the parameters for the Bash tool.
type BashInput struct {
	Command     string `json:"command" desc:"The shell command to execute"`
	Timeout     *int   `json:"timeout,omitempty" desc:"Optional timeout in milliseconds (max 600000)"`
	Background  bool   `json:"background,omitempty" desc:"Run the command detached and return immediately with process status and log file paths"`
	Description string `json:"description,omitempty" desc:"Clear, concise description of what this command does"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"additionalProperties": false,
	"required": ["command"],
	"properties": {
		"command": {
			"type": "string",
			"description": "The command to execute"
		},
		"timeout": {
			"type": "number",
			"description": "Optional timeout in milliseconds (max 600000)"
		},
		"background": {
			"type": "boolean",
			"description": "Run the command detached and return immediately with process status and log file paths"
		},
		"description": {
			"type": "string",
			"description": "Clear, concise description of what this command does"
		}
	}
}`)

// Tool implements the Bash tool for shell command execution.
type Tool struct {
	PatchMode bool
}

func (t *Tool) SetPatchMode(enabled bool) {
	t.PatchMode = enabled
}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Bash\"")
	return "Bash"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Executes a given bash command and returns its output...\"")
	observe.GlobalTrace("return: bashDescription")
	return bashDescription
}

const bashDescription = `Executes a given bash command and returns its output.

The working directory persists between commands, but shell state does not. The shell environment is initialized from the user's profile (bash or zsh).

IMPORTANT: Avoid using this tool for file discovery, content search, file reading, or file editing when the active tool list includes a more specific dedicated tool. Dedicated tools provide a better user experience and make it easier to review calls and permissions. For communication, output text directly instead of using shell output commands.

# Instructions
 - If your command will create new directories or files, first use this tool to run ` + "`ls`" + ` to verify the parent directory exists and is the correct location.
 - Always quote file paths that contain spaces with double quotes in your command (e.g., cd "path with spaces/file.txt")
 - Try to maintain your current working directory throughout the session by using absolute paths and avoiding usage of ` + "`cd`" + `. You may use ` + "`cd`" + ` if the User explicitly requests it.
 - You may specify an optional timeout in milliseconds (up to 600000ms / 10 minutes). By default, your command will timeout after 120000ms (2 minutes).
 - Write a clear, concise description of what your command does. For simple commands, keep it brief (5-10 words). For complex commands (piped commands, obscure flags, or anything hard to understand at a glance), include enough context so that the user can understand what your command will do.
 - When issuing multiple commands:
  - If the commands are independent and can run in parallel, make multiple shell tool calls in a single message.
  - If the commands depend on each other and must run sequentially, use a single shell call with '&&' to chain them together.
  - Use ';' only when you need to run commands sequentially but don't care if earlier commands fail.
  - DO NOT use newlines to separate commands (newlines are ok in quoted strings).
 - For git commands:
  - Prefer to create a new commit rather than amending an existing commit.
  - Before running destructive operations (e.g., git reset --hard, git push --force, git checkout --), consider whether there is a safer alternative that achieves the same goal. Only use destructive operations when they are truly the best approach.
  - Never skip hooks (--no-verify) or bypass signing (--no-gpg-sign, -c commit.gpgsign=false) unless the user has explicitly asked for it. If a hook fails, investigate and fix the underlying issue.
 - Avoid unnecessary ` + "`sleep`" + ` commands:
  - Do not sleep between commands that can run immediately — just run them.
  - Do not retry failing commands in a sleep loop — diagnose the root cause.
  - If you must poll an external process, use a check command (e.g. ` + "`gh run view`" + `) rather than sleeping first.
  - If you must sleep, keep the duration short (1-5 seconds) to avoid blocking the user.

# Committing changes with git

Only create commits when requested by the user. If unclear, ask first. When the user asks you to create a new git commit, follow these steps carefully:

Git Safety Protocol:
- NEVER update the git config
- NEVER run destructive git commands (push --force, reset --hard, checkout ., restore ., clean -f, branch -D) unless the user explicitly requests these actions
- NEVER skip hooks (--no-verify, --no-gpg-sign, etc) unless the user explicitly requests it
- NEVER run force push to main/master, warn the user if they request it
- CRITICAL: Always create NEW commits rather than amending, unless the user explicitly requests a git amend. When a pre-commit hook fails, the commit did NOT happen — so --amend would modify the PREVIOUS commit, which may result in destroying work or losing previous changes. Instead, after hook failure, fix the issue, re-stage, and create a NEW commit
- When staging files, prefer adding specific files by name rather than using "git add -A" or "git add .", which can accidentally include sensitive files (.env, credentials) or large binaries
- NEVER commit changes unless the user explicitly asks you to

1. Run the following bash commands in parallel:
  - Run a git status command to see all untracked files. IMPORTANT: Never use the -uall flag as it can cause memory issues on large repos.
  - Run a git diff command to see both staged and unstaged changes that will be committed.
  - Run a git log command to see recent commit messages, so that you can follow this repository's commit message style.
2. Analyze all staged changes and draft a commit message:
  - Summarize the nature of the changes (eg. new feature, enhancement, bug fix, refactoring, test, docs, etc.)
  - Do not commit files that likely contain secrets (.env, credentials.json, etc). Warn the user if they specifically request to commit those files
  - Draft a concise (1-2 sentences) commit message that focuses on the "why" rather than the "what"
3. Run the following commands:
   - Add relevant untracked files to the staging area.
   - Create the commit with a message.
   - Run git status after the commit completes to verify success.
4. If the commit fails due to pre-commit hook: fix the issue and create a NEW commit

Important notes:
- NEVER run additional commands to read or explore code, besides git bash commands
- Do not switch to unrelated task-management or delegation tools during commit preparation
- DO NOT push to the remote repository unless the user explicitly asks you to do so
- IMPORTANT: Never use git commands with the -i flag (like git rebase -i or git add -i) since they require interactive input which is not supported.
- If there are no changes to commit, do not create an empty commit
- In order to ensure good formatting, ALWAYS pass the commit message via a HEREDOC, a la this example:
<example>
git commit -m "$(cat <<'EOF'
   Commit message here.
   EOF
   )"
</example>

# Creating pull requests
Use the gh command through shell execution for ALL GitHub-related tasks including working with issues, pull requests, checks, and releases. If given a Github URL use the gh command to get the information needed.

IMPORTANT: When the user asks you to create a pull request, follow these steps carefully:

1. Run the following bash commands in parallel, in order to understand the current state of the branch since it diverged from the main branch:
   - Run a git status command to see all untracked files (never use -uall flag)
   - Run a git diff command to see both staged and unstaged changes that will be committed
   - Check if the current branch tracks a remote branch and is up to date with the remote
   - Run a git log command and ` + "`git diff [base-branch]...HEAD`" + ` to understand the full commit history for the current branch
2. Analyze all changes that will be included in the pull request, making sure to look at all relevant commits (NOT just the latest commit, but ALL commits), and draft a pull request title and summary:
   - Keep the PR title short (under 70 characters)
   - Use the description/body for details, not the title
3. Run the following commands:
   - Create new branch if needed
   - Push to remote with -u flag if needed
   - Create PR using gh pr create with the format below. Use a HEREDOC to pass the body to ensure correct formatting.
<example>
gh pr create --title "the pr title" --body "$(cat <<'EOF'
## Summary
<1-3 bullet points>

## Test plan
[Bulleted markdown checklist of TODOs for testing the pull request...]
EOF
)"
</example>

Important:
- Do not switch to unrelated task-management or delegation tools while creating the pull request
- Return the PR URL when you're done, so the user can see it

# Other common operations
- View comments on a Github PR: gh api repos/foo/bar/pulls/123/comments`

func (t *Tool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: inputSchema")
	return inputSchema
}
func (t *Tool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: false, Concurrent: false, Destructive: true}")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: false, Concurrent: false, Destructive: true, MaxResu...")
	return tool.ToolFlags{ReadOnly: false, Concurrent: false, Destructive: true, MaxResultSizeChars: 30_000}
}

func (t *Tool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "bash", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "bash", "Tool.CheckPerm", "exit")
	var in struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &in); err != nil || in.Command == "" {
		observe.TraceCtx(ctx, "bash", "Tool.CheckPerm", "if: err != nil || in.Command == \"\"")
		observe.TraceCtx(ctx, "bash", "Tool.CheckPerm", "return: checker.Check(ctx, \"Bash\", \"\")")
		return checker.Check(ctx, "Bash", "")
	}
	observe.TraceCtx(ctx, "bash", "Tool.CheckPerm", "return: checker.Check(ctx, \"Bash\", in.Command)")
	return checker.Check(ctx, "Bash", in.Command)
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "bash", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "bash", "Tool.Invoke", "exit")
	var in BashInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "bash", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "bash", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.Command == "" {
		observe.TraceCtx(ctx, "bash", "Tool.Invoke", "if: in.Command == \"\"")
		observe.TraceCtx(ctx, "bash", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"command is required\")")
		return tool.InvokeResult{}, fmt.Errorf("command is required")
	}
	if patch, patchWorkDir, ok, err := applypatch.ExtractShellApplyPatch(in.Command); err != nil {
		observe.TraceCtx(ctx, "bash", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "bash", "Tool.Invoke", "return: tool.InvokeResult{}, err")
		return tool.InvokeResult{}, err
	} else if ok {
		observe.TraceCtx(ctx, "bash", "Tool.Invoke", "else-if: ok")
		workDir := state.WorkDir()
		if patchWorkDir != "" {
			if filepath.IsAbs(patchWorkDir) {
				workDir = patchWorkDir
			} else {
				workDir = filepath.Join(state.WorkDir(), patchWorkDir)
			}
		}
		return applypatch.ApplyPatchText(ctx, patch, workDir, state)
	}
	if t.PatchMode {
		observe.TraceCtx(ctx, "bash", "Tool.Invoke", "if: t.PatchMode")
		if reason := sourceMutationReason(in.Command); reason != "" {
			observe.TraceCtx(ctx, "bash", "Tool.Invoke", "if: reason != \"\"")
			observe.TraceCtx(ctx, "bash", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"Bash rejected: %s. Use apply_patch for sourc...")
			return tool.InvokeResult{}, fmt.Errorf("Bash rejected: %s. Use apply_patch for source edits when apply_patch is available; Bash remains available for tests and read-only inspection", reason)
		}
	}

	timeoutMs := defaultTimeoutMs
	if in.Timeout != nil {
		observe.TraceCtx(ctx, "bash", "Tool.Invoke", "if: in.Timeout != nil")
		timeoutMs = *in.Timeout
		if timeoutMs <= 0 {
			observe.TraceCtx(ctx, "bash", "Tool.Invoke", "if: timeoutMs <= 0")
			timeoutMs = defaultTimeoutMs
		}
		if timeoutMs > maxTimeoutMs {
			observe.TraceCtx(ctx, "bash", "Tool.Invoke", "if: timeoutMs > maxTimeoutMs")
			timeoutMs = maxTimeoutMs
		}
	}

	timeout := time.Duration(timeoutMs) * time.Millisecond
	files, err := newCommandFiles()
	if err != nil {
		return tool.InvokeResult{}, err
	}
	if writeErr := os.WriteFile(files.Command, []byte(in.Command), 0o644); writeErr != nil {
		return tool.InvokeResult{}, fmt.Errorf("write command log: %w", writeErr)
	}

	if in.Background {
		return t.invokeBackground(in.Command, timeout, state.WorkDir(), files)
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "bash", "-c", in.Command)
	cmd.Dir = state.WorkDir()
	cmd.WaitDelay = 5 * time.Second

	setProcAttr(cmd)

	stdoutFile, err := os.OpenFile(files.Stdout, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return tool.InvokeResult{}, fmt.Errorf("open stdout log: %w", err)
	}
	defer stdoutFile.Close()
	stderrFile, err := os.OpenFile(files.Stderr, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return tool.InvokeResult{}, fmt.Errorf("open stderr log: %w", err)
	}
	defer stderrFile.Close()
	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile

	err = cmd.Run()

	output := strings.TrimRight(readCommandOutput(files), "\n")

	output = strings.TrimLeft(output, "\n")

	if err != nil {
		observe.TraceCtx(ctx, "bash", "Tool.Invoke", "if: err != nil")
		if cmdCtx.Err() == context.DeadlineExceeded {
			observe.TraceCtx(ctx, "bash", "Tool.Invoke", "if: cmdCtx.Err() == context.DeadlineExceeded")
			if output != "" {
				observe.TraceCtx(ctx, "bash", "Tool.Invoke", "if: output != \"\"")
				output += "\n"
			}
			output += fmt.Sprintf("Command timed out after %dms", timeoutMs)
			output = appendLogPaths(output, files)
			display := fmt.Sprintf("timeout:%d", timeoutMs)
			observe.TraceCtx(ctx, "bash", "Tool.Invoke", "return: tool.InvokeResult{Content: output, Display: display}, nil")
			return tool.InvokeResult{Content: output, Display: display}, nil
		}

		if exitErr, ok := err.(*exec.ExitError); ok {
			observe.TraceCtx(ctx, "bash", "Tool.Invoke", "if: ok")
			exitCode := exitErr.ExitCode()
			if output != "" {
				observe.TraceCtx(ctx, "bash", "Tool.Invoke", "if: output != \"\"")
				output += "\n"
			}
			output += fmt.Sprintf("Exit code %d", exitCode)
			display := fmt.Sprintf("exit_code:%d", exitCode)
			observe.TraceCtx(ctx, "bash", "Tool.Invoke", "return: tool.InvokeResult{Content: output, Display: display}, nil")

			return tool.InvokeResult{Content: output, Display: display}, nil
		}
		observe.TraceCtx(ctx, "bash", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"execute command: %w\", err)")

		return tool.InvokeResult{}, fmt.Errorf("execute command: %w", err)
	}
	observe.TraceCtx(ctx, "bash", "Tool.Invoke", "return: tool.InvokeResult{Content: output}, nil")
	if commandMayUseBackground(in.Command) {
		output = appendLogPaths(output, files)
	}

	return tool.InvokeResult{Content: output}, nil
}

type commandFiles struct {
	Dir     string
	Command string
	Stdout  string
	Stderr  string
	Status  string
}

func newCommandFiles() (commandFiles, error) {
	base := filepath.Join(os.TempDir(), "pragma-bash")
	if err := os.MkdirAll(base, 0o755); err != nil {
		return commandFiles{}, fmt.Errorf("create bash log directory: %w", err)
	}
	dir, err := os.MkdirTemp(base, "cmd-")
	if err != nil {
		return commandFiles{}, fmt.Errorf("create bash command directory: %w", err)
	}
	return commandFiles{
		Dir:     dir,
		Command: filepath.Join(dir, "command.sh"),
		Stdout:  filepath.Join(dir, "stdout.log"),
		Stderr:  filepath.Join(dir, "stderr.log"),
		Status:  filepath.Join(dir, "status.txt"),
	}, nil
}

func (t *Tool) invokeBackground(command string, timeout time.Duration, workDir string, files commandFiles) (tool.InvokeResult, error) {
	cmdCtx, cancel := context.WithTimeout(context.Background(), timeout)
	cmd := exec.CommandContext(cmdCtx, "bash", "-c", command)
	cmd.Dir = workDir
	cmd.WaitDelay = 5 * time.Second
	setProcAttr(cmd)

	stdoutFile, err := os.OpenFile(files.Stdout, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		cancel()
		return tool.InvokeResult{}, fmt.Errorf("open stdout log: %w", err)
	}
	stderrFile, err := os.OpenFile(files.Stderr, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		cancel()
		stdoutFile.Close()
		return tool.InvokeResult{}, fmt.Errorf("open stderr log: %w", err)
	}
	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile

	if err := cmd.Start(); err != nil {
		cancel()
		stdoutFile.Close()
		stderrFile.Close()
		return tool.InvokeResult{}, fmt.Errorf("start background command: %w", err)
	}

	pid := cmd.Process.Pid
	_ = os.WriteFile(files.Status, []byte(fmt.Sprintf("running\npid=%d\nstarted=%s\n", pid, time.Now().Format(time.RFC3339))), 0o644)
	go func() {
		defer cancel()
		defer stdoutFile.Close()
		defer stderrFile.Close()

		err := cmd.Wait()
		status := "exited"
		detail := "exit_code=0"
		if cmdCtx.Err() == context.DeadlineExceeded {
			status = "timeout"
			detail = fmt.Sprintf("timeout_ms=%d", timeout.Milliseconds())
		} else if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				detail = fmt.Sprintf("exit_code=%d", exitErr.ExitCode())
			} else {
				status = "error"
				detail = fmt.Sprintf("error=%s", err.Error())
			}
		}
		_ = os.WriteFile(files.Status, []byte(fmt.Sprintf("%s\npid=%d\n%s\nended=%s\n", status, pid, detail, time.Now().Format(time.RFC3339))), 0o644)
	}()

	content := fmt.Sprintf(`Started background command.
PID: %d
Command: %s
Status: %s
Stdout: %s
Stderr: %s

Poll with: cat %q
Inspect logs with: tail -100 %q %q`, pid, files.Command, files.Status, files.Stdout, files.Stderr, files.Status, files.Stdout, files.Stderr)
	return tool.InvokeResult{Content: content, Display: "background"}, nil
}

func readCommandOutput(files commandFiles) string {
	var parts []string
	if stdout, err := os.ReadFile(files.Stdout); err == nil && len(stdout) > 0 {
		parts = append(parts, string(stdout))
	}
	if stderr, err := os.ReadFile(files.Stderr); err == nil && len(stderr) > 0 {
		parts = append(parts, string(stderr))
	}
	return strings.Join(parts, "\n")
}

func commandMayUseBackground(command string) bool {
	return strings.Contains(command, "&")
}

func appendLogPaths(output string, files commandFiles) string {
	if output != "" {
		output += "\n"
	}
	return output + fmt.Sprintf("Logs:\nStdout: %s\nStderr: %s", files.Stdout, files.Stderr)
}

func sourceMutationReason(command string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	fields := strings.Fields(command)
	for i, field := range fields {
		observe.GlobalTrace("range fields")
		base := filepath.Base(field)
		switch base {
		case "sed", "gsed", "perl":
			observe.GlobalTrace("case: \"sed\", \"gsed\", \"perl\"")
			if i+1 < len(fields) && strings.HasPrefix(fields[i+1], "-i") {
				observe.GlobalTrace("return: base + \" in-place edits are blocked\"")
				return base + " in-place edits are blocked"
			}
		case "tee":
			observe.GlobalTrace("case: \"tee\"")
			return "tee writes are blocked"
		case "python", "python3", "perl5":
			observe.GlobalTrace("case: \"python\", \"python3\", \"perl5\"")
			if shellContainsWriteIntent(command) {
				observe.GlobalTrace("return: base + \" file-write snippets are blocked\"")
				return base + " file-write snippets are blocked"
			}
		}
	}

	normalized := " " + strings.Join(fields, " ") + " "
	for _, pat := range []string{
		" cat > ",
		" cat >> ",
		" echo > ",
		" echo >> ",
		" printf > ",
		" printf >> ",
		" tee -a ",
	} {
		observe.GlobalTrace("range []string{\n\t\" cat > \",\n\t\" cat >> \",\n\t\" echo > \",\n\t\" echo >> \",\n\t\" printf > \",\n...")
		if strings.Contains(normalized, pat) {
			observe.GlobalTrace("if: strings.Contains(normalized, pat)")
			observe.GlobalTrace("return: \"shell redirection writes are blocked\"")
			return "shell redirection writes are blocked"
		}
	}
	observe.GlobalTrace("return: \"\"")
	return ""
}

func shellContainsWriteIntent(command string) bool {
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
