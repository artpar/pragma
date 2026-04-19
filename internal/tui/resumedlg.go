package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artpar/pragma/internal/observe"
)

// SessionEntry is a lightweight session summary for the resume dialog.
// Decoupled from session.SessionSummary to avoid DAG violation (tui/ cannot import session/).
type SessionEntry struct {
	ID        string
	Summary   string
	WorkDir   string
	TurnCount int
	UpdatedAt time.Time
}

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
	active    bool
	sessions  []SessionEntry
	selected  int
	showAll   bool   // false = filter to cwd, true = all sessions
	cwd       string // current working directory for filtering
}

// Show activates the session picker dialog.
func (d *resumeDialog) Show(sessions []SessionEntry, cwd string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(sessions) == 0 {
		return
	}
	d.active = true
	d.sessions = sessions
	d.selected = 0
	d.showAll = false
	d.cwd = cwd
}

// Dismiss closes the dialog without selecting.
func (d *resumeDialog) Dismiss() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	d.active = false
	d.sessions = nil
}

// filtered returns sessions matching the current filter mode.
func (d *resumeDialog) filtered() []SessionEntry {
	if d.showAll {
		return d.sessions
	}
	var result []SessionEntry
	for _, s := range d.sessions {
		if s.WorkDir == d.cwd {
			result = append(result, s)
		}
	}
	// Fall back to all sessions if none match current directory
	if len(result) == 0 {
		return d.sessions
	}
	return result
}

// Update handles keyboard events while the dialog is active.
// Returns the selected session ID on Enter, empty string otherwise.
func (d *resumeDialog) Update(msg tea.Msg) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !d.active {
		return ""
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return ""
	}

	items := d.filtered()

	switch keyMsg.Type {
	case tea.KeyEsc:
		d.Dismiss()
	case tea.KeyUp:
		if d.selected > 0 {
			d.selected--
		}
	case tea.KeyDown:
		if d.selected < len(items)-1 {
			d.selected++
		}
	case tea.KeyTab:
		d.showAll = !d.showAll
		d.selected = 0
	case tea.KeyEnter:
		if d.selected < len(items) {
			selected := items[d.selected].ID
			d.active = false
			d.sessions = nil
			return selected
		}
	case tea.KeyRunes:
		r := keyMsg.String()
		if len(r) == 1 && r[0] >= '1' && r[0] <= '9' {
			idx := int(r[0] - '1')
			if idx < len(items) {
				d.selected = idx
			}
		}
	}
	return ""
}

// View renders the session picker dialog as a bordered overlay.
func (d *resumeDialog) View(width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !d.active {
		return ""
	}

	items := d.filtered()
	if len(items) == 0 {
		return ""
	}

	var b strings.Builder

	// Title with filter indicator
	title := "Resume Session"
	if d.showAll {
		title += " (all)"
	} else {
		title += " (this directory)"
	}
	b.WriteString(resumeDlgTitle.Render(title))
	b.WriteString("\n\n")

	// Show up to 9 items
	maxShow := 9
	if len(items) < maxShow {
		maxShow = len(items)
	}

	for i := 0; i < maxShow; i++ {
		s := items[i]

		prefix := "  "
		if i == d.selected {
			prefix = resumeDlgSelected.Render("❯ ")
		}

		num := fmt.Sprintf("%d. ", i+1)

		// Summary (truncated)
		summary := s.Summary
		if summary == "" {
			summary = "(no summary)"
		}
		if len(summary) > 40 {
			summary = summary[:37] + "..."
		}

		// Metadata
		dir := filepath.Base(s.WorkDir)
		ago := resumeFormatAgo(s.UpdatedAt)
		meta := fmt.Sprintf("%s — %d turns — %s", dir, s.TurnCount, ago)

		if i == d.selected {
			summary = resumeDlgSelected.Render(summary)
		}
		metaStyled := resumeDlgFaint.Render(meta)

		b.WriteString(fmt.Sprintf("%s%s%s\n", prefix, num, summary))
		b.WriteString(fmt.Sprintf("     %s\n", metaStyled))
	}

	if len(items) > maxShow {
		b.WriteString(resumeDlgFaint.Render(fmt.Sprintf("  ... and %d more\n", len(items)-maxShow)))
	}

	b.WriteByte('\n')
	hint := "[↑↓] navigate  [1-9] jump  [Enter] select  [Tab] toggle filter  [Esc] cancel"
	b.WriteString(resumeDlgFaint.Render(hint))

	innerWidth := width - 8
	if innerWidth < 50 {
		innerWidth = 50
	}
	return resumeDlgBorder.Width(innerWidth).Render(b.String())
}

// resumeFormatAgo returns a human-readable relative time.
func resumeFormatAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}
