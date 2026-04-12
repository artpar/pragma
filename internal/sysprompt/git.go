package sysprompt

import (
	"context"
	"github.com/artpar/gogent/internal/observe"
	"os/exec"
	"strings"
	"time"
)

const gitTimeout = 2 * time.Second

// GitRoot returns the git repository root for the given directory,
// or empty string if not inside a git repo or git is unavailable.
func GitRoot(dir string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: gitCommand(dir, \"rev-parse\", \"--show-toplevel\")")
	observe.GlobalTrace("return: gitCommand(dir, \"rev-parse\", \"--show-toplevel\")")
	observe.GlobalTrace("return: gitCommand(dir, \"rev-parse\", \"--show-toplevel\")")
	return gitCommand(dir, "rev-parse", "--show-toplevel")
}

// GitBranch returns the current branch name, or empty string.
func GitBranch(dir string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: gitCommand(dir, \"rev-parse\", \"--abbrev-ref\", \"HEAD\")")
	observe.GlobalTrace("return: gitCommand(dir, \"rev-parse\", \"--abbrev-ref\", \"HEAD\")")
	observe.GlobalTrace("return: gitCommand(dir, \"rev-parse\", \"--abbrev-ref\", \"HEAD\")")
	return gitCommand(dir, "rev-parse", "--abbrev-ref", "HEAD")
}

// GitRemoteURL returns the origin remote URL for stable project identity.
// Empty string if no remote or not a git repo.
func GitRemoteURL(dir string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: gitCommand(dir, \"remote\", \"get-url\", \"origin\")")
	observe.GlobalTrace("return: gitCommand(dir, \"remote\", \"get-url\", \"origin\")")
	observe.GlobalTrace("return: gitCommand(dir, \"remote\", \"get-url\", \"origin\")")
	return gitCommand(dir, "remote", "get-url", "origin")
}

// gitCommand runs a git command in the given directory with a timeout.
// Returns trimmed stdout on success, empty string on any error.
func gitCommand(dir string, args ...string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\"")
		observe.GlobalTrace("return: \"\"")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	observe.GlobalTrace("return: strings.TrimSpace(string(out))")
	observe.GlobalTrace("return: strings.TrimSpace(string(out))")
	observe.GlobalTrace("return: strings.TrimSpace(string(out))")
	return strings.TrimSpace(string(out))
}
