package bash

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
)

const (
	defaultTimeoutMs = 120_000 // 2 minutes
	maxTimeoutMs     = 600_000 // 10 minutes
)

// BashInput defines the parameters for the Bash tool.
type BashInput struct {
	Command     string `json:"command" desc:"The shell command to execute"`
	Timeout     *int   `json:"timeout,omitempty" desc:"Optional timeout in milliseconds (max 600000)"`
	Description string `json:"description,omitempty" desc:"Clear, concise description of what this command does"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
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
		"description": {
			"type": "string",
			"description": "Clear, concise description of what this command does"
		}
	}
}`)

// Tool implements the Bash tool for shell command execution.
type Tool struct{}

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

IMPORTANT: Avoid using this tool to run ` + "`find`, `grep`, `cat`, `head`, `tail`, `sed`, `awk`, or `echo`" + ` commands, unless explicitly instructed or after you have verified that a dedicated tool cannot accomplish your task. Instead, use the appropriate dedicated tool as this will provide a much better experience for the user:

 - File search: Use Glob (NOT find or ls)
 - Content search: Use Grep (NOT grep or rg)
 - Read files: Use Read (NOT cat/head/tail)
 - Edit files: Use Edit (NOT sed/awk)
 - Write files: Use Write (NOT echo >/cat <<EOF)
 - Communication: Output text directly (NOT echo/printf)
While the Bash tool can do similar things, it's better to use the built-in tools as they provide a better user experience and make it easier to review tool calls and give permission.

# Instructions
 - If your command will create new directories or files, first use this tool to run ` + "`ls`" + ` to verify the parent directory exists and is the correct location.
 - Always quote file paths that contain spaces with double quotes in your command (e.g., cd "path with spaces/file.txt")
 - Try to maintain your current working directory throughout the session by using absolute paths and avoiding usage of ` + "`cd`" + `. You may use ` + "`cd`" + ` if the User explicitly requests it.
 - You may specify an optional timeout in milliseconds (up to 600000ms / 10 minutes). By default, your command will timeout after 120000ms (2 minutes).
 - You can use the ` + "`run_in_background`" + ` parameter to run the command in the background. Only use this if you don't need the result immediately and are OK being notified when the command completes later. You do not need to check the output right away - you'll be notified when it finishes. You do not need to use '&' at the end of the command when using this parameter.
 - Write a clear, concise description of what your command does. For simple commands, keep it brief (5-10 words). For complex commands (piped commands, obscure flags, or anything hard to understand at a glance), include enough context so that the user can understand what your command will do.
 - When issuing multiple commands:
  - If the commands are independent and can run in parallel, make multiple Bash tool calls in a single message.
  - If the commands depend on each other and must run sequentially, use a single Bash call with '&&' to chain them together.
  - Use ';' only when you need to run commands sequentially but don't care if earlier commands fail.
  - DO NOT use newlines to separate commands (newlines are ok in quoted strings).
 - For git commands:
  - Prefer to create a new commit rather than amending an existing commit.
  - Before running destructive operations (e.g., git reset --hard, git push --force, git checkout --), consider whether there is a safer alternative that achieves the same goal. Only use destructive operations when they are truly the best approach.
  - Never skip hooks (--no-verify) or bypass signing (--no-gpg-sign, -c commit.gpgsign=false) unless the user has explicitly asked for it. If a hook fails, investigate and fix the underlying issue.
 - Avoid unnecessary ` + "`sleep`" + ` commands:
  - Do not sleep between commands that can run immediately — just run them.
  - If your command is long running and you would like to be notified when it finishes — use ` + "`run_in_background`" + `. No sleep needed.
  - Do not retry failing commands in a sleep loop — diagnose the root cause.
  - If waiting for a background task you started with ` + "`run_in_background`" + `, you will be notified when it completes — do not poll.
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
- NEVER use the TodoWrite or Agent tools
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
Use the gh command via the Bash tool for ALL GitHub-related tasks including working with issues, pull requests, checks, and releases. If given a Github URL use the gh command to get the information needed.

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
- DO NOT use the TodoWrite or Agent tools
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
	return tool.ToolFlags{ReadOnly: false, Concurrent: false, Destructive: true}
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
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "bash", "-c", in.Command)
	cmd.Dir = state.WorkDir()

	setProcAttr(cmd)

	// Capture combined stdout+stderr (merged fd, like TS)
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined

	err := cmd.Run()

	output := strings.TrimRight(combined.String(), "\n")

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
			observe.TraceCtx(ctx, "bash", "Tool.Invoke", "return: tool.InvokeResult{Content: output}, nil")
			return tool.InvokeResult{Content: output}, nil
		}

		if exitErr, ok := err.(*exec.ExitError); ok {
			observe.TraceCtx(ctx, "bash", "Tool.Invoke", "if: ok")
			exitCode := exitErr.ExitCode()
			if output != "" {
				observe.TraceCtx(ctx, "bash", "Tool.Invoke", "if: output != \"\"")
				output += "\n"
			}
			output += fmt.Sprintf("Exit code %d", exitCode)
			observe.TraceCtx(ctx, "bash", "Tool.Invoke", "return: tool.InvokeResult{Content: output}, nil")

			return tool.InvokeResult{Content: output}, nil
		}
		observe.TraceCtx(ctx, "bash", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"execute command: %w\", err)")

		return tool.InvokeResult{}, fmt.Errorf("execute command: %w", err)
	}
	observe.TraceCtx(ctx, "bash", "Tool.Invoke", "return: tool.InvokeResult{Content: output}, nil")

	return tool.InvokeResult{Content: output}, nil
}
