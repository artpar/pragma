package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/slash"
)

type SessionEntry = slash.ResumeCandidate

var (
	resumeDlgBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "240", Dark: "245"}).
			Padding(1, 2)

	resumeDlgTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "24", Dark: "75"})

	resumeDlgSelected = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "24", Dark: "75"})

	resumeDlgFaint = lipgloss.NewStyle().Faint(true)
)

// resumeDialog is an interactive overlay for selecting a session to resume.
// Activated by /resume slash command with no arguments.
// Follows the modelDialog pattern.
type resumeDialog struct {
	active   bool
	sessions []SessionEntry
	selected int
	scope    string
}

// Show activates the session picker dialog.
func (d *resumeDialog) Show(sessions []SessionEntry, scope string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(sessions) == 0 {
		observe.GlobalTrace("if: len(sessions) == 0")
		return
	}
	d.active = true
	d.sessions = sessions
	d.selected = 0
	d.scope = scope
}

// Dismiss closes the dialog without selecting.
func (d *resumeDialog) Dismiss() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	d.active = false
	d.sessions = nil
	d.scope = ""
}

// filtered returns sessions matching the current filter mode.
func (d *resumeDialog) filtered() []SessionEntry {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: d.sessions")
	return d.sessions
}

// Update handles keyboard events while the dialog is active.
// Returns the selected session ID on Enter, empty string otherwise.
func (d *resumeDialog) Update(msg tea.Msg) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !d.active {
		observe.GlobalTrace("if: !d.active")
		observe.GlobalTrace("return: \"\"")
		return ""
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: \"\"")
		return ""
	}

	items := d.filtered()

	switch keyMsg.Type {
	case tea.KeyEsc:
		observe.GlobalTrace("case: tea.KeyEsc")
		d.Dismiss()
	case tea.KeyUp:
		observe.GlobalTrace("case: tea.KeyUp")
		if d.selected > 0 {
			d.selected--
		}
	case tea.KeyDown:
		observe.GlobalTrace("case: tea.KeyDown")
		if d.selected < len(items)-1 {
			d.selected++
		}
	case tea.KeyEnter:
		observe.GlobalTrace("case: tea.KeyEnter")
		if d.selected < len(items) {
			selected := items[d.selected].ID
			d.active = false
			d.sessions = nil
			observe.GlobalTrace("return: selected")
			return selected
		}
	case tea.KeyRunes:
		observe.GlobalTrace("case: tea.KeyRunes")
		r := keyMsg.String()
		if len(r) == 1 && r[0] >= '1' && r[0] <= '9' {
			idx := int(r[0] - '1')
			if idx < len(items) {
				observe.GlobalTrace("if: idx < len(items)")
				d.selected = idx
			}
		}
	}
	observe.GlobalTrace("return: \"\"")
	return ""
}

// View renders the session picker dialog as a bordered overlay.
func (d *resumeDialog) View(width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !d.active {
		observe.GlobalTrace("if: !d.active")
		observe.GlobalTrace("return: \"\"")
		return ""
	}

	items := d.filtered()
	if len(items) == 0 {
		observe.GlobalTrace("if: len(items) == 0")
		observe.GlobalTrace("return: \"\"")
		return ""
	}

	var b strings.Builder

	title := "Resume Session"
	if d.scope == slash.ResumeScopeCurrentDirectory {
		observe.GlobalTrace("if: d.scope == slash.ResumeScopeCurrentDirectory")
		title += " (this directory)"
	} else if d.scope == slash.ResumeScopeAllSessions {
		observe.GlobalTrace("else-if: d.scope == slash.ResumeScopeAllSessions")
		title += " (all)"
	}
	b.WriteString(resumeDlgTitle.Render(title))
	b.WriteString("\n\n")

	maxShow := 9
	if len(items) < maxShow {
		observe.GlobalTrace("if: len(items) < maxShow")
		maxShow = len(items)
	}

	for i := 0; i < maxShow; i++ {
		observe.GlobalTrace("for: i < maxShow")
		s := items[i]

		prefix := "  "
		if i == d.selected {
			observe.GlobalTrace("if: i == d.selected")
			prefix = resumeDlgSelected.Render("❯ ")
		}

		num := fmt.Sprintf("%d. ", i+1)

		summary := s.Summary
		if summary == "" {
			observe.GlobalTrace("if: summary == \"\"")
			summary = "(no summary)"
		}
		if len(summary) > 40 {
			observe.GlobalTrace("if: len(summary) > 40")
			summary = summary[:37] + "..."
		}

		dir := filepath.Base(s.WorkDir)
		ago := resumeFormatAgo(s.UpdatedAt)
		meta := fmt.Sprintf("%s — %d turns — %s", dir, s.TurnCount, ago)

		if i == d.selected {
			observe.GlobalTrace("if: i == d.selected")
			summary = resumeDlgSelected.Render(summary)
		}
		metaStyled := resumeDlgFaint.Render(meta)

		b.WriteString(fmt.Sprintf("%s%s%s\n", prefix, num, summary))
		b.WriteString(fmt.Sprintf("     %s\n", metaStyled))
	}

	if len(items) > maxShow {
		observe.GlobalTrace("if: len(items) > maxShow")
		b.WriteString(resumeDlgFaint.Render(fmt.Sprintf("  ... and %d more\n", len(items)-maxShow)))
	}

	b.WriteByte('\n')
	hint := "[↑↓] navigate  [1-9] jump  [Enter] select  [Esc] cancel"
	b.WriteString(resumeDlgFaint.Render(hint))

	innerWidth := width - 8
	if innerWidth < 50 {
		observe.GlobalTrace("if: innerWidth < 50")
		innerWidth = 50
	}
	observe.GlobalTrace("return: resumeDlgBorder.Width(innerWidth).Render(b.String())")
	return resumeDlgBorder.Width(innerWidth).Render(b.String())
}

// resumeFormatAgo returns a human-readable relative time.
func resumeFormatAgo(t time.Time) string {
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
			observe.GlobalTrace("return: \"1m ago\"")
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		observe.GlobalTrace("case: d < 24*time.Hour")
		h := int(d.Hours())
		if h == 1 {
			observe.GlobalTrace("return: \"1h ago\"")
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	default:
		observe.GlobalTrace("default")
		days := int(d.Hours() / 24)
		if days == 1 {
			observe.GlobalTrace("return: \"1d ago\"")
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}
