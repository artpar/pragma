package gitutil

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

const gitTimeout = 2 * time.Second

// RemoteURL returns the origin remote URL for stable project identity.
// Empty string if no remote, not a git repo, or git is unavailable.
func RemoteURL(dir string) string {
	return gitCommand(dir, "remote", "get-url", "origin")
}

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
