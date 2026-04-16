package worktree

import (
	"fmt"
	"github.com/artpar/pragma/internal/observe"
	"os/exec"
	"regexp"
	"strings"
)

const maxSlugLen = 64

// validSegment matches [a-zA-Z0-9._-] for each segment of a slug.
var validSegment = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// validateSlug checks that a slug is safe for use as a worktree directory name.
// Max 64 chars, each segment (split on /) must match [a-zA-Z0-9._-], no "..",
// no absolute paths.
func validateSlug(slug string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if slug == "" {
		observe.GlobalTrace("if: slug == \"\"")
		observe.GlobalTrace("return: fmt.Errorf(\"slug is empty\")")
		return fmt.Errorf("slug is empty")
	}
	if len(slug) > maxSlugLen {
		observe.GlobalTrace("if: len(slug) > maxSlugLen")
		observe.GlobalTrace("return: fmt.Errorf(\"slug exceeds %d characters\", maxSlugLen)")
		return fmt.Errorf("slug exceeds %d characters", maxSlugLen)
	}
	if strings.HasPrefix(slug, "/") {
		observe.GlobalTrace("if: strings.HasPrefix(slug, \"/\")")
		observe.GlobalTrace("return: fmt.Errorf(\"slug must not be an absolute path\")")
		return fmt.Errorf("slug must not be an absolute path")
	}
	segments := strings.Split(slug, "/")
	for _, seg := range segments {
		observe.GlobalTrace("range segments")
		if seg == "" {
			observe.GlobalTrace("if: seg == \"\"")
			observe.GlobalTrace("return: fmt.Errorf(\"slug contains empty segment\")")
			return fmt.Errorf("slug contains empty segment")
		}
		if seg == ".." {
			observe.GlobalTrace("if: seg == \"..\"")
			observe.GlobalTrace("return: fmt.Errorf(\"slug contains '..' traversal\")")
			return fmt.Errorf("slug contains '..' traversal")
		}
		if !validSegment.MatchString(seg) {
			observe.GlobalTrace("if: !validSegment.MatchString(seg)")
			observe.GlobalTrace("return: fmt.Errorf(\"slug segment %q contains invalid characters (allowed: a-zA-Z0-9._...")
			return fmt.Errorf("slug segment %q contains invalid characters (allowed: a-zA-Z0-9._-)", seg)
		}
	}
	observe.GlobalTrace("return: nil")
	return nil
}

// FlattenSlug converts a slug with path separators to a flat directory name.
// user/feature -> user+feature
func FlattenSlug(slug string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: strings.ReplaceAll(slug, \"/\", \"+\")")
	return strings.ReplaceAll(slug, "/", "+")
}

// HasChanges returns true if the worktree at worktreePath has uncommitted changes
// or new commits beyond headCommit.
func HasChanges(worktreePath, headCommit string) (bool, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	statusCmd := exec.Command("git", "-C", worktreePath, "status", "--porcelain")
	statusOut, err := statusCmd.Output()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: false, fmt.Errorf(\"git status: %w\", err)")
		return false, fmt.Errorf("git status: %w", err)
	}
	if len(strings.TrimSpace(string(statusOut))) > 0 {
		observe.GlobalTrace("if: len(strings.TrimSpace(string(statusOut))) > 0")
		observe.GlobalTrace("return: true, nil")
		return true, nil
	}

	revListCmd := exec.Command("git", "-C", worktreePath, "rev-list", headCommit+"..HEAD", "--count")
	revOut, err := revListCmd.Output()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: false, fmt.Errorf(\"git rev-list: %w\", err)")
		return false, fmt.Errorf("git rev-list: %w", err)
	}
	count := strings.TrimSpace(string(revOut))
	observe.GlobalTrace("return: count != \"0\", nil")
	return count != "0", nil
}
