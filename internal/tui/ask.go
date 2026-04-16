package tui

import (
	"fmt"
	"strings"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/tool"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	askQuestionStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "24", Dark: "75"})

	askInputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "235", Dark: "252"})

	askOptionSelected = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "24", Dark: "75"})

	askDescStyle = lipgloss.NewStyle().Faint(true)

	askHintStyle = lipgloss.NewStyle().Faint(true)

	askNavActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "255", Dark: "232"}).
			Background(lipgloss.AdaptiveColor{Light: "24", Dark: "75"})

	askNavDone = lipgloss.NewStyle().Faint(true)
)

// askDialog renders a question from a tool and captures the user's answer.
// Supports both legacy plain-text questions and structured multi-choice questions.
type askDialog struct {
	active    bool
	questions []tool.AskQuestion
	response  chan<- tool.AskResponse

	currentQ    int               // active question index
	answers     map[string]string // accumulated answers
	selectedIdx int               // highlighted option (last = "Other" when options exist)
	freeText    strings.Builder   // typed text for "Other" or plain free-text
	inFreeText  bool              // editing free-text
}

// optionCount returns the total selectable positions for the current question.
// When options exist, the last position is the virtual "Other" free-text option.
func (d *askDialog) optionCount() int {
	if d.currentQ >= len(d.questions) {
		return 0
	}
	n := len(d.questions[d.currentQ].Options)
	if n == 0 {
		return 0
	}
	return n + 1 // +1 for "Other"
}

// isPlainText returns true if the current question has no options (free-text only).
func (d *askDialog) isPlainText() bool {
	if d.currentQ >= len(d.questions) {
		return true
	}
	return len(d.questions[d.currentQ].Options) == 0
}

// Show activates the dialog with a question request.
func (d *askDialog) Show(msg *AskRequestMsg) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	d.active = true
	d.response = msg.Response
	d.answers = make(map[string]string)
	d.currentQ = 0
	d.selectedIdx = 0
	d.freeText.Reset()

	if len(msg.Request.Questions) > 0 {
		d.questions = msg.Request.Questions
		d.inFreeText = len(msg.Request.Questions[0].Options) == 0
	} else {
		// Legacy plain-text: synthesize a single free-text question
		d.questions = []tool.AskQuestion{{Question: msg.Request.Question}}
		d.inFreeText = true
	}
}

// submit sends the accumulated answers and deactivates the dialog.
func (d *askDialog) submit() {
	resp := d.response
	answers := d.answers
	d.active = false
	d.response = nil
	if resp != nil {
		resp <- tool.AskResponse{Answers: answers}
	}
}

// cancel sends an empty response and deactivates.
func (d *askDialog) cancel() {
	resp := d.response
	d.active = false
	d.response = nil
	if resp != nil {
		resp <- tool.AskResponse{Answers: map[string]string{}}
	}
}

// recordAnswer stores the answer for the current question.
func (d *askDialog) recordAnswer() {
	q := d.questions[d.currentQ]
	opts := q.Options

	if len(opts) == 0 {
		// Plain free-text
		d.answers[q.Question] = d.freeText.String()
	} else if d.selectedIdx < len(opts) {
		// Concrete option
		d.answers[q.Question] = opts[d.selectedIdx].Label
	} else {
		// "Other" free-text
		d.answers[q.Question] = d.freeText.String()
	}
}

// navigateTab moves to the next (+1) or previous (-1) question.
// Returns true if navigation occurred.
func (d *askDialog) navigateTab(dir int) bool {
	next := d.currentQ + dir
	if next < 0 || next >= len(d.questions) || len(d.questions) <= 1 {
		return false
	}
	d.recordAnswer()
	d.currentQ = next
	d.selectedIdx = 0
	d.freeText.Reset()
	d.inFreeText = len(d.questions[d.currentQ].Options) == 0
	return true
}

// advanceOrSubmit moves to the next question or submits if on the last.
func (d *askDialog) advanceOrSubmit() {
	d.recordAnswer()

	if d.currentQ+1 >= len(d.questions) {
		d.submit()
		return
	}

	d.currentQ++
	d.selectedIdx = 0
	d.freeText.Reset()
	d.inFreeText = len(d.questions[d.currentQ].Options) == 0
}

// Update handles key input while the dialog is active.
func (d *askDialog) Update(msg tea.Msg) tea.Cmd {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !d.active {
		return nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}

	// Esc always cancels
	if keyMsg.Type == tea.KeyEsc {
		d.cancel()
		return nil
	}

	// Plain free-text mode: simple text editing
	if d.isPlainText() {
		return d.updateFreeText(keyMsg)
	}

	// Option selection mode
	if d.inFreeText {
		return d.updateFreeTextOption(keyMsg)
	}
	return d.updateOptionNav(keyMsg)
}

// updateFreeText handles keys for plain free-text questions (no options).
func (d *askDialog) updateFreeText(keyMsg tea.KeyMsg) tea.Cmd {
	switch keyMsg.Type {
	case tea.KeyEnter:
		d.advanceOrSubmit()
	case tea.KeyBackspace:
		s := d.freeText.String()
		if len(s) > 0 {
			d.freeText.Reset()
			d.freeText.WriteString(s[:len(s)-1])
		}
	case tea.KeyRunes:
		d.freeText.WriteString(keyMsg.String())
	case tea.KeySpace:
		d.freeText.WriteString(" ")
	case tea.KeyTab:
		d.navigateTab(+1)
	case tea.KeyShiftTab:
		d.navigateTab(-1)
	}
	return nil
}

