package sysprompt

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

const gitTimeout = 2 * time.Second

// GitRoot returns the git repository root for the given directory,
// or empty string if not inside a git repo or git is unavailable.
func GitRoot(dir string) string {
	return gitCommand(dir, "rev-parse", "--show-toplevel")
}

// GitBranch returns the current branch name, or empty string.
func GitBranch(dir string) string {
	return gitCommand(dir, "rev-parse", "--abbrev-ref", "HEAD")
}

// GitRemoteURL returns the origin remote URL for stable project identity.
// Empty string if no remote or not a git repo.
func GitRemoteURL(dir string) string {
	return gitCommand(dir, "remote", "get-url", "origin")
}

// gitCommand runs a git command in the given directory with a timeout.
// Returns trimmed stdout on success, empty string on any error.
func gitCommand(dir string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
