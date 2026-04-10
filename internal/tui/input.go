package tui

import (
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
	ta := textarea.New()
	ta.Placeholder = "Type a message..."
	ta.Prompt = inputPromptStyle.Render("> ")
	ta.CharLimit = 0 // unlimited
	ta.MaxHeight = 5
	ta.ShowLineNumbers = false
	ta.Focus()

	return inputComponent{
		textarea: ta,
		active:   true,
	}
}

// Update handles key events. Enter submits the message, Alt+Enter adds a newline.
func (c *inputComponent) Update(msg tea.Msg) tea.Cmd {
	if !c.active {
		return nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyEnter:
			// Enter submits the message (unless Alt/Shift held)
			if !keyMsg.Alt {
				text := c.textarea.Value()
				if text == "" {
					return nil
				}
				c.textarea.Reset()
				return func() tea.Msg {
					return InputSubmittedMsg{Text: text}
				}
			}
			// Alt+Enter falls through to textarea for newline
		}
	}

	var cmd tea.Cmd
	c.textarea, cmd = c.textarea.Update(msg)
	return cmd
}

// View renders the input area.
func (c inputComponent) View() string {
	if !c.active {
		return inputPromptStyle.Render("> ") + thinkingStyle.Render("waiting...")
	}
	return c.textarea.View()
}

// SetActive enables or disables the input.
func (c *inputComponent) SetActive(active bool) {
	c.active = active
	if active {
		c.textarea.Focus()
	} else {
		c.textarea.Blur()
	}
}

// SetWidth adjusts the textarea width to fit the terminal.
func (c *inputComponent) SetWidth(width int) {
	c.textarea.SetWidth(width)
}

// Reset clears the input value and refocuses.
func (c *inputComponent) Reset() {
	c.textarea.Reset()
	c.textarea.Focus()
}
