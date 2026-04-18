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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if d.currentQ >= len(d.questions) {
		observe.GlobalTrace("if: d.currentQ >= len(d.questions)")
		observe.GlobalTrace("return: 0")
		return 0
	}
	n := len(d.questions[d.currentQ].Options)
	if n == 0 {
		observe.GlobalTrace("if: n == 0")
		observe.GlobalTrace("return: 0")
		return 0
	}
	observe.GlobalTrace("return: n + 1")
	return n + 1
}

// isPlainText returns true if the current question has no options (free-text only).
func (d *askDialog) isPlainText() bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if d.currentQ >= len(d.questions) {
		observe.GlobalTrace("if: d.currentQ >= len(d.questions)")
		observe.GlobalTrace("return: true")
		return true
	}
	observe.GlobalTrace("return: len(d.questions[d.currentQ].Options) == 0")
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
		observe.GlobalTrace("if: len(msg.Request.Questions) > 0")
		d.questions = msg.Request.Questions
		d.inFreeText = len(msg.Request.Questions[0].Options) == 0
	} else {
		observe.GlobalTrace("else: len(msg.Request.Questions) > 0")

		d.questions = []tool.AskQuestion{{Question: msg.Request.Question}}
		d.inFreeText = true
	}
}

// submit sends the accumulated answers and deactivates the dialog.
func (d *askDialog) submit() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	resp := d.response
	answers := d.answers
	d.active = false
	d.response = nil
	if resp != nil {
		observe.GlobalTrace("if: resp != nil")
		resp <- tool.AskResponse{Answers: answers}
	}
}

// cancel sends an empty response and deactivates.
func (d *askDialog) cancel() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	resp := d.response
	d.active = false
	d.response = nil
	if resp != nil {
		observe.GlobalTrace("if: resp != nil")
		resp <- tool.AskResponse{Answers: map[string]string{}}
	}
}

// recordAnswer stores the answer for the current question.
func (d *askDialog) recordAnswer() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	q := d.questions[d.currentQ]
	opts := q.Options

	if len(opts) == 0 {
		observe.GlobalTrace("if: len(opts) == 0")

		d.answers[q.Question] = d.freeText.String()
	} else if d.selectedIdx < len(opts) {
		observe.GlobalTrace("else-if: d.selectedIdx < len(opts)")

		d.answers[q.Question] = opts[d.selectedIdx].Label
	} else {

		d.answers[q.Question] = d.freeText.String()
	}
}

// navigateTab moves to the next (+1) or previous (-1) question.
// Returns true if navigation occurred.
func (d *askDialog) navigateTab(dir int) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	next := d.currentQ + dir
	if next < 0 || next >= len(d.questions) || len(d.questions) <= 1 {
		observe.GlobalTrace("if: next < 0 || next >= len(d.questions) || len(d.questions) <= 1")
		observe.GlobalTrace("return: false")
		return false
	}
	d.recordAnswer()
	d.currentQ = next
	d.selectedIdx = 0
	d.freeText.Reset()
	d.inFreeText = len(d.questions[d.currentQ].Options) == 0
	observe.GlobalTrace("return: true")
	return true
}

// advanceOrSubmit moves to the next question or submits if on the last.
func (d *askDialog) advanceOrSubmit() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	d.recordAnswer()

	if d.currentQ+1 >= len(d.questions) {
		observe.GlobalTrace("if: d.currentQ+1 >= len(d.questions)")
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
		observe.GlobalTrace("if: !d.active")
		observe.GlobalTrace("return: nil")
		return nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: nil")
		return nil
	}

	if keyMsg.Type == tea.KeyEsc {
		observe.GlobalTrace("if: keyMsg.Type == tea.KeyEsc")
		d.cancel()
		observe.GlobalTrace("return: nil")
		return nil
	}

	if d.isPlainText() {
		observe.GlobalTrace("if: d.isPlainText()")
		observe.GlobalTrace("return: d.updateFreeText(keyMsg)")
		return d.updateFreeText(keyMsg)
	}

	if d.inFreeText {
		observe.GlobalTrace("if: d.inFreeText")
		observe.GlobalTrace("return: d.updateFreeTextOption(keyMsg)")
		return d.updateFreeTextOption(keyMsg)
	}
	observe.GlobalTrace("return: d.updateOptionNav(keyMsg)")
	return d.updateOptionNav(keyMsg)
}

