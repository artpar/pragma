package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/task"
	"github.com/artpar/pragma/internal/tui/render"
)

var (
	teamsBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "240", Dark: "245"}).
			Padding(1, 2)

	teamsTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "24", Dark: "75"})

	teamsSelected = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "24", Dark: "75"})

	teamsHint = lipgloss.NewStyle().Faint(true)

	teamsFeedback = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "28", Dark: "120"})
)

// teamsDialog is an interactive overlay for managing running teammates.
// Activated by /teams slash command. Follows permissionDialog/askDialog pattern.
type teamsDialog struct {
	active   bool
	entries  []render.TeammateEntry
	selected int
	taskReg  *task.Registry
	feedback string // transient feedback message ("Shutdown requested", etc.)
}

// Show activates the teams dialog with current teammate entries.
func (d *teamsDialog) Show(entries []render.TeammateEntry, reg *task.Registry) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	d.active = true
	d.entries = entries
	d.selected = 0
	d.taskReg = reg
	d.feedback = ""
}

// Dismiss closes the dialog.
func (d *teamsDialog) Dismiss() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	d.active = false
	d.entries = nil
	d.feedback = ""
}

// Refresh updates entries from the task registry.
func (d *teamsDialog) Refresh() {
	if d.taskReg == nil {
		return
	}
	teammates := d.taskReg.ListRunningTeammates()
	d.entries = buildTeammateEntries(teammates)
	if d.selected >= len(d.entries) {
		d.selected = max(0, len(d.entries)-1)
	}
}

// Update handles keyboard events. Returns a tea.Cmd if the dialog should close.
func (d *teamsDialog) Update(msg tea.Msg) tea.Cmd {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !d.active {
		return nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}

	switch keyMsg.String() {
	case "esc":
		d.Dismiss()
		return nil
	case "up", "k":
		if len(d.entries) > 0 {
			d.selected--
			if d.selected < 0 {
				d.selected = len(d.entries) - 1
			}
			d.feedback = ""
		}
	case "down", "j":
		if len(d.entries) > 0 {
			d.selected++
			if d.selected >= len(d.entries) {
				d.selected = 0
			}
			d.feedback = ""
		}
	case "s":
		d.shutdownSelected()
	case "K":
		d.killSelected()
	}
	return nil
}

// shutdownSelected requests graceful shutdown for the selected teammate.
func (d *teamsDialog) shutdownSelected() {
	if d.selected >= len(d.entries) || d.taskReg == nil {
		return
	}
	e := d.entries[d.selected]
	_ = d.taskReg.Update(e.TaskID, func(tt *task.Task) {
		tt.ShutdownRequested = true
	})
	d.taskReg.NotifyTask(e.TaskID)
	d.feedback = fmt.Sprintf("Shutdown requested for %s", e.Name)
	d.Refresh()
}

// killSelected force-cancels the selected teammate.
func (d *teamsDialog) killSelected() {
	if d.selected >= len(d.entries) || d.taskReg == nil {
		return
	}
	e := d.entries[d.selected]
	d.taskReg.Cancel(e.TaskID)
	d.feedback = fmt.Sprintf("Killed %s", e.Name)
	d.Refresh()
}

// View renders the teams dialog as a bordered overlay.
func (d *teamsDialog) View(width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !d.active {
		return ""
	}

	var b strings.Builder
	b.WriteString(teamsTitle.Render("Teams"))
	b.WriteString("\n\n")

	if len(d.entries) == 0 {
		b.WriteString("No active teammates.\n")
	} else {
		for i, e := range d.entries {
			prefix := "  "
			if i == d.selected {
				prefix = teamsSelected.Render("❯ ")
			}
			name := e.Name
			if i == d.selected {
				name = teamsSelected.Render(name)
			}
			status := render.TeammateStatusText(e)
			line := fmt.Sprintf("%s%s (%s) · %s", prefix, name, e.TaskID, status)
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}

	if d.feedback != "" {
		b.WriteByte('\n')
		b.WriteString(teamsFeedback.Render(d.feedback))
		b.WriteByte('\n')
	}

	b.WriteByte('\n')
	b.WriteString(teamsHint.Render("[↑↓] navigate  [s] shutdown  [K] kill  [Esc] close"))

	innerWidth := width - 8
	if innerWidth < 40 {
		innerWidth = 40
	}
	return teamsBorder.Width(innerWidth).Render(b.String())
}

// buildTeammateEntries converts task snapshots to TeammateEntry slice.
func buildTeammateEntries(tasks []task.Task) []render.TeammateEntry {
	entries := make([]render.TeammateEntry, len(tasks))
	for i, t := range tasks {
		entries[i] = render.TeammateEntry{
			Name:              t.AgentName,
			TaskID:            t.ID,
			TokenCount:        t.TokensUsed,
			IsIdle:            t.IsIdle,
			IdleSince:         t.IdleSince,
			ShutdownRequested: t.ShutdownRequested,
		}
	}
	return entries
}
