package slash

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/observe"
)

func handleResume(_ context.Context, args string, deps Deps) (Result, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	if deps.SessionStore == nil {
		return Result{DisplayText: "Session store not available."}, nil
	}

	args = strings.TrimSpace(args)

	// No args: open interactive dialog
	if args == "" {
		return Result{ShowResumeDialog: true}, nil
	}

	// With args: prefix-match on session ID
	summaries, err := deps.SessionStore.List()
	if err != nil {
		return Result{DisplayText: fmt.Sprintf("Error listing sessions: %s", err)}, nil
	}

	var matches []string
	for _, s := range summaries {
		if strings.HasPrefix(s.ID, args) {
			matches = append(matches, s.ID)
		}
	}

	if len(matches) == 0 {
		return Result{DisplayText: fmt.Sprintf("No session found matching %q", args)}, nil
	}
	if len(matches) > 1 {
		var b strings.Builder
		fmt.Fprintf(&b, "Ambiguous session ID %q — %d matches:\n", args, len(matches))
		for _, id := range matches {
			fmt.Fprintf(&b, "  %s\n", id)
		}
		return Result{DisplayText: b.String()}, nil
	}

	// Exact single match — load and display summary
	sess, err := deps.SessionStore.Load(matches[0])
	if err != nil {
		return Result{DisplayText: fmt.Sprintf("Error loading session: %s", err)}, nil
	}

	summary := sess.Summary
	if summary == "" {
		summary = "(no summary)"
	}
	dir := filepath.Base(sess.Conversation.WorkDir)
	ago := formatTimeAgo(sess.Conversation.UpdatedAt)

	shortID := sess.Conversation.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}

	return Result{
		DisplayText: fmt.Sprintf("Resumed session %s\n  %s — %s — %d turns — %s",
			shortID, summary, dir, sess.TurnCount, ago),
		ResumeSessionID: matches[0],
	}, nil
}

// formatTimeAgo returns a human-readable relative time string.
func formatTimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}