// updateFreeText handles keys for plain free-text questions (no options).
func (d *askDialog) updateFreeText(keyMsg tea.KeyMsg) tea.Cmd {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch keyMsg.Type {
	case tea.KeyEnter:
		observe.GlobalTrace("case: tea.KeyEnter")
		d.advanceOrSubmit()
	case tea.KeyBackspace:
		observe.GlobalTrace("case: tea.KeyBackspace")
		s := d.freeText.String()
		if len(s) > 0 {
			d.freeText.Reset()
			d.freeText.WriteString(s[:len(s)-1])
		}
	case tea.KeyRunes:
		observe.GlobalTrace("case: tea.KeyRunes")
		d.freeText.WriteString(keyMsg.String())
	case tea.KeySpace:
		observe.GlobalTrace("case: tea.KeySpace")
		d.freeText.WriteString(" ")
	case tea.KeyTab:
		observe.GlobalTrace("case: tea.KeyTab")
		d.navigateTab(+1)
	case tea.KeyShiftTab:
		observe.GlobalTrace("case: tea.KeyShiftTab")
		d.navigateTab(-1)
	}
	observe.GlobalTrace("return: nil")
	return nil
}

// updateOptionNav handles keys when navigating the option list.
func (d *askDialog) updateOptionNav(keyMsg tea.KeyMsg) tea.Cmd {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	total := d.optionCount()

	switch keyMsg.Type {
	case tea.KeyUp:
		observe.GlobalTrace("case: tea.KeyUp")
		if d.selectedIdx > 0 {
			d.selectedIdx--
		}
	case tea.KeyDown:
		observe.GlobalTrace("case: tea.KeyDown")
		if d.selectedIdx < total-1 {
			d.selectedIdx++
		}
	case tea.KeyEnter:
		observe.GlobalTrace("case: tea.KeyEnter")
		opts := d.questions[d.currentQ].Options
		if d.selectedIdx >= len(opts) {

			d.inFreeText = true
			d.freeText.Reset()
		} else {
			d.advanceOrSubmit()
		}
	case tea.KeyTab:
		observe.GlobalTrace("case: tea.KeyTab")
		d.navigateTab(+1)
	case tea.KeyShiftTab:
		observe.GlobalTrace("case: tea.KeyShiftTab")
		d.navigateTab(-1)
	case tea.KeyRunes:
		observe.GlobalTrace("case: tea.KeyRunes")

		r := keyMsg.String()
		if len(r) == 1 && r[0] >= '1' && r[0] <= '9' {
			idx := int(r[0] - '1')
			if idx < total {
				observe.GlobalTrace("if: idx < total")
				d.selectedIdx = idx
				opts := d.questions[d.currentQ].Options
				if d.selectedIdx >= len(opts) {
					observe.GlobalTrace("if: d.selectedIdx >= len(opts)")
					d.inFreeText = true
					d.freeText.Reset()
				} else {
					observe.GlobalTrace("else: d.selectedIdx >= len(opts)")
					d.advanceOrSubmit()
				}
			}
		}
	}
	observe.GlobalTrace("return: nil")
	return nil
}

// updateFreeTextOption handles keys when typing in the "Other" free-text within an options question.
func (d *askDialog) updateFreeTextOption(keyMsg tea.KeyMsg) tea.Cmd {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch keyMsg.Type {
	case tea.KeyEnter:
		observe.GlobalTrace("case: tea.KeyEnter")
		d.advanceOrSubmit()
	case tea.KeyBackspace:
		observe.GlobalTrace("case: tea.KeyBackspace")
		s := d.freeText.String()
		if len(s) > 0 {
			d.freeText.Reset()
			d.freeText.WriteString(s[:len(s)-1])
		}
	case tea.KeyRunes:
		observe.GlobalTrace("case: tea.KeyRunes")
		d.freeText.WriteString(keyMsg.String())
	case tea.KeySpace:
		observe.GlobalTrace("case: tea.KeySpace")
		d.freeText.WriteString(" ")
	case tea.KeyUp:
		observe.GlobalTrace("case: tea.KeyUp")

		d.inFreeText = false
	case tea.KeyTab:
		observe.GlobalTrace("case: tea.KeyTab")
		d.navigateTab(+1)
	case tea.KeyShiftTab:
		observe.GlobalTrace("case: tea.KeyShiftTab")
		d.navigateTab(-1)
	}
	observe.GlobalTrace("return: nil")
	return nil
}

