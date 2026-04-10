package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var askQuestionStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("12"))

var askInputStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("15"))

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
	d.active = true
	d.question = msg.Question
	d.response = msg.Response
	d.answer.Reset()
}

// Update handles key input while the dialog is active.
func (d *askDialog) Update(msg tea.Msg) tea.Cmd {
	if !d.active {
		return nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}

	switch keyMsg.Type {
	case tea.KeyEnter:
		answer := d.answer.String()
		resp := d.response
		d.active = false
		d.response = nil
		if resp != nil {
			resp <- answer
		}
		return nil
	case tea.KeyBackspace:
		s := d.answer.String()
		if len(s) > 0 {
			d.answer.Reset()
			d.answer.WriteString(s[:len(s)-1])
		}
	case tea.KeyRunes:
		d.answer.WriteString(keyMsg.String())
	case tea.KeySpace:
		d.answer.WriteString(" ")
	}
	return nil
}

// View renders the ask dialog.
func (d *askDialog) View() string {
	if !d.active {
		return ""
	}
	var b strings.Builder
	b.WriteString(askQuestionStyle.Render("? " + d.question))
	b.WriteString("\n")
	b.WriteString(askInputStyle.Render("> " + d.answer.String() + "█"))
	return b.String()
}
