package slash

import (
	"context"
	"fmt"

	"github.com/artpar/pragma/internal/observe"
)

const commitPromptTemplate = `## Context

Use the Bash tool to inspect the repository before committing. At minimum, gather:

- Current git status
- Current git diff, including staged and unstaged changes
- Current branch
- Recent commit messages

## Git Safety Protocol

- NEVER update the git config
- NEVER skip hooks (--no-verify, --no-gpg-sign, etc) unless the user explicitly requests it
- CRITICAL: ALWAYS create NEW commits. NEVER use git commit --amend, unless the user explicitly requests it
- Do not commit files that likely contain secrets (.env, credentials.json, etc). Warn the user if they specifically request to commit those files
- If there are no changes to commit (i.e., no untracked files and no modifications), do not create an empty commit
- Never use git commands with the -i flag (like git rebase -i or git add -i) since they require interactive input which is not supported

## Your task

Based on the above changes, create a single git commit:

1. Use Bash to inspect the current repository state and recent commit style.
2. Analyze all staged changes and draft a commit message:
   - Summarize the nature of the changes (new feature, enhancement, bug fix, refactoring, test, docs, etc.)
   - Ensure the message accurately reflects the changes and their purpose (i.e. "add" means a wholly new feature, "update" means an enhancement to an existing feature, "fix" means a bug fix, etc.)
   - Draft a concise (1-2 sentences) commit message that focuses on the "why" rather than the "what"

3. Stage relevant files and create the commit using HEREDOC syntax:
` + "```" + `
git commit -m "$(cat <<'EOF'
Commit message here.%s
EOF
)"
` + "```" + `

You have the capability to call multiple tools in a single response. Stage and create the commit using a single message. Do not use any other tools or do anything else. Do not send any other text or messages besides these tool calls.`

func handleCommit(ctx context.Context, _ string, deps Deps) (Result, error) {
	observe.TraceCtx(ctx, "slash", "handleCommit", "enter")
	defer observe.TraceCtx(ctx, "slash", "handleCommit", "exit")

	attribution := ""
	modelName := deps.ModelName
	if modelName != "" {
		observe.TraceCtx(ctx, "slash", "handleCommit", "if: modelName != \"\"")
		attribution = fmt.Sprintf("\n\nCo-Authored-By: %s <noreply@anthropic.com>", modelName)
	}

	promptWithAttribution := fmt.Sprintf(commitPromptTemplate, attribution)
	observe.TraceCtx(ctx, "slash", "handleCommit", "return: Result{InjectPrompt: promptWithAttribution}, nil")
	return Result{InjectPrompt: promptWithAttribution}, nil
}
