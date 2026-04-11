package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/hook"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/query"
	"github.com/artpar/gogent/internal/slash"
)

// Config holds all dependencies for the TUI model.
type Config struct {
	ParentCtx   context.Context // parent context for cancellation propagation (e.g., cmd.Context())
	Engine      *query.Engine
	Store       *app.StateStore
	CostTracker *model.CostTracker
	ModelName   string
	Provider    string
	SessionSave func()
	SlashCmds   *slash.Registry
	SlashDeps   slash.Deps
	HookMgr     *hook.Manager // nil if no hooks configured
}

// Model is the main bubbletea model for the interactive TUI.
type Model struct {
	// Dependencies
	engine      *query.Engine
	store       *app.StateStore
	costTracker *model.CostTracker
	sessionSave func()
	slashCmds   *slash.Registry
	slashDeps   slash.Deps
	hookMgr     *hook.Manager

	// Components
	viewport viewport.Model
	input    inputComponent
	perm     permissionDialog
	ask      askDialog
	toolbar  toolbar

	// Streaming state
	outputBuf      *strings.Builder // accumulated rendered output for viewport
	streamBuf      *strings.Builder // current streaming text (not yet finalized)
	eventCh        <-chan query.LoopEvent
	streaming      bool
	ctx            context.Context
	cancel         context.CancelFunc
	interruptCount int

	// Layout
	width  int
	height int
	ready  bool
}

// New creates a new TUI model with all dependencies.
func New(cfg Config) Model {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	parentCtx := cfg.ParentCtx
	if parentCtx == nil {
		observe.GlobalTrace("if: parentCtx == nil")
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(parentCtx)
	observe.GlobalTrace("return: Model{\n\tengine:\t\tcfg.Engine,\n\tstore:\t\tcfg.Store,\n\tcostTracker:\tcfg.CostTracke...")

	return Model{
		engine:      cfg.Engine,
		store:       cfg.Store,
		costTracker: cfg.CostTracker,
		sessionSave: cfg.SessionSave,
		slashCmds:   cfg.SlashCmds,
		slashDeps:   cfg.SlashDeps,
		hookMgr:     cfg.HookMgr,
		input:       newInputComponent(),
		perm:        newPermissionDialog(),
		toolbar:     newToolbar(cfg.ModelName, cfg.Provider),
		outputBuf:   &strings.Builder{},
		streamBuf:   &strings.Builder{},
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Init is the bubbletea initialization command.
func (m Model) Init() tea.Cmd {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: m.input.textarea.Focus()")
	return m.input.textarea.Focus()
}

// Update handles all messages in the bubbletea event loop.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		observe.GlobalTrace("typecase: tea.WindowSizeMsg")
		return m.handleResize(msg)

	case tea.KeyMsg:
		observe.GlobalTrace("typecase: tea.KeyMsg")
		return m.handleKey(msg)

	case InputSubmittedMsg:
		observe.GlobalTrace("typecase: InputSubmittedMsg")
		return m.handleInputSubmitted(msg)

	case LoopEventMsg:
		observe.GlobalTrace("typecase: LoopEventMsg")
		return m.handleLoopEvent(msg)

	case PermRequestMsg:
		observe.GlobalTrace("typecase: PermRequestMsg")
		return m.handlePermRequest(msg)

	case PermResponseMsg:
		observe.GlobalTrace("typecase: PermResponseMsg")
		return m.handlePermResponse(msg)

	case AskRequestMsg:
		observe.GlobalTrace("typecase: AskRequestMsg")
		return m.handleAskRequest(msg)

	case SlashResultMsg:
		observe.GlobalTrace("typecase: SlashResultMsg")
		return m.handleSlashResult(msg)

	case sessionSavedMsg:
		observe.GlobalTrace("typecase: sessionSavedMsg")
		return m, nil
	}

	if m.ask.active {
		observe.GlobalTrace("if: m.ask.active")
		observe.GlobalTrace("return: m, nil")
		return m, nil
	}
	if m.perm.active {
		observe.GlobalTrace("if: m.perm.active")
		cmd := m.perm.Update(msg)
		observe.GlobalTrace("return: m, cmd")
		return m, cmd
	}

	cmd := m.input.Update(msg)
	observe.GlobalTrace("return: m, cmd")
	return m, cmd
}

// View renders the full TUI layout.
func (m Model) View() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !m.ready {
		observe.GlobalTrace("if: !m.ready")
		observe.GlobalTrace("return: \"Initializing...\"")
		return "Initializing..."
	}

	var b strings.Builder

	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	if m.perm.active {
		observe.GlobalTrace("if: m.perm.active")
		b.WriteString(m.perm.View())
		b.WriteString("\n")
	}

	if m.ask.active {
		observe.GlobalTrace("if: m.ask.active")
		b.WriteString(m.ask.View())
		b.WriteString("\n")
	}

	b.WriteString(m.toolbar.View(m.width))
	b.WriteString("\n")

	b.WriteString(m.input.View())
	observe.GlobalTrace("return: b.String()")

	return b.String()
}

