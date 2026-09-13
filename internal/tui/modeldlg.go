package tui

import (
	"fmt"
	"strconv"
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

// Windowed rendering bounds. The catalog lists qualified "provider/model"
// IDs for every provider with credentials, each contributing its live
// /models endpoint — easily hundreds of entries. Rendering them all at
// once produced a dialog taller than the terminal (the selection scrolled
// off-screen) and re-rendering hundreds of styled rows on every
// keystroke made the picker sluggish. The picker therefore renders a
// bounded, scrolling window around the selection instead.
const (
	// modelDlgChromeRows is the render budget for everything that is not
	// a list row: border (2), padding (2), title + blank (2), position
	// + blank + hint footer (3), up to two scroll indicators (2), the
	// blank separator line the dialog is preceded by (1), and a line of
	// margin so the window never needs to scroll (1).
	modelDlgChromeRows = 13
	// Rows shown when the terminal height is unknown.
	modelDlgDefaultRows = 12
	// Upper bound on rendered rows regardless of terminal size, so a
	// very tall terminal still gets a snappy, fzf-like list.
	modelDlgMaxRows = 20
)

// modelDialog is an interactive overlay for selecting a model.
// Activated by /model or /models slash command with no arguments.
type modelDialog struct {
	active   bool
	models   []string
	current  string
	selected int
	filter   string
	vis      []string // cached filter results; rebuilt only when models/filter change
	offset   int      // index within vis of the first rendered row
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
	d.filter = ""
	d.refresh()

	for i, m := range models {
		observe.GlobalTrace("range models")
		if m == current {
			observe.GlobalTrace("if: m == current")
			d.selected = i
			break
		}
	}

	d.offset = d.selected - modelDlgDefaultRows/2
	if d.offset < 0 {
		observe.GlobalTrace("if: d.offset < 0")
		d.offset = 0
	}
}

// Dismiss closes the dialog without selecting.
func (d *modelDialog) Dismiss() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	d.active = false
	d.models = nil
	d.vis = nil
	d.filter = ""
	d.offset = 0
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
		if d.filter != "" {
			observe.GlobalTrace("if: d.filter != \"\" — clear filter first")
			d.filter = ""
			d.refresh()
		} else {
			observe.GlobalTrace("else: dismiss")
			d.Dismiss()
		}
	case tea.KeyBackspace:
		observe.GlobalTrace("case: tea.KeyBackspace")
		if d.filter != "" {
			observe.GlobalTrace("if: d.filter != \"\"")
			d.filter = d.filter[:len(d.filter)-1]
			d.refresh()
		}
	case tea.KeyUp:
		observe.GlobalTrace("case: tea.KeyUp")
		if d.selected > 0 {
			observe.GlobalTrace("if: d.selected > 0")
			d.selected--
		}
	case tea.KeyDown:
		observe.GlobalTrace("case: tea.KeyDown")
		if d.selected < len(d.vis)-1 {
			observe.GlobalTrace("if: d.selected < len(d.vis)-1")
			d.selected++
		}
	case tea.KeyEnter:
		observe.GlobalTrace("case: tea.KeyEnter")
		if d.selected < len(d.vis) {
			selected := d.vis[d.selected]
			d.active = false
			d.models = nil
			d.vis = nil
			d.filter = ""
			observe.GlobalTrace("return: selected")
			return selected
		}
	case tea.KeySpace:
		observe.GlobalTrace("case: tea.KeySpace")
		d.filter += " "
		d.refresh()
	case tea.KeyRunes:
		observe.GlobalTrace("case: tea.KeyRunes")

		if keyMsg.Alt {
			observe.GlobalTrace("if: keyMsg.Alt — shortcut, not filter text")
			observe.GlobalTrace("return: \"\"")
			return ""
		}

		runes := keyMsg.Runes
		if d.filter == "" && len(runes) == 1 && runes[0] >= '1' && runes[0] <= '9' {
			observe.GlobalTrace("if: digit jump while filter is empty")
			idx := int(runes[0] - '1')
			if idx < len(d.vis) {
				observe.GlobalTrace("if: idx < len(d.vis)")
				d.selected = idx
			}
			observe.GlobalTrace("return: \"\"")
			return ""
		}
		if len(runes) > 0 {
			observe.GlobalTrace("if: printable runes — extend filter")
			d.filter += string(runes)
			d.refresh()
		}
	}
	observe.GlobalTrace("return: \"\"")
	return ""
}

// visible returns the cached models matching the current filter. With no
// filter, all models are visible. The cache keeps per-keystroke cost to a
// single filter pass instead of one per call site.
func (d *modelDialog) visible() []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: d.vis")
	return d.vis
}

// refresh rebuilds the cached filtered list after the model list or the
// filter changed, then re-anchors the selection. Called only on those
// changes — not per keystroke, not per render.
func (d *modelDialog) refresh() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	d.vis = filterModels(d.models, d.filter)
	if d.selected >= len(d.vis) {
		observe.GlobalTrace("if: d.selected >= len(d.vis)")
		d.selected = max(len(d.vis)-1, 0)
	}
	if d.selected < 0 {
		observe.GlobalTrace("if: d.selected < 0")
		d.selected = 0
	}

	d.offset = 0
}

