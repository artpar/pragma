package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artpar/pragma/internal/observe"
)

var (
	modelDlgBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "240", Dark: "245"}).
			Padding(1, 2)

	modelDlgTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "24", Dark: "75"})

	modelDlgSelected = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "24", Dark: "75"})

	modelDlgCurrent = lipgloss.NewStyle().
			Faint(true)

	modelDlgHint = lipgloss.NewStyle().Faint(true)
)

// modelDialog is an interactive overlay for selecting a model.
// Activated by /model or /models slash command with no arguments.
// Follows the teamsDialog/askDialog pattern.
type modelDialog struct {
	active   bool
	models   []string
	current  string
	selected int
}

// Show activates the model picker dialog.
// No-op if models is empty.
func (d *modelDialog) Show(models []string, current string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(models) == 0 {
		observe.GlobalTrace("if: len(models) == 0 — no models, skip")
		return
	}
	d.active = true
	d.models = models
	d.current = current
	d.selected = 0

	for i, m := range models {
		observe.GlobalTrace("range models")
		if m == current {
			observe.GlobalTrace("if: m == current")
			d.selected = i
			break
		}
	}
}

// Dismiss closes the dialog without selecting.
func (d *modelDialog) Dismiss() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	d.active = false
	d.models = nil
}

// Update handles keyboard events while the dialog is active.
// Returns the selected model name on Enter, empty string otherwise.
func (d *modelDialog) Update(msg tea.Msg) string {
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
		if d.selected < len(d.models)-1 {
			d.selected++
		}
	case tea.KeyEnter:
		observe.GlobalTrace("case: tea.KeyEnter")
		if d.selected < len(d.models) {
			selected := d.models[d.selected]
			d.active = false
			d.models = nil
			observe.GlobalTrace("return: selected")
			return selected
		}
	case tea.KeyRunes:
		observe.GlobalTrace("case: tea.KeyRunes")

		r := keyMsg.String()
		if len(r) == 1 && r[0] >= '1' && r[0] <= '9' {
			idx := int(r[0] - '1')
			if idx < len(d.models) {
				observe.GlobalTrace("if: idx < len(d.models)")
				d.selected = idx
			}
		}
	}
	observe.GlobalTrace("return: \"\"")
	return ""
}

// View renders the model picker dialog as a bordered overlay.
func (d *modelDialog) View(width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !d.active || len(d.models) == 0 {
		observe.GlobalTrace("if: !d.active || len(d.models) == 0")
		observe.GlobalTrace("return: \"\"")
		return ""
	}

	var b strings.Builder
	b.WriteString(modelDlgTitle.Render("Select Model"))
	b.WriteString("\n\n")

	for i, m := range d.models {
		observe.GlobalTrace("range d.models")
		prefix := "  "
		if i == d.selected {
			observe.GlobalTrace("if: i == d.selected")
			prefix = modelDlgSelected.Render("❯ ")
		}
		num := fmt.Sprintf("%d. ", i+1)
		name := m
		if i == d.selected {
			observe.GlobalTrace("if: i == d.selected")
			name = modelDlgSelected.Render(name)
		}
		suffix := ""
		if m == d.current {
			observe.GlobalTrace("if: m == d.current")
			suffix = modelDlgCurrent.Render(" (current)")
		}
		b.WriteString(fmt.Sprintf("%s%s%s%s\n", prefix, num, name, suffix))
	}

	b.WriteByte('\n')
	maxNum := len(d.models)
	if maxNum > 9 {
		observe.GlobalTrace("if: maxNum > 9")
		maxNum = 9
	}
	hint := fmt.Sprintf("[↑↓] navigate  [1-%d] jump  [Enter] select  [Esc] cancel", maxNum)
	b.WriteString(modelDlgHint.Render(hint))

	innerWidth := width - 8
	if innerWidth < 40 {
		observe.GlobalTrace("if: innerWidth < 40")
		innerWidth = 40
	}
	observe.GlobalTrace("return: modelDlgBorder.Width(innerWidth).Render(b.String())")
	return modelDlgBorder.Width(innerWidth).Render(b.String())
}