// handleResize adjusts all components to the new terminal size.
func (m Model) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.width = msg.Width
	m.height = msg.Height

	inputHeight := 3
	toolbarHeight := 1
	headerHeight := inputHeight + toolbarHeight + 2

	vpHeight := max(m.height-headerHeight, 1)

	if !m.ready {
		observe.GlobalTrace("if: !m.ready")
		m.viewport = viewport.New(m.width, vpHeight)
		m.ready = true

		snap := m.store.Snapshot()
		if len(snap.Conversation.Messages) > 0 {
			observe.GlobalTrace("if: len(snap.Conversation.Messages) > 0")
			m.outputBuf.WriteString(renderConversation(snap.Conversation.Messages))
			m.viewport.SetContent(m.outputBuf.String())
			m.viewport.GotoBottom()
		}
	} else {
		observe.GlobalTrace("else: !m.ready")
		m.viewport.Width = m.width
		m.viewport.Height = vpHeight
	}

	m.input.SetWidth(m.width)
	observe.GlobalTrace("return: m, nil")
	return m, nil
}

// handleKey processes keyboard input.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch msg.Type {
	case tea.KeyCtrlC:
		observe.GlobalTrace("case: tea.KeyCtrlC")
		if m.streaming {
			m.interruptCount++
			if m.interruptCount >= 2 {
				observe.GlobalTrace("if: m.interruptCount >= 2")
				observe.GlobalTrace("return: m.quit()")
				return m.quit()
			}

			m.cancel()
			observe.GlobalTrace("return: m, nil")
			return m, nil
		}
		return m.quit()

	case tea.KeyEsc:
		observe.GlobalTrace("case: tea.KeyEsc")
		if m.perm.active {
			cmd := m.perm.Update(msg)
			observe.GlobalTrace("return: m, cmd")
			return m, cmd
		}
		if m.ask.active {
			observe.GlobalTrace("return: m, nil")

			return m, nil
		}
	}

	if m.perm.active {
		observe.GlobalTrace("if: m.perm.active")
		cmd := m.perm.Update(msg)
		observe.GlobalTrace("return: m, cmd")
		return m, cmd
	}
	if m.ask.active {
		observe.GlobalTrace("if: m.ask.active")
		cmd := m.ask.Update(msg)
		if !m.ask.active {
			observe.GlobalTrace("if: !m.ask.active")

			if m.streaming {
				observe.GlobalTrace("if: m.streaming")
				m.toolbar.SetStatus("streaming...")
			} else {
				observe.GlobalTrace("else: m.streaming")
				m.toolbar.SetStatus("ready")
			}
		}
		observe.GlobalTrace("return: m, cmd")
		return m, cmd
	}

	cmd := m.input.Update(msg)
	observe.GlobalTrace("return: m, cmd")
	return m, cmd
}

// handleInputSubmitted processes user message submission.
func (m Model) handleInputSubmitted(msg InputSubmittedMsg) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if m.streaming {
		observe.GlobalTrace("if: m.streaming")
		observe.GlobalTrace("return: m, nil")
		return m, nil
	}

	if name, args, ok := slash.Parse(msg.Text); ok {
		observe.GlobalTrace("if: ok")
		observe.GlobalTrace("return: m.handleSlashCommand(name, args)")
		return m.handleSlashCommand(name, args)
	}

	// UserPromptSubmit hook — can block submission
	if m.hookMgr != nil {
		hookResult := m.hookMgr.Execute(context.Background(), hook.UserPromptSubmit, hook.HookInput{
			PromptText: msg.Text,
		})
		if hookResult.Blocked {
			m.outputBuf.WriteString(errorStyle.Render("Blocked: "+hookResult.BlockMsg) + "\n")
			m.viewport.SetContent(m.outputBuf.String())
			m.viewport.GotoBottom()
			return m, nil
		}
	}

	userMsg := model.Message{
		ID:   model.NewUUID(),
		Role: model.RoleUser,
		Content: []model.ContentPart{
			model.TextPart{Text: msg.Text},
		},
	}
	m.outputBuf.WriteString(renderMessage(userMsg))
	m.viewport.SetContent(m.outputBuf.String())
	m.viewport.GotoBottom()

	m.streaming = true
	m.interruptCount = 0
	m.input.SetActive(false)
	m.toolbar.SetStatus("streaming...")
	m.toolbar.IncrementTurn()

	// Cancel previous context before creating a new one to avoid goroutine leaks.
	m.cancel()
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.eventCh = m.engine.Run(m.ctx, msg.Text)

	m.outputBuf.WriteString(assistantLabelStyle.Render("Assistant"))
	m.outputBuf.WriteString("\n")
	observe.GlobalTrace("return: m, waitForEvent(m.eventCh)")

	return m, waitForEvent(m.eventCh)
}