// scrollIntoView adjusts the window offset so the selected row is inside
// the rendered window, moving the window as little as possible.
func (d *modelDialog) scrollIntoView(rows int) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if rows < 1 {
		observe.GlobalTrace("if: rows < 1")
		rows = 1
	}
	if d.selected < d.offset {
		observe.GlobalTrace("if: d.selected < d.offset")
		d.offset = d.selected
	}
	if last := d.offset + rows - 1; d.selected > last {
		observe.GlobalTrace("if: d.selected > d.offset+rows-1")
		d.offset = d.selected - rows + 1
	}
	if maxOff := max(len(d.vis)-rows, 0); d.offset > maxOff {
		observe.GlobalTrace("if: d.offset > maxOff")
		d.offset = maxOff
	}
	if d.offset < 0 {
		observe.GlobalTrace("if: d.offset < 0")
		d.offset = 0
	}
}

// filterModels returns the models matching filter (case-insensitive
// substring). An empty filter returns the input unchanged.
func filterModels(models []string, filter string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if filter == "" {
		observe.GlobalTrace("return: models")
		return models
	}
	needle := strings.ToLower(filter)
	out := make([]string, 0, len(models))
	for _, m := range models {
		observe.GlobalTrace("range models")
		if strings.Contains(strings.ToLower(m), needle) {
			observe.GlobalTrace("if: strings.Contains(strings.ToLower(m), needle)")
			out = append(out, m)
		}
	}
	observe.GlobalTrace("return: out")
	return out
}

// listRowsFor returns how many list rows the dialog may render for the
// given terminal height: enough to fill the screen without overflow, at
// least a usable minimum, and never more than modelDlgMaxRows so huge
// terminals still get a cheap, navigable list.
func listRowsFor(height int) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if height <= 0 {
		observe.GlobalTrace("if: height <= 0")
		observe.GlobalTrace("return: modelDlgDefaultRows")
		return modelDlgDefaultRows
	}
	rows := height - modelDlgChromeRows
	if rows < 3 {
		observe.GlobalTrace("if: rows < 3")
		rows = 3
	}
	if rows > modelDlgMaxRows {
		observe.GlobalTrace("if: rows > modelDlgMaxRows")
		rows = modelDlgMaxRows
	}
	observe.GlobalTrace("return: rows")
	return rows
}

// View renders the model picker dialog as a bordered overlay. Only a
// scrolling window of the filtered list is rendered, bounded by the
// terminal height, so catalogs with hundreds of models stay navigable
// and cheap to re-render.
func (d *modelDialog) View(width, height int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !d.active || len(d.models) == 0 {
		observe.GlobalTrace("if: !d.active || len(d.models) == 0")
		observe.GlobalTrace("return: \"\"")
		return ""
	}

	rows := listRowsFor(height)

	d.scrollIntoView(rows)

	var b strings.Builder
	b.WriteString(modelDlgTitle.Render("Select Model"))
	b.WriteString("\n\n")

	if len(d.vis) == 0 {
		observe.GlobalTrace("if: len(d.vis) == 0")
		b.WriteString("(no matching models)\n\n")
	} else {
		observe.GlobalTrace("else: len(d.vis) == 0")
		if d.offset > 0 {
			observe.GlobalTrace("if: d.offset > 0")
			b.WriteString(modelDlgCurrent.Render("  ↑ more above"))
			b.WriteByte('\n')
		}
		end := min(d.offset+rows, len(d.vis))

		numWidth := len(strconv.Itoa(len(d.vis)))
		for i := d.offset; i < end; i++ {
			observe.GlobalTrace("range rendered window")
			m := d.vis[i]
			prefix := "  "
			if i == d.selected {
				observe.GlobalTrace("if: i == d.selected")
				prefix = modelDlgSelected.Render("❯ ")
			}
			num := fmt.Sprintf("%*d. ", numWidth, i+1)
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
			b.WriteString(prefix)
			b.WriteString(num)
			b.WriteString(name)
			b.WriteString(suffix)
			b.WriteByte('\n')
		}
		if end < len(d.vis) {
			observe.GlobalTrace("if: end < len(d.vis)")
			b.WriteString(modelDlgCurrent.Render("  ↓ more below"))
			b.WriteByte('\n')
		}

		b.WriteString(fmt.Sprintf("\n  showing %d–%d of %d", d.offset+1, end, len(d.vis)))
	}

	b.WriteString("\n\n")
	jump := ""
	if maxNum := min(len(d.vis), 9); maxNum > 0 {
		observe.GlobalTrace("if: maxNum > 0")
		jump = fmt.Sprintf("  [1-%d] jump", maxNum)
	}
	hint := "[↑↓] move" + jump + "  [type] filter  [Enter] select  [Esc] cancel"
	if d.filter != "" {
		observe.GlobalTrace("if: d.filter != \"\"")
		hint = fmt.Sprintf("filter: %q  %d match(es)  [Backspace] edit  [Esc] clear", d.filter, len(d.vis))
	}
	b.WriteString(modelDlgHint.Render(hint))

	innerWidth := width - 8
	if innerWidth < 40 {
		observe.GlobalTrace("if: innerWidth < 40")
		innerWidth = 40
	}
	observe.GlobalTrace("return: modelDlgBorder.Width(innerWidth).Render(b.String())")
	return modelDlgBorder.Width(innerWidth).Render(b.String())
}
