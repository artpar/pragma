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
		observe.GlobalTrace("if: deps.SessionStore == nil")
		observe.GlobalTrace("return: Result{DisplayText: \"Session store not available.\"}, nil")
		return Result{DisplayText: "Session store not available."}, nil
	}

	args = strings.TrimSpace(args)

	if args == "" {
		observe.GlobalTrace("if: args == \"\"")
		observe.GlobalTrace("return: Result{ShowResumeDialog: true}, nil")
		return Result{ShowResumeDialog: true}, nil
	}

	summaries, err := deps.SessionStore.List()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: Result{DisplayText: fmt.Sprintf(\"Error listing sessions: %s\", err)}, nil")
		return Result{DisplayText: fmt.Sprintf("Error listing sessions: %s", err)}, nil
	}

	var matches []string
	for _, s := range summaries {
		observe.GlobalTrace("range summaries")
		if strings.HasPrefix(s.ID, args) {
			observe.GlobalTrace("if: strings.HasPrefix(s.ID, args)")
			matches = append(matches, s.ID)
		}
	}

	if len(matches) == 0 {
		observe.GlobalTrace("if: len(matches) == 0")
		observe.GlobalTrace("return: Result{DisplayText: fmt.Sprintf(\"No session found matching %q\", args)}, nil")
		return Result{DisplayText: fmt.Sprintf("No session found matching %q", args)}, nil
	}
	if len(matches) > 1 {
		observe.GlobalTrace("if: len(matches) > 1")
		var b strings.Builder
		fmt.Fprintf(&b, "Ambiguous session ID %q — %d matches:\n", args, len(matches))
		for _, id := range matches {
			observe.GlobalTrace("range matches")
			fmt.Fprintf(&b, "  %s\n", id)
		}
		observe.GlobalTrace("return: Result{DisplayText: b.String()}, nil")
		return Result{DisplayText: b.String()}, nil
	}

	sess, err := deps.SessionStore.Load(matches[0])
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: Result{DisplayText: fmt.Sprintf(\"Error loading session: %s\", err)}, nil")
		return Result{DisplayText: fmt.Sprintf("Error loading session: %s", err)}, nil
	}

	summary := sess.Summary
	if summary == "" {
		observe.GlobalTrace("if: summary == \"\"")
		summary = "(no summary)"
	}
	dir := filepath.Base(sess.Conversation.WorkDir)
	ago := formatTimeAgo(sess.Conversation.UpdatedAt)

	shortID := sess.Conversation.ID
	if len(shortID) > 8 {
		observe.GlobalTrace("if: len(shortID) > 8")
		shortID = shortID[:8]
	}
	observe.GlobalTrace("return: Result{\n\tDisplayText: fmt.Sprintf(\"Resumed session %s\\n  %s — %s — %d tur...")

	return Result{
		DisplayText: fmt.Sprintf("Resumed session %s\n  %s — %s — %d turns — %s",
			shortID, summary, dir, sess.TurnCount, ago),
		ResumeSessionID: matches[0],
	}, nil
}

// formatTimeAgo returns a human-readable relative time string.
func formatTimeAgo(t time.Time) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	d := time.Since(t)
	switch {
	case d < time.Minute:
		observe.GlobalTrace("case: d < time.Minute")
		return "just now"
	case d < time.Hour:
		observe.GlobalTrace("case: d < time.Hour")
		m := int(d.Minutes())
		if m == 1 {
			observe.GlobalTrace("return: \"1 minute ago\"")
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		observe.GlobalTrace("case: d < 24*time.Hour")
		h := int(d.Hours())
		if h == 1 {
			observe.GlobalTrace("return: \"1 hour ago\"")
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		observe.GlobalTrace("default")
		days := int(d.Hours() / 24)
		if days == 1 {
			observe.GlobalTrace("return: \"1 day ago\"")
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}
