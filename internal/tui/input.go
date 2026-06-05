package tui

import (
	"os"
	"path/filepath"
	"sort"
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

type scoredCompletionItem struct {
	item  completionItem
	score int
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
			if c.updateHistorySearch(keyMsg) {
				return nil
			}
		} else if keyMsg.Type == tea.KeyCtrlR {
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
				c.remember(text)
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
	return c.historyIndex >= 0 || c.textarea.Value() == "" || c.textarea.Line() == 0
}

func (c *inputComponent) shouldNavigateHistoryDown() bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: c.historyIndex >= 0 || c.textarea.Line() >= c.textarea.LineCount()-1")
	return c.historyIndex >= 0 || c.textarea.Value() == "" || c.textarea.Line() >= c.textarea.LineCount()-1
}

func (c *inputComponent) startHistorySearch() {
	c.searchActive = true
	c.searchDraft = c.textarea.Value()
	c.searchQuery = ""
	c.refreshSearchMatches()
	c.applySearchMatch()
}

func (c *inputComponent) updateHistorySearch(keyMsg tea.KeyMsg) bool {
	switch keyMsg.Type {
	case tea.KeyCtrlR:
		c.cycleSearchMatch(1)
		return true
	case tea.KeyUp:
		c.cycleSearchMatch(1)
		return true
	case tea.KeyDown:
		c.cycleSearchMatch(-1)
		return true
	case tea.KeyEnter:
		c.acceptHistorySearch()
		return true
	case tea.KeyEsc:
		c.cancelHistorySearch()
		return true
	case tea.KeyBackspace, tea.KeyCtrlH:
		if c.searchQuery != "" {
			runes := []rune(c.searchQuery)
			c.searchQuery = string(runes[:len(runes)-1])
			c.refreshSearchMatches()
			c.applySearchMatch()
		}
		return true
	case tea.KeySpace:
		c.searchQuery += " "
		c.refreshSearchMatches()
		c.applySearchMatch()
		return true
	}

	if len(keyMsg.Runes) > 0 {
		c.searchQuery += string(keyMsg.Runes)
		c.refreshSearchMatches()
		c.applySearchMatch()
		return true
	}
	return false
}

func (c *inputComponent) refreshSearchMatches() {
	c.searchMatches = c.searchMatches[:0]
	query := strings.ToLower(c.searchQuery)
	for i := len(c.history) - 1; i >= 0; i-- {
		if query == "" || strings.Contains(strings.ToLower(c.history[i]), query) {
			c.searchMatches = append(c.searchMatches, i)
		}
	}
	if c.searchIndex >= len(c.searchMatches) {
		c.searchIndex = 0
	}
	if c.searchIndex < 0 {
		c.searchIndex = 0
	}
}

func (c *inputComponent) applySearchMatch() {
	if len(c.searchMatches) == 0 {
		c.textarea.SetValue(c.searchDraft)
		c.textarea.Focus()
		return
	}
	c.textarea.SetValue(c.history[c.searchMatches[c.searchIndex]])
	c.textarea.Focus()
}

func (c *inputComponent) cycleSearchMatch(delta int) {
	if len(c.searchMatches) == 0 {
		return
	}
	c.searchIndex = (c.searchIndex + delta + len(c.searchMatches)) % len(c.searchMatches)
	c.applySearchMatch()
}

func (c *inputComponent) acceptHistorySearch() {
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
	draft := c.searchDraft
	c.resetHistorySearch()
	c.textarea.SetValue(draft)
	c.textarea.Focus()
}

func (c *inputComponent) resetHistorySearch() {
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
	if c.searchActive {
		return false
	}
	c.RefreshSlashCompletions(commands, cwd)
	if len(c.completions) == 0 {
		return false
	}
	c.applyCompletion(c.completions[c.completionIndex].Replacement)
	return true
}

func (c *inputComponent) RefreshSlashCompletions(commands []slash.Command, cwd string) {
	items := slashCompletionItems(c.textarea.Value(), commands, cwd)
	if len(items) == 0 {
		c.clearCompletions()
		return
	}
	if len(items) > completionMenuLimit {
		items = items[:completionMenuLimit]
	}
	if c.completionInput != c.textarea.Value() {
		c.completionIndex = 0
	}
	c.completionInput = c.textarea.Value()
	c.completions = items
	if c.completionIndex >= len(c.completions) {
		c.completionIndex = len(c.completions) - 1
	}
	if c.completionIndex < 0 {
		c.completionIndex = 0
	}
}

func (c *inputComponent) clearCompletions() {
	c.completionInput = ""
	c.completions = nil
	c.completionIndex = 0
}

func (c *inputComponent) cycleCompletion(delta int) bool {
	if len(c.completions) == 0 {
		return false
	}
	c.completionIndex = (c.completionIndex + delta + len(c.completions)) % len(c.completions)
	return true
}

