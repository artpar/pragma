package tui

import (
	"strings"

	"github.com/artpar/pragma/internal/observe"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var askQuestionStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.AdaptiveColor{Light: "24", Dark: "75"}) // blue

var askInputStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "235", Dark: "252"}) // near-white/near-black

// askDialog renders a question from a tool and captures the user's typed answer.
// Zero value is valid (inactive).
type askDialog struct {
	active   bool
	question string
	answer   strings.Builder
	response chan<- string
}

// Show activates the dialog with a question.
func (d *askDialog) Show(msg *AskRequestMsg) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	d.active = true
	d.question = msg.Question
	d.response = msg.Response
	d.answer.Reset()
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

	switch keyMsg.Type {
	case tea.KeyEnter:
		observe.GlobalTrace("case: tea.KeyEnter")
		answer := d.answer.String()
		resp := d.response
		d.active = false
		d.response = nil
		if resp != nil {
			resp <- answer
		}
		return nil
	case tea.KeyBackspace:
		observe.GlobalTrace("case: tea.KeyBackspace")
		s := d.answer.String()
		if len(s) > 0 {
			d.answer.Reset()
			d.answer.WriteString(s[:len(s)-1])
		}
	case tea.KeyRunes:
		observe.GlobalTrace("case: tea.KeyRunes")
		d.answer.WriteString(keyMsg.String())
	case tea.KeySpace:
		observe.GlobalTrace("case: tea.KeySpace")
		d.answer.WriteString(" ")
	}
	observe.GlobalTrace("return: nil")
	return nil
}

// View renders the ask dialog.
func (d *askDialog) View() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !d.active {
		observe.GlobalTrace("if: !d.active")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	var b strings.Builder
	b.WriteString(askQuestionStyle.Render("? " + d.question))
	b.WriteString("\n")
	b.WriteString(askInputStyle.Render("> " + d.answer.String() + "█"))
	observe.GlobalTrace("return: b.String()")
	return b.String()
}