// View renders the ask dialog.
func (d *askDialog) View() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !d.active || len(d.questions) == 0 {
		observe.GlobalTrace("if: !d.active || len(d.questions) == 0")
		observe.GlobalTrace("return: \"\"")
		return ""
	}

	var b strings.Builder

	if len(d.questions) > 1 {
		observe.GlobalTrace("if: len(d.questions) > 1")
		b.WriteString(d.renderNavBar())
		b.WriteString("\n")
	}

	q := d.questions[d.currentQ]

	header := "? "
	if q.Header != "" {
		observe.GlobalTrace("if: q.Header != \"\"")
		header = "? [" + q.Header + "] "
	}
	b.WriteString(askQuestionStyle.Render(header + q.Question))
	b.WriteString("\n")

	if d.isPlainText() {
		observe.GlobalTrace("if: d.isPlainText()")

		b.WriteString(askInputStyle.Render("> " + d.freeText.String() + "█"))
	} else {
		observe.GlobalTrace("else: d.isPlainText()")

		b.WriteString("\n")
		for i, opt := range q.Options {
			observe.GlobalTrace("range q.Options")
			prefix := "  "
			if i == d.selectedIdx && !d.inFreeText {
				observe.GlobalTrace("if: i == d.selectedIdx && !d.inFreeText")
				prefix = "> "
			}
			num := fmt.Sprintf("%d. ", i+1)

			if i == d.selectedIdx && !d.inFreeText {
				observe.GlobalTrace("if: i == d.selectedIdx && !d.inFreeText")
				b.WriteString(askOptionSelected.Render(prefix + num + opt.Label))
			} else {
				observe.GlobalTrace("else: i == d.selectedIdx && !d.inFreeText")
				b.WriteString(prefix + num + opt.Label)
			}
			b.WriteString("\n")

			if opt.Description != "" {
				observe.GlobalTrace("if: opt.Description != \"\"")
				b.WriteString("     " + askDescStyle.Render(opt.Description))
				b.WriteString("\n")
			}
		}

		otherIdx := len(q.Options)
		otherPrefix := "  "
		if d.selectedIdx == otherIdx && !d.inFreeText {
			observe.GlobalTrace("if: d.selectedIdx == otherIdx && !d.inFreeText")
			otherPrefix = "> "
		}
		otherNum := fmt.Sprintf("%d. ", otherIdx+1)

		if d.inFreeText {
			observe.GlobalTrace("if: d.inFreeText")
			b.WriteString(askOptionSelected.Render("> " + otherNum + "Other: "))
			b.WriteString(askInputStyle.Render(d.freeText.String() + "█"))
		} else if d.selectedIdx == otherIdx {
			observe.GlobalTrace("else-if: d.selectedIdx == otherIdx")
			b.WriteString(askOptionSelected.Render(otherPrefix + otherNum + "Other (type your answer)"))
		} else {
			b.WriteString(otherPrefix + otherNum + "Other (type your answer)")
		}
		b.WriteString("\n")

		b.WriteString("\n")
		total := d.optionCount()
		hint := fmt.Sprintf("[1-%d] select  [↑↓] navigate  [Enter] confirm", total)
		if len(d.questions) > 1 {
			observe.GlobalTrace("if: len(d.questions) > 1")
			hint += "  [Tab] next"
		}
		b.WriteString(askHintStyle.Render(hint))
	}
	observe.GlobalTrace("return: b.String()")

	return b.String()
}

// renderNavBar renders the multi-question navigation tabs.
func (d *askDialog) renderNavBar() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var parts []string
	for i, q := range d.questions {
		observe.GlobalTrace("range d.questions")
		label := q.Header
		if label == "" {
			observe.GlobalTrace("if: label == \"\"")
			label = fmt.Sprintf("Q%d", i+1)
		}

		if _, answered := d.answers[q.Question]; answered && i != d.currentQ {
			observe.GlobalTrace("if: answered && i != d.currentQ")
			parts = append(parts, askNavDone.Render("["+label+" ✓]"))
		} else if i == d.currentQ {
			observe.GlobalTrace("else-if: i == d.currentQ")
			parts = append(parts, askNavActive.Render(" "+label+" "))
		} else {
			parts = append(parts, askHintStyle.Render("["+label+"]"))
		}
	}
	observe.GlobalTrace("return: \"  \" + strings.Join(parts, \" \")")
	return "  " + strings.Join(parts, " ")
}
