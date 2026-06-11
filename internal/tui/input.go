package tui

import (
	"strings"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/slash"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

const InputHistoryLimit = 100
const inputHistoryLimit = InputHistoryLimit
const completionMenuLimit = 6

// inputComponent wraps a textarea for user message input.
// The input is always active — never disabled during streaming.
// Users can type and queue messages while the assistant is responding.
type inputComponent struct {
	textarea     textarea.Model
	history      []string
	historyIndex int
	historyDraft string

	searchActive  bool
	searchQuery   string
	searchDraft   string
	searchMatches []int
	searchIndex   int

	completionInput string
	completions     []completionItem
	completionIndex int
}

type completionItem struct {
	Label       string
	Detail      string
	Replacement string
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
		if c.searchActive {
			observe.GlobalTrace("if: c.searchActive")
			if c.updateHistorySearch(keyMsg) {
				observe.GlobalTrace("if: c.updateHistorySearch(keyMsg)")
				observe.GlobalTrace("return: nil")
				return nil
			}
		} else if keyMsg.Type == tea.KeyCtrlR {
			observe.GlobalTrace("else-if: keyMsg.Type == tea.KeyCtrlR")
			c.startHistorySearch()
			return nil
		}

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
				c.historyIndex = -1
				c.historyDraft = ""
				c.resetHistorySearch()
				c.clearCompletions()
				observe.GlobalTrace("return: func() tea.Msg {\n\treturn InputSubmittedMsg{Text: text}\n}")
				return func() tea.Msg {
					return InputSubmittedMsg{Text: text}
				}
			}
		case tea.KeyUp:
			observe.GlobalTrace("case: tea.KeyUp")
			if !keyMsg.Alt && c.cycleCompletion(-1) {
				observe.GlobalTrace("return: nil")
				return nil
			}
			if !keyMsg.Alt && c.shouldNavigateHistoryUp() && c.previousHistory() {
				observe.GlobalTrace("return: nil")
				return nil
			}
		case tea.KeyDown:
			observe.GlobalTrace("case: tea.KeyDown")
			if !keyMsg.Alt && c.cycleCompletion(1) {
				observe.GlobalTrace("return: nil")
				return nil
			}
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
		c.resetHistorySearch()
	}
	observe.GlobalTrace("return: cmd")
	return cmd
}

func (c *inputComponent) shouldNavigateHistoryUp() bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: c.historyIndex >= 0 || c.textarea.Line() == 0")
	observe.GlobalTrace("return: c.historyIndex >= 0 || c.textarea.Value() == \"\" || c.textarea.Line() == 0")
	return c.historyIndex >= 0 || c.textarea.Value() == "" || c.textarea.Line() == 0
}

func (c *inputComponent) shouldNavigateHistoryDown() bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: c.historyIndex >= 0 || c.textarea.Line() >= c.textarea.LineCount()-1")
	observe.GlobalTrace("return: c.historyIndex >= 0 || c.textarea.Value() == \"\" || c.textarea.Line() >= c.tex...")
	return c.historyIndex >= 0 || c.textarea.Value() == "" || c.textarea.Line() >= c.textarea.LineCount()-1
}

func (c *inputComponent) startHistorySearch() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.searchActive = true
	c.searchDraft = c.textarea.Value()
	c.searchQuery = ""
	c.refreshSearchMatches()
	c.applySearchMatch()
}