func (c *inputComponent) applyCompletion(value string) {
	c.textarea.SetValue(value)
	c.textarea.Focus()
	c.historyIndex = -1
	c.historyDraft = ""
	c.resetHistorySearch()
	c.clearCompletions()
}

func slashCompletionItems(value string, commands []slash.Command, cwd string) []completionItem {
	if !strings.HasPrefix(value, "/") {
		return nil
	}
	if strings.Contains(value, "\n") {
		return nil
	}

	cmdToken, hasArgs := firstSlashToken(value)
	if !hasArgs {
		return slashCommandCompletionItems(value, strings.TrimPrefix(cmdToken, "/"), commands)
	}

	name := strings.TrimPrefix(cmdToken, "/")
	if name != "orchestrate" && name != "fsm" {
		return nil
	}
	return orchestrateCompletionItems(value, cwd)
}

func firstSlashToken(value string) (string, bool) {
	rest := strings.TrimPrefix(value, "/")
	i := strings.IndexAny(rest, " \t")
	if i < 0 {
		return value, false
	}
	return "/" + rest[:i], true
}

func slashCommandCompletionItems(value, prefix string, commands []slash.Command) []completionItem {
	seen := make(map[string]bool)
	var scored []scoredCompletionItem
	for _, cmd := range commands {
		add := func(name, detail string) {
			score, ok := fuzzyCompletionScore(name, prefix)
			if name == "" || seen[name] || !ok {
				return
			}
			seen[name] = true
			scored = append(scored, scoredCompletionItem{
				item: completionItem{
					Label:       "/" + name,
					Detail:      detail,
					Replacement: "/" + name + " ",
				},
				score: score,
			})
		}
		add(cmd.Name, cmd.Description)
		for _, alias := range cmd.Aliases {
			add(alias, "alias for /"+cmd.Name)
		}
	}
	return sortedCompletionItems(scored)
}

func orchestrateCompletionItems(value, cwd string) []completionItem {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return nil
	}
	endsSpace := endsWithSpace(value)
	args := fields[1:]
	current := ""
	previous := fields[len(fields)-1]
	currentArgIndex := -1
	if !endsSpace {
		current = fields[len(fields)-1]
		if len(fields) > 1 {
			currentArgIndex = len(fields) - 2
			previous = fields[len(fields)-2]
		}
	}

	state := parseOrchestrateCompletionState(args, currentArgIndex)
	switch {
	case state.currentRole == "definition":
		return pathCompletionItems(value, current, cwd, pathCompletionYAML)
	case state.currentRole == "persona":
		return pathCompletionItems(value, current, cwd, pathCompletionDir)
	case state.currentRole == "flag":
		if strings.HasPrefix(current, "--persona-dir=") {
			prefix := strings.TrimPrefix(current, "--persona-dir=")
			items := pathCompletionItems(value, prefix, cwd, pathCompletionDir)
			for i := range items {
				replacement := "--persona-dir=" + items[i].Label + " "
				items[i].Replacement = replaceCurrentToken(value, replacement)
			}
			return items
		}
		return flagCompletionItems(value, current, state.flagCandidates())
	case endsSpace && previous != "--prompt":
		switch {
		case previous == "--persona-dir":
			return pathCompletionItems(value, "", cwd, pathCompletionDir)
		case !state.hasDefinition:
			return pathCompletionItems(value, "", cwd, pathCompletionYAML)
		default:
			return flagCompletionItems(value, "", state.flagCandidates())
		}
	}
	return nil
}

func flagCompletionItems(value, prefix string, flags []string) []completionItem {
	var scored []scoredCompletionItem
	for _, flag := range flags {
		score, ok := fuzzyCompletionScore(flag, prefix)
		if ok {
			scored = append(scored, scoredCompletionItem{
				item: completionItem{
					Label:       flag,
					Detail:      flagDetail(flag),
					Replacement: replaceCurrentToken(value, flag+" "),
				},
				score: score,
			})
		}
	}
	return sortedCompletionItems(scored)
}

func flagDetail(flag string) string {
	switch flag {
	case "--persona-dir":
		return "persona directory"
	case "--prompt":
		return "task prompt"
	default:
		return "flag"
	}
}

type orchestrateCompletionState struct {
	hasDefinition bool
	hasPersonaDir bool
	hasPrompt     bool
	currentRole   string
}

