package tui

import (
	"context"
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
	parentCtx      context.Context // original parent context — never overwritten
	ctx            context.Context
	cancel         context.CancelFunc
	interruptCount int
	permQueue      []PermRequestMsg // queued permission requests when dialog is already visible

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
	observe.GlobalTrace("return: Model{\n\tengine:\t\tcfg.Engine,\n\tstore:\t\tcfg.Store,\n\tcostTracker:\tcfg.CostTracke...")
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
		parentCtx:   parentCtx,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Init is the bubbletea initialization command.
func (m Model) Init() tea.Cmd {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: m.input.textarea.Focus()")
	observe.GlobalTrace("return: m.input.textarea.Focus()")
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
		observe.GlobalTrace("return: m, nil")
		observe.GlobalTrace("return: m, nil")
		return m, nil
	}
	if m.perm.active {
		observe.GlobalTrace("if: m.perm.active")
		cmd := m.perm.Update(msg)
		observe.GlobalTrace("return: m, cmd")
		observe.GlobalTrace("return: m, cmd")
		observe.GlobalTrace("return: m, cmd")
		return m, cmd
	}

	cmd := m.input.Update(msg)
	observe.GlobalTrace("return: m, cmd")
	observe.GlobalTrace("return: m, cmd")
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
		observe.GlobalTrace("return: \"Initializing...\"")
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
	observe.GlobalTrace("return: b.String()")
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
	observe.GlobalTrace("return: m, nil")
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
				observe.GlobalTrace("return: m.quit()")
				observe.GlobalTrace("return: m.quit()")
				return m.quit()
			}

			m.cancel()
			observe.GlobalTrace("return: m, nil")
			observe.GlobalTrace("return: m, nil")
			observe.GlobalTrace("return: m, nil")
			return m, nil
		}
		return m.quit()

	case tea.KeyEsc:
		observe.GlobalTrace("case: tea.KeyEsc")
		if m.perm.active {
			cmd := m.perm.Update(msg)
			observe.GlobalTrace("return: m, cmd")
			observe.GlobalTrace("return: m, cmd")
			observe.GlobalTrace("return: m, cmd")
			return m, cmd
		}
		if m.ask.active {
			observe.GlobalTrace("return: m, nil")
			observe.GlobalTrace("return: m, nil")
			observe.GlobalTrace("return: m, nil")

			return m, nil
		}
	}

	if m.perm.active {
		observe.GlobalTrace("if: m.perm.active")
		cmd := m.perm.Update(msg)
		observe.GlobalTrace("return: m, cmd")
		observe.GlobalTrace("return: m, cmd")
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
		observe.GlobalTrace("return: m, cmd")
		observe.GlobalTrace("return: m, cmd")
		return m, cmd
	}

	cmd := m.input.Update(msg)
	observe.GlobalTrace("return: m, cmd")
	observe.GlobalTrace("return: m, cmd")
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
		observe.GlobalTrace("return: m, nil")
		observe.GlobalTrace("return: m, nil")
		return m, nil
	}

	if name, args, ok := slash.Parse(msg.Text); ok {
		observe.GlobalTrace("if: ok")
		observe.GlobalTrace("return: m.handleSlashCommand(name, args)")
		observe.GlobalTrace("return: m.handleSlashCommand(name, args)")
		observe.GlobalTrace("return: m.handleSlashCommand(name, args)")
		return m.handleSlashCommand(name, args)
	}

	if m.hookMgr != nil {
		observe.GlobalTrace("if: m.hookMgr != nil")
		hookResult := m.hookMgr.Execute(m.ctx, hook.UserPromptSubmit, hook.HookInput{
			PromptText: msg.Text,
		})
		if hookResult.Blocked {
			observe.GlobalTrace("if: hookResult.Blocked")
			m.outputBuf.WriteString(errorStyle.Render("Blocked: "+hookResult.BlockMsg) + "\n")
			m.viewport.SetContent(m.outputBuf.String())
			m.viewport.GotoBottom()
			observe.GlobalTrace("return: m, nil")
			observe.GlobalTrace("return: m, nil")
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

	m.cancel()
	m.ctx, m.cancel = context.WithCancel(m.parentCtx)
	m.eventCh = m.engine.Run(m.ctx, msg.Text)

	m.outputBuf.WriteString(assistantLabelStyle.Render("Assistant"))
	m.outputBuf.WriteString("\n")
	observe.GlobalTrace("return: m, waitForEvent(m.eventCh)")
	observe.GlobalTrace("return: m, waitForEvent(m.eventCh)")
	observe.GlobalTrace("return: m, waitForEvent(m.eventCh)")

	return m, waitForEvent(m.eventCh)
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
	observe.GlobalTrace("return: m")
	observe.GlobalTrace("return: m")
	return m
}

// maxOutputBufBytes is the maximum size of the output buffer before trimming.
// Prevents unbounded memory growth in long sessions.
const maxOutputBufBytes = 512 * 1024 // 512KB

// flushStreamBuf moves accumulated streaming text into the permanent output buffer.
func (m Model) flushStreamBuf() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if m.streamBuf.Len() > 0 {
		observe.GlobalTrace("if: m.streamBuf.Len() > 0")
		m.outputBuf.WriteString(m.streamBuf.String())
		m.streamBuf.Reset()
	}
	m.trimOutputBuf()
}

// trimOutputBuf trims the output buffer to maxOutputBufBytes, keeping the tail.
// The viewport only renders visible content, so losing old prefix is invisible to the user.
func (m Model) trimOutputBuf() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if m.outputBuf.Len() <= maxOutputBufBytes {
		observe.GlobalTrace("if: m.outputBuf.Len() <= maxOutputBufBytes")
		return
	}
	content := m.outputBuf.String()

	trimAt := len(content) - maxOutputBufBytes
	idx := strings.IndexByte(content[trimAt:], '\n')
	if idx >= 0 {
		observe.GlobalTrace("if: idx >= 0")
		trimAt += idx + 1
	}
	m.outputBuf.Reset()
	m.outputBuf.WriteString(content[trimAt:])
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
	observe.GlobalTrace("return: m, tea.Quit")
	observe.GlobalTrace("return: m, tea.Quit")
	return m, tea.Quit
}

// waitForEvent returns a tea.Cmd that reads the next event from the channel.
func waitForEvent(ch <-chan query.LoopEvent) tea.Cmd {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: func() tea.Msg {\n\tevent, ok := <-ch\n\tif !ok {\n\t\treturn LoopEventMsg{Event: ni...")
	observe.GlobalTrace("return: func() tea.Msg {\n\tevent, ok := <-ch\n\tif !ok {\n\t\treturn LoopEventMsg{Event: ni...")
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
		observe.GlobalTrace("return: nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	observe.GlobalTrace("return: func() tea.Msg {\n\tsaveFn()\n\treturn sessionSavedMsg{}\n}")
	observe.GlobalTrace("return: func() tea.Msg {\n\tsaveFn()\n\treturn sessionSavedMsg{}\n}")
	observe.GlobalTrace("return: func() tea.Msg {\n\tsaveFn()\n\treturn sessionSavedMsg{}\n}")
	return func() tea.Msg {
		saveFn()
		return sessionSavedMsg{}
	}
}
