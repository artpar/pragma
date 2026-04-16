package tui

import (
	"github.com/artpar/pragma/internal/observe"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

// inputComponent wraps a textarea for user message input.
// It handles submit (Enter), newlines (Shift+Enter / Alt+Enter),
// and active/inactive states.
type inputComponent struct {
	textarea textarea.Model
	active   bool
}

func newInputComponent() inputComponent {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	ta := textarea.New()
	ta.Placeholder = "Type a message..."
	ta.Prompt = inputPromptStyle.Render("> ")
	ta.CharLimit = 0
	ta.MaxHeight = 5
	ta.ShowLineNumbers = false
	ta.Focus()
	observe.GlobalTrace("return: inputComponent{\n\ttextarea:\tta,\n\tactive:\t\ttrue,\n}")

	return inputComponent{
		textarea: ta,
		active:   true,
	}
}

// Update handles key events. Enter submits the message, Alt+Enter adds a newline.
func (c *inputComponent) Update(msg tea.Msg) tea.Cmd {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !c.active {
		observe.GlobalTrace("if: !c.active")
		observe.GlobalTrace("return: nil")
		return nil
	}

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

// View renders the input area.
func (c inputComponent) View() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !c.active {
		observe.GlobalTrace("if: !c.active")
		observe.GlobalTrace("return: inputPromptStyle.Render(\"> \") + thinkingStyle.Render(\"waiting...\")")
		return inputPromptStyle.Render("> ") + thinkingStyle.Render("waiting...")
	}
	observe.GlobalTrace("return: c.textarea.View()")
	return c.textarea.View()
}

// SetActive enables or disables the input.
func (c *inputComponent) SetActive(active bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.active = active
	if active {
		observe.GlobalTrace("if: active")
		c.textarea.Focus()
	} else {
		observe.GlobalTrace("else: active")
		c.textarea.Blur()
	}
}

// SetWidth adjusts the textarea width to fit the terminal.
func (c *inputComponent) SetWidth(width int) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.textarea.SetWidth(width)
}

// Reset clears the input value and refocuses.
func (c *inputComponent) Reset() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.textarea.Reset()
	c.textarea.Focus()
}
