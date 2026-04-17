package tui

import (
	"github.com/artpar/pragma/internal/observe"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

// inputComponent wraps a textarea for user message input.
// The input is always active — never disabled during streaming.
// This matches pragma behavior where users can type and queue messages
// while the assistant is responding.
type inputComponent struct {
	textarea textarea.Model
}

func newInputComponent() inputComponent {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	ta := textarea.New()
	ta.Placeholder = ""
	ta.Prompt = inputPromptStyle.Render("❯ ")
	ta.CharLimit = 0
	ta.MaxHeight = 3
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.Focus()

	return inputComponent{
		textarea: ta,
	}
}

// Update handles key events. Enter submits the message, Alt+Enter adds a newline.
func (c *inputComponent) Update(msg tea.Msg) tea.Cmd {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		observe.GlobalTrace("if: ok")
		switch keyMsg.Type {
		case tea.KeyEnter:
			observe.GlobalTrace("case: tea.KeyEnter")

			if !keyMsg.Alt {
				text := c.textarea.Value()
				if text == "" {
					observe.GlobalTrace("if: text == \"\"")
					observe.GlobalTrace("return: nil")
					return nil
				}
				c.textarea.Reset()
				observe.GlobalTrace("return: func() tea.Msg {\n\treturn InputSubmittedMsg{Text: text}\n}")
				return func() tea.Msg {
					return InputSubmittedMsg{Text: text}
				}
			}

		}
	}

	var cmd tea.Cmd
	c.textarea, cmd = c.textarea.Update(msg)
	observe.GlobalTrace("return: cmd")
	return cmd
}

// View renders the input area. Always shows the textarea (never disabled).
func (c inputComponent) View() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	return c.textarea.View()
}

// SetWidth adjusts the textarea width to fit the terminal.
func (c *inputComponent) SetWidth(width int) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.textarea.SetWidth(width)
}

// SetStreaming updates the visual state of the prompt glyph.
// When streaming, the ❯ prompt is dimmed to indicate the model is responding.
func (c *inputComponent) SetStreaming(v bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if v {
		c.textarea.Prompt = inputPromptDimStyle.Render("❯ ")
	} else {
		c.textarea.Prompt = inputPromptStyle.Render("❯ ")
	}
}

// SetQueued updates the placeholder to indicate a queued message.
// Only shown when there actually IS a queued message (avoids TS #17157).
func (c *inputComponent) SetQueued(queued bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if queued {
		c.textarea.Placeholder = "Message queued — will send when ready"
	} else {
		c.textarea.Placeholder = ""
	}
}

// Reset clears the input value and refocuses.
func (c *inputComponent) Reset() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.textarea.Reset()
	c.textarea.Focus()
}