func (c *inputComponent) updateHistorySearch(keyMsg tea.KeyMsg) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch keyMsg.Type {
	case tea.KeyCtrlR:
		observe.GlobalTrace("case: tea.KeyCtrlR")
		c.cycleSearchMatch(1)
		return true
	case tea.KeyUp:
		observe.GlobalTrace("case: tea.KeyUp")
		c.cycleSearchMatch(1)
		return true
	case tea.KeyDown:
		observe.GlobalTrace("case: tea.KeyDown")
		c.cycleSearchMatch(-1)
		return true
	case tea.KeyEnter:
		observe.GlobalTrace("case: tea.KeyEnter")
		c.acceptHistorySearch()
		return true
	case tea.KeyEsc:
		observe.GlobalTrace("case: tea.KeyEsc")
		c.cancelHistorySearch()
		return true
	case tea.KeyBackspace, tea.KeyCtrlH:
		observe.GlobalTrace("case: tea.KeyBackspace, tea.KeyCtrlH")
		if c.searchQuery != "" {
			runes := []rune(c.searchQuery)
			c.searchQuery = string(runes[:len(runes)-1])
			c.refreshSearchMatches()
			c.applySearchMatch()
		}
		return true
	case tea.KeySpace:
		observe.GlobalTrace("case: tea.KeySpace")
		c.searchQuery += " "
		c.refreshSearchMatches()
		c.applySearchMatch()
		return true
	}

	if len(keyMsg.Runes) > 0 {
		observe.GlobalTrace("if: len(keyMsg.Runes) > 0")
		c.searchQuery += string(keyMsg.Runes)
		c.refreshSearchMatches()
		c.applySearchMatch()
		observe.GlobalTrace("return: true")
		return true
	}
	observe.GlobalTrace("return: false")
	return false
}

func (c *inputComponent) refreshSearchMatches() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.searchMatches = c.searchMatches[:0]
	query := strings.ToLower(c.searchQuery)
	for i := len(c.history) - 1; i >= 0; i-- {
		observe.GlobalTrace("for: i >= 0")
		if query == "" || strings.Contains(strings.ToLower(c.history[i]), query) {
			observe.GlobalTrace("if: query == \"\" || strings.Contains(strings.ToLower(c.history[i]), query)")
			c.searchMatches = append(c.searchMatches, i)
		}
	}
	if c.searchIndex >= len(c.searchMatches) {
		observe.GlobalTrace("if: c.searchIndex >= len(c.searchMatches)")
		c.searchIndex = 0
	}
	if c.searchIndex < 0 {
		observe.GlobalTrace("if: c.searchIndex < 0")
		c.searchIndex = 0
	}
}

func (c *inputComponent) applySearchMatch() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(c.searchMatches) == 0 {
		observe.GlobalTrace("if: len(c.searchMatches) == 0")
		c.textarea.SetValue(c.searchDraft)
		c.textarea.Focus()
		return
	}
	c.textarea.SetValue(c.history[c.searchMatches[c.searchIndex]])
	c.textarea.Focus()
}

func (c *inputComponent) cycleSearchMatch(delta int) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(c.searchMatches) == 0 {
		observe.GlobalTrace("if: len(c.searchMatches) == 0")
		return
	}
	c.searchIndex = (c.searchIndex + delta + len(c.searchMatches)) % len(c.searchMatches)
	c.applySearchMatch()
}

func (c *inputComponent) acceptHistorySearch() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.searchActive = false
	c.searchQuery = ""
	c.searchDraft = ""
	c.searchMatches = nil
	c.searchIndex = 0
	c.historyIndex = -1
	c.historyDraft = ""
	c.textarea.Focus()
}

func (c *inputComponent) cancelHistorySearch() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	draft := c.searchDraft
	c.resetHistorySearch()
	c.textarea.SetValue(draft)
	c.textarea.Focus()
}

func (c *inputComponent) resetHistorySearch() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.searchActive = false
	c.searchQuery = ""
	c.searchDraft = ""
	c.searchMatches = nil
	c.searchIndex = 0
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

func (c *inputComponent) CompleteSlash(commands []slash.Command, cwd string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if c.searchActive {
		observe.GlobalTrace("if: c.searchActive")
		observe.GlobalTrace("return: false")
		return false
	}
	c.RefreshSlashCompletions(commands, cwd)
	if len(c.completions) == 0 {
		observe.GlobalTrace("if: len(c.completions) == 0")
		observe.GlobalTrace("return: false")
		return false
	}
	c.applyCompletion(c.completions[c.completionIndex].Replacement)
	observe.GlobalTrace("return: true")
	return true
}

