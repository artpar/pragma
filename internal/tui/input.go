package tui

import (
	"github.com/artpar/pragma/internal/observe"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

const inputHistoryLimit = 100

// inputComponent wraps a textarea for user message input.
// The input is always active — never disabled during streaming.
// Users can type and queue messages while the assistant is responding.
type inputComponent struct {
	textarea     textarea.Model
	history      []string
	historyIndex int
	historyDraft string
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
	observe.GlobalTrace("return: inputComponent{\n\ttextarea: ta,\n}")
	observe.GlobalTrace("return: inputComponent{\n\ttextarea:\tta,\n\thistoryIndex:\t-1,\n}")

	return inputComponent{
		textarea:     ta,
		historyIndex: -1,
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
				c.remember(text)
				c.textarea.Reset()
				c.historyIndex = -1
				c.historyDraft = ""
				observe.GlobalTrace("return: func() tea.Msg {\n\treturn InputSubmittedMsg{Text: text}\n}")
				return func() tea.Msg {
					return InputSubmittedMsg{Text: text}
				}
			}
		case tea.KeyUp:
			observe.GlobalTrace("case: tea.KeyUp")
			if !keyMsg.Alt && c.shouldNavigateHistoryUp() && c.previousHistory() {
				observe.GlobalTrace("return: nil")
				return nil
			}
		case tea.KeyDown:
			observe.GlobalTrace("case: tea.KeyDown")
			if !keyMsg.Alt && c.shouldNavigateHistoryDown() && c.nextHistory() {
				observe.GlobalTrace("return: nil")
				return nil
			}

		}
	}

	var cmd tea.Cmd
	c.textarea, cmd = c.textarea.Update(msg)
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type != tea.KeyUp && keyMsg.Type != tea.KeyDown {
		observe.GlobalTrace("if: ok && keyMsg.Type != tea.KeyUp && keyMsg.Type != tea.KeyDown")
		c.historyIndex = -1
		c.historyDraft = ""
	}
	observe.GlobalTrace("return: cmd")
	return cmd
}

func (c *inputComponent) shouldNavigateHistoryUp() bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: c.historyIndex >= 0 || c.textarea.Line() == 0")
	return c.historyIndex >= 0 || c.textarea.Line() == 0
}

func (c *inputComponent) shouldNavigateHistoryDown() bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: c.historyIndex >= 0 || c.textarea.Line() >= c.textarea.LineCount()-1")
	return c.historyIndex >= 0 || c.textarea.Line() >= c.textarea.LineCount()-1
}

func (c *inputComponent) previousHistory() bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(c.history) == 0 {
		observe.GlobalTrace("if: len(c.history) == 0")
		observe.GlobalTrace("return: false")
		return false
	}
	if c.historyIndex < 0 {
		observe.GlobalTrace("if: c.historyIndex < 0")
		c.historyDraft = c.textarea.Value()
		c.historyIndex = len(c.history) - 1
	} else if c.historyIndex > 0 {
		observe.GlobalTrace("else-if: c.historyIndex > 0")
		c.historyIndex--
	}
	c.textarea.SetValue(c.history[c.historyIndex])
	c.textarea.Focus()
	observe.GlobalTrace("return: true")
	return true
}

func (c *inputComponent) nextHistory() bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(c.history) == 0 || c.historyIndex < 0 {
		observe.GlobalTrace("if: len(c.history) == 0 || c.historyIndex < 0")
		observe.GlobalTrace("return: false")
		return false
	}
	c.historyIndex++
	if c.historyIndex >= len(c.history) {
		observe.GlobalTrace("if: c.historyIndex >= len(c.history)")
		c.historyIndex = -1
		c.textarea.SetValue(c.historyDraft)
		c.historyDraft = ""
		c.textarea.Focus()
		observe.GlobalTrace("return: true")
		return true
	}
	c.textarea.SetValue(c.history[c.historyIndex])
	c.textarea.Focus()
	observe.GlobalTrace("return: true")
	return true
}

func (c *inputComponent) remember(text string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if text == "" {
		observe.GlobalTrace("if: text == \"\"")
		return
	}
	if len(c.history) > 0 && c.history[len(c.history)-1] == text {
		observe.GlobalTrace("if: len(c.history) > 0 && c.history[len(c.history)-1] == text")
		return
	}
	c.history = append(c.history, text)
	if len(c.history) > inputHistoryLimit {
		observe.GlobalTrace("if: len(c.history) > inputHistoryLimit")
		c.history = c.history[len(c.history)-inputHistoryLimit:]
	}
}

// View renders the input area. Always shows the textarea (never disabled).
func (c inputComponent) View() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: c.textarea.View()")
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
		observe.GlobalTrace("if: v")
		c.textarea.Prompt = inputPromptDimStyle.Render("❯ ")
	} else {
		observe.GlobalTrace("else: v")
		c.textarea.Prompt = inputPromptStyle.Render("❯ ")
	}
}

// SetQueued updates the placeholder to indicate a queued message.
// Only shown when there actually IS a queued message (avoids TS #17157).
func (c *inputComponent) SetQueued(queued bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if queued {
		observe.GlobalTrace("if: queued")
		c.textarea.Placeholder = "Message queued — will send when ready"
	} else {
		observe.GlobalTrace("else: queued")
		c.textarea.Placeholder = ""
	}
}

// Reset clears the input value and refocuses.
func (c *inputComponent) Reset() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.textarea.Reset()
	c.textarea.Focus()
	c.historyIndex = -1
	c.historyDraft = ""
}