// updateOptionNav handles keys when navigating the option list.
func (d *askDialog) updateOptionNav(keyMsg tea.KeyMsg) tea.Cmd {
	total := d.optionCount()

	switch keyMsg.Type {
	case tea.KeyUp:
		if d.selectedIdx > 0 {
			d.selectedIdx--
		}
	case tea.KeyDown:
		if d.selectedIdx < total-1 {
			d.selectedIdx++
		}
	case tea.KeyEnter:
		opts := d.questions[d.currentQ].Options
		if d.selectedIdx >= len(opts) {
			// "Other" selected — enter free-text mode
			d.inFreeText = true
			d.freeText.Reset()
		} else {
			d.advanceOrSubmit()
		}
	case tea.KeyTab:
		d.navigateTab(+1)
	case tea.KeyShiftTab:
		d.navigateTab(-1)
	case tea.KeyRunes:
		// Number quick-select (1-9)
		r := keyMsg.String()
		if len(r) == 1 && r[0] >= '1' && r[0] <= '9' {
			idx := int(r[0]-'1')
			if idx < total {
				d.selectedIdx = idx
				opts := d.questions[d.currentQ].Options
				if d.selectedIdx >= len(opts) {
					d.inFreeText = true
					d.freeText.Reset()
				} else {
					d.advanceOrSubmit()
				}
			}
		}
	}
	return nil
}

// updateFreeTextOption handles keys when typing in the "Other" free-text within an options question.
func (d *askDialog) updateFreeTextOption(keyMsg tea.KeyMsg) tea.Cmd {
	switch keyMsg.Type {
	case tea.KeyEnter:
		d.advanceOrSubmit()
	case tea.KeyBackspace:
		s := d.freeText.String()
		if len(s) > 0 {
			d.freeText.Reset()
			d.freeText.WriteString(s[:len(s)-1])
		}
	case tea.KeyRunes:
		d.freeText.WriteString(keyMsg.String())
	case tea.KeySpace:
		d.freeText.WriteString(" ")
	case tea.KeyUp:
		// Exit free-text, go back to option nav
		d.inFreeText = false
	case tea.KeyTab:
		d.navigateTab(+1)
	case tea.KeyShiftTab:
		d.navigateTab(-1)
	}
	return nil
}

// View renders the ask dialog.
func (d *askDialog) View() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !d.active || len(d.questions) == 0 {
		return ""
	}

	var b strings.Builder

	// Multi-question navigation bar
	if len(d.questions) > 1 {
		b.WriteString(d.renderNavBar())
		b.WriteString("\n")
	}

	q := d.questions[d.currentQ]

	// Question header
	header := "? "
	if q.Header != "" {
		header = "? [" + q.Header + "] "
	}
	b.WriteString(askQuestionStyle.Render(header + q.Question))
	b.WriteString("\n")

	if d.isPlainText() {
		// Pure free-text
		b.WriteString(askInputStyle.Render("> " + d.freeText.String() + "█"))
	} else {
		// Option list
		b.WriteString("\n")
		for i, opt := range q.Options {
			prefix := "  "
			if i == d.selectedIdx && !d.inFreeText {
				prefix = "> "
			}
			num := fmt.Sprintf("%d. ", i+1)

			if i == d.selectedIdx && !d.inFreeText {
				b.WriteString(askOptionSelected.Render(prefix + num + opt.Label))
			} else {
				b.WriteString(prefix + num + opt.Label)
			}
			b.WriteString("\n")

			if opt.Description != "" {
				b.WriteString("     " + askDescStyle.Render(opt.Description))
				b.WriteString("\n")
			}
		}

		// "Other" option
		otherIdx := len(q.Options)
		otherPrefix := "  "
		if d.selectedIdx == otherIdx && !d.inFreeText {
			otherPrefix = "> "
		}
		otherNum := fmt.Sprintf("%d. ", otherIdx+1)

		if d.inFreeText {
			b.WriteString(askOptionSelected.Render("> " + otherNum + "Other: "))
			b.WriteString(askInputStyle.Render(d.freeText.String() + "█"))
		} else if d.selectedIdx == otherIdx {
			b.WriteString(askOptionSelected.Render(otherPrefix + otherNum + "Other (type your answer)"))
		} else {
			b.WriteString(otherPrefix + otherNum + "Other (type your answer)")
		}
		b.WriteString("\n")

		// Hint bar
		b.WriteString("\n")
		total := d.optionCount()
		hint := fmt.Sprintf("[1-%d] select  [↑↓] navigate  [Enter] confirm", total)
		if len(d.questions) > 1 {
			hint += "  [Tab] next"
		}
		b.WriteString(askHintStyle.Render(hint))
	}

	return b.String()
}

// renderNavBar renders the multi-question navigation tabs.
func (d *askDialog) renderNavBar() string {
	var parts []string
	for i, q := range d.questions {
		label := q.Header
		if label == "" {
			label = fmt.Sprintf("Q%d", i+1)
		}

		if _, answered := d.answers[q.Question]; answered && i != d.currentQ {
			parts = append(parts, askNavDone.Render("["+label+" ✓]"))
		} else if i == d.currentQ {
			parts = append(parts, askNavActive.Render(" "+label+" "))
		} else {
			parts = append(parts, askHintStyle.Render("["+label+"]"))
		}
	}
	return "  " + strings.Join(parts, " ")
}