func (c *inputComponent) RefreshSlashCompletions(commands []slash.Command, cwd string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	items := slashCompletionItems(c.textarea.Value(), commands, cwd)
	if len(items) == 0 {
		observe.GlobalTrace("if: len(items) == 0")
		c.clearCompletions()
		return
	}
	if len(items) > completionMenuLimit {
		observe.GlobalTrace("if: len(items) > completionMenuLimit")
		items = items[:completionMenuLimit]
	}
	if c.completionInput != c.textarea.Value() {
		observe.GlobalTrace("if: c.completionInput != c.textarea.Value()")
		c.completionIndex = 0
	}
	c.completionInput = c.textarea.Value()
	c.completions = items
	if c.completionIndex >= len(c.completions) {
		observe.GlobalTrace("if: c.completionIndex >= len(c.completions)")
		c.completionIndex = len(c.completions) - 1
	}
	if c.completionIndex < 0 {
		observe.GlobalTrace("if: c.completionIndex < 0")
		c.completionIndex = 0
	}
}

func (c *inputComponent) clearCompletions() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.completionInput = ""
	c.completions = nil
	c.completionIndex = 0
}

func (c *inputComponent) cycleCompletion(delta int) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(c.completions) == 0 {
		observe.GlobalTrace("if: len(c.completions) == 0")
		observe.GlobalTrace("return: false")
		return false
	}
	c.completionIndex = (c.completionIndex + delta + len(c.completions)) % len(c.completions)
	observe.GlobalTrace("return: true")
	return true
}

func (c *inputComponent) applyCompletion(value string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.textarea.SetValue(value)
	c.textarea.Focus()
	c.historyIndex = -1
	c.historyDraft = ""
	c.resetHistorySearch()
	c.clearCompletions()
}

func slashCompletionItems(value string, commands []slash.Command, cwd string) []completionItem {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	items := slash.CompletionItems(value, commands, cwd)
	out := make([]completionItem, 0, len(items))
	for _, item := range items {
		observe.GlobalTrace("range items")
		out = append(out, completionItem{
			Label:       item.Label,
			Detail:      item.Detail,
			Replacement: item.Replacement,
		})
	}
	observe.GlobalTrace("return: out")
	return out
}

// View renders the input area. Always shows the textarea (never disabled).
func (c inputComponent) View() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	view := c.textarea.View()
	if len(c.completions) == 0 {
		observe.GlobalTrace("return: view")
		return view
	}
	var b strings.Builder
	b.WriteString(view)
	for i, item := range c.completions {
		observe.GlobalTrace("range c.completions")
		b.WriteString("\n")
		line := "  " + item.Label
		if item.Detail != "" {
			observe.GlobalTrace("if: item.Detail != \"\"")
			line += "  " + completionDetailStyle.Render(item.Detail)
		}
		if i == c.completionIndex {
			observe.GlobalTrace("if: i == c.completionIndex")
			line = completionSelectedStyle.Render("> " + item.Label)
			if item.Detail != "" {
				observe.GlobalTrace("if: item.Detail != \"\"")
				line += " " + completionDetailStyle.Render(item.Detail)
			}
		} else {
			observe.GlobalTrace("else: i == c.completionIndex")
			line = completionItemStyle.Render(line)
		}
		b.WriteString(line)
	}
	observe.GlobalTrace("return: b.String()")
	return b.String()
}

func (c inputComponent) ViewHeight() int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: 3 + len(c.completions)")
	return 3 + len(c.completions)
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

// SetHistory replaces the navigation history with the given entries.
// Used when opening or resuming a conversation to include prior user prompts.
func (c *inputComponent) SetHistory(prompts []string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.history = prompts
	c.historyIndex = -1
	c.historyDraft = ""
	c.resetHistorySearch()
	c.clearCompletions()
}

// Reset clears the input value and refocuses.
func (c *inputComponent) Reset() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.textarea.Reset()
	c.textarea.Focus()
	c.historyIndex = -1
	c.historyDraft = ""
	c.resetHistorySearch()
	c.clearCompletions()
}