// handleSlashCommand dispatches a slash command and returns a SlashResultMsg.
func (m Model) handleSlashCommand(name, args string) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	trimmedArgs := strings.TrimSpace(args)
	label := "/" + name
	if trimmedArgs != "" {
		observe.GlobalTrace("if: trimmedArgs != \"\"")
		label += " " + trimmedArgs
	}
	m.outputBuf.WriteString(userLabelStyle.Render("> "+label) + "\n")
	m.viewport.SetContent(m.outputBuf.String())
	m.viewport.GotoBottom()

	slashCmds := m.slashCmds
	slashDeps := m.slashDeps
	observe.GlobalTrace("return: m, func() tea.Msg {\n\tresult, err := slashCmds.Execute(context.Background(), n...")
	return m, func() tea.Msg {
		result, err := slashCmds.Execute(context.Background(), name, trimmedArgs, slashDeps)
		return SlashResultMsg{Result: result, Err: err}
	}
}

// handleSlashResult processes the output of a slash command.
func (m Model) handleSlashResult(msg SlashResultMsg) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if msg.Err != nil {
		observe.GlobalTrace("if: msg.Err != nil")
		m.outputBuf.WriteString(errorStyle.Render("Error: "+msg.Err.Error()) + "\n\n")
	} else {
		observe.GlobalTrace("else: msg.Err != nil")
		if msg.Result.Quit {
			observe.GlobalTrace("if: msg.Result.Quit")
			observe.GlobalTrace("return: m.quit()")
			return m.quit()
		}
		if msg.Result.ClearConversation {
			observe.GlobalTrace("if: msg.Result.ClearConversation")
			m.outputBuf.Reset()
		}
		if msg.Result.DisplayText != "" {
			observe.GlobalTrace("if: msg.Result.DisplayText != \"\"")
			m.outputBuf.WriteString(msg.Result.DisplayText + "\n\n")
		}
	}
	m.viewport.SetContent(m.outputBuf.String())
	m.viewport.GotoBottom()
	observe.GlobalTrace("return: m, nil")
	return m, nil
}