func parseOrchestrateCompletionState(args []string, currentIndex int) orchestrateCompletionState {
	state := orchestrateCompletionState{}
	expectPersona := false
	inPrompt := false
	for i, arg := range args {
		role := "prompt"
		switch {
		case inPrompt:
			role = "prompt"
		case expectPersona:
			role = "persona"
			expectPersona = false
		case arg == "--persona-dir":
			role = "flag"
			state.hasPersonaDir = true
			expectPersona = true
		case strings.HasPrefix(arg, "--persona-dir="):
			role = "flag"
			state.hasPersonaDir = true
		case arg == "--prompt":
			role = "flag"
			state.hasPrompt = true
			inPrompt = true
		case strings.HasPrefix(arg, "--prompt="):
			role = "flag"
			state.hasPrompt = true
		case strings.HasPrefix(arg, "--"):
			role = "flag"
		case !state.hasDefinition:
			role = "definition"
			state.hasDefinition = true
		}
		if i == currentIndex {
			state.currentRole = role
		}
	}
	if state.currentRole == "" && currentIndex >= 0 {
		state.currentRole = "prompt"
	}
	return state
}

func (s orchestrateCompletionState) flagCandidates() []string {
	if !s.hasPersonaDir {
		return []string{"--persona-dir"}
	}
	if !s.hasPrompt {
		return []string{"--prompt"}
	}
	return nil
}

type pathCompletionKind int

const (
	pathCompletionYAML pathCompletionKind = iota
	pathCompletionDir
)

func pathCompletionItems(value, prefix, cwd string, kind pathCompletionKind) []completionItem {
	if cwd == "" {
		cwd = "."
	}
	dirPart, basePart := filepath.Split(prefix)
	readDir := dirPart
	if readDir == "" {
		readDir = "."
	}
	if !filepath.IsAbs(readDir) {
		readDir = filepath.Join(cwd, readDir)
	}
	entries, err := os.ReadDir(readDir)
	if err != nil {
		return nil
	}
	var scored []scoredCompletionItem
	for _, entry := range entries {
		name := entry.Name()
		score, ok := fuzzyCompletionScore(name, basePart)
		if !ok {
			continue
		}
		isDir := entry.IsDir()
		if kind == pathCompletionDir && !isDir {
			continue
		}
		if kind == pathCompletionYAML && !isDir && !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		candidate := filepath.ToSlash(filepath.Join(dirPart, name))
		if isDir {
			candidate += "/"
		}
		detail := "directory"
		if !isDir {
			detail = "orchestration yaml"
		}
		replacement := candidate
		if kind == pathCompletionDir || !isDir {
			replacement += " "
		}
		scored = append(scored, scoredCompletionItem{
			item: completionItem{
				Label:       candidate,
				Detail:      detail,
				Replacement: replaceCurrentToken(value, replacement),
			},
			score: score,
		})
	}
	return sortedCompletionItems(scored)
}

func sortedCompletionItems(scored []scoredCompletionItem) []completionItem {
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].item.Label < scored[j].item.Label
	})
	items := make([]completionItem, 0, len(scored))
	for _, entry := range scored {
		items = append(items, entry.item)
	}
	return items
}

func fuzzyCompletionScore(candidate, query string) (int, bool) {
	query = strings.ToLower(query)
	candidate = strings.ToLower(candidate)
	if query == "" {
		return 0, true
	}
	if strings.HasPrefix(candidate, query) {
		return 100000 - len(candidate), true
	}

	score := 50000 - len(candidate)
	lastMatch := -1
	searchFrom := 0
	for _, want := range query {
		match := -1
		for i, have := range candidate[searchFrom:] {
			if have == want {
				match = searchFrom + i
				break
			}
		}
		if match < 0 {
			return 0, false
		}
		score += 100
		if isCompletionBoundary(candidate, match) {
			score += 25
		}
		if lastMatch >= 0 {
			gap := match - lastMatch - 1
			if gap == 0 {
				score += 50
			} else {
				score -= gap
			}
		}
		lastMatch = match
		searchFrom = match + 1
	}
	return score, true
}

func isCompletionBoundary(value string, index int) bool {
	if index == 0 || index >= len(value) {
		return true
	}
	switch value[index-1] {
	case '-', '_', '.', '/', ' ':
		return true
	default:
		return false
	}
}

func replaceCurrentToken(value, replacement string) string {
	if endsWithSpace(value) {
		return value + replacement
	}
	start := strings.LastIndexAny(value, " \t")
	if start < 0 {
		return replacement
	}
	return value[:start+1] + replacement
}

func endsWithSpace(value string) bool {
	return value != "" && (value[len(value)-1] == ' ' || value[len(value)-1] == '\t')
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
		b.WriteString("\n")
		line := "  " + item.Label
		if item.Detail != "" {
			line += "  " + completionDetailStyle.Render(item.Detail)
		}
		if i == c.completionIndex {
			line = completionSelectedStyle.Render("> " + item.Label)
			if item.Detail != "" {
				line += " " + completionDetailStyle.Render(item.Detail)
			}
		} else {
			line = completionItemStyle.Render(line)
		}
		b.WriteString(line)
	}
	observe.GlobalTrace("return: b.String()")
	return b.String()
}

func (c inputComponent) ViewHeight() int {
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
