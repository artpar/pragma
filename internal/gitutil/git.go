package gitutil

import (
	"context"
	"github.com/artpar/pragma/internal/observe"
	"os/exec"
	"strings"
	"time"
)

const gitTimeout = 2 * time.Second

// RemoteURL returns the origin remote URL for stable project identity.
// Empty string if no remote, not a git repo, or git is unavailable.
func RemoteURL(dir string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: gitCommand(dir, \"remote\", \"get-url\", \"origin\")")
	return gitCommand(dir, "remote", "get-url", "origin")
}

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
		return ""
	}
	observe.GlobalTrace("return: strings.TrimSpace(string(out))")
	return strings.TrimSpace(string(out))
}