// handleLoopEvent processes a streaming event from the query engine.
func (m Model) handleLoopEvent(msg LoopEventMsg) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if msg.Event == nil {
		observe.GlobalTrace("if: msg.Event == nil")
		observe.GlobalTrace("return: m.finishTurn(), nil")

		return m.finishTurn(), nil
	}

	switch e := msg.Event.(type) {
	case query.CompactionEvent:
		observe.GlobalTrace("typecase: query.CompactionEvent")
		m.flushStreamBuf()
		m.outputBuf.WriteString(thinkingStyle.Render(
			fmt.Sprintf("[auto-compacted: %d → %d tokens]", e.PreTokens, e.PostTokens)) + "\n")
		m.viewport.SetContent(m.outputBuf.String())
		m.viewport.GotoBottom()

	case query.TextEvent:
		observe.GlobalTrace("typecase: query.TextEvent")
		m.streamBuf.WriteString(e.Text)
		m.viewport.SetContent(m.outputBuf.String() + m.streamBuf.String())
		m.viewport.GotoBottom()

	case query.ThinkingEvent:
		observe.GlobalTrace("typecase: query.ThinkingEvent")
		m.flushStreamBuf()
		m.outputBuf.WriteString(thinkingStyle.Render(e.Text))
		m.viewport.SetContent(m.outputBuf.String())
		m.viewport.GotoBottom()

	case query.ToolCallEvent:
		observe.GlobalTrace("typecase: query.ToolCallEvent")

		m.flushStreamBuf()
		m.outputBuf.WriteString(renderToolCall(e.Call))
		m.outputBuf.WriteString("\n")
		m.toolbar.SetStatus("executing: " + e.Call.Name)
		m.viewport.SetContent(m.outputBuf.String())
		m.viewport.GotoBottom()

	case query.ToolResultEvent:
		observe.GlobalTrace("typecase: query.ToolResultEvent")
		m.outputBuf.WriteString(renderToolResult(e.Result))
		m.outputBuf.WriteString("\n")
		m.toolbar.SetStatus("streaming...")
		m.viewport.SetContent(m.outputBuf.String())
		m.viewport.GotoBottom()

	case query.TurnCompleteEvent:
		observe.GlobalTrace("typecase: query.TurnCompleteEvent")
		m.flushStreamBuf()
		if e.StopReason == model.StopMaxTokens {
			m.outputBuf.WriteString("\n" + thinkingStyle.Render("[response truncated — hit max_tokens limit]") + "\n")
		}
		m.outputBuf.WriteString("\n")
		m.toolbar.UpdateCost(m.costTracker.TotalUSD())
		return m.finishTurn(), saveSessionCmd(m.sessionSave)

	case query.ErrorEvent:
		observe.GlobalTrace("typecase: query.ErrorEvent")
		m.flushStreamBuf()
		if m.ctx.Err() != nil {

			m.outputBuf.WriteString("\n" + thinkingStyle.Render("[interrupted]") + "\n\n")
		} else {
			m.outputBuf.WriteString("\n" + errorStyle.Render("Error: "+e.Err.Error()) + "\n\n")
		}
		return m.finishTurn(), nil
	}
	observe.GlobalTrace("return: m, waitForEvent(m.eventCh)")

	return m, waitForEvent(m.eventCh)
}

// handleAskRequest shows the ask dialog for a tool question.
func (m Model) handleAskRequest(msg AskRequestMsg) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.ask.Show(&msg)
	m.toolbar.SetStatus("waiting for answer...")
	observe.GlobalTrace("return: m, nil")
	return m, nil
}

// handlePermRequest shows the permission dialog.
func (m Model) handlePermRequest(msg PermRequestMsg) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.perm.Show(&msg)
	m.toolbar.SetStatus("waiting for permission...")
	observe.GlobalTrace("return: m, nil")
	return m, nil
}

// handlePermResponse hides the permission dialog after user decision.
func (m Model) handlePermResponse(_ PermResponseMsg) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if m.streaming {
		observe.GlobalTrace("if: m.streaming")
		m.toolbar.SetStatus("streaming...")
	} else {
		observe.GlobalTrace("else: m.streaming")
		m.toolbar.SetStatus("ready")
	}
	observe.GlobalTrace("return: m, nil")
	return m, nil
}

// finishTurn resets streaming state and re-enables input.
func (m Model) finishTurn() Model {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.streaming = false
	m.eventCh = nil
	m.input.SetActive(true)
	m.toolbar.SetStatus("ready")
	m.viewport.SetContent(m.outputBuf.String())
	m.viewport.GotoBottom()
	observe.GlobalTrace("return: m")
	return m
}

// flushStreamBuf moves accumulated streaming text into the permanent output buffer.
func (m Model) flushStreamBuf() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if m.streamBuf.Len() > 0 {
		observe.GlobalTrace("if: m.streamBuf.Len() > 0")
		m.outputBuf.WriteString(m.streamBuf.String())
		m.streamBuf.Reset()
	}
}

// quit saves the session and exits.
func (m Model) quit() (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.cancel()
	if m.sessionSave != nil {
		observe.GlobalTrace("if: m.sessionSave != nil")
		m.sessionSave()
	}
	observe.GlobalTrace("return: m, tea.Quit")
	return m, tea.Quit
}

// waitForEvent returns a tea.Cmd that reads the next event from the channel.
func waitForEvent(ch <-chan query.LoopEvent) tea.Cmd {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: func() tea.Msg {\n\tevent, ok := <-ch\n\tif !ok {\n\t\treturn LoopEventMsg{Event: ni...")
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			return LoopEventMsg{Event: nil}
		}
		return LoopEventMsg{Event: event}
	}
}

// saveSessionCmd returns a tea.Cmd that saves the session in the background.
func saveSessionCmd(saveFn func()) tea.Cmd {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if saveFn == nil {
		observe.GlobalTrace("if: saveFn == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	observe.GlobalTrace("return: func() tea.Msg {\n\tsaveFn()\n\treturn sessionSavedMsg{}\n}")
	return func() tea.Msg {
		saveFn()
		return sessionSavedMsg{}
	}
}
