package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/query"
)

// Config holds all dependencies for the TUI model.
type Config struct {
	Engine      *query.Engine
	Store       *app.StateStore
	CostTracker *model.CostTracker
	ModelName   string
	Provider    string
	SessionSave func()
}

// Model is the main bubbletea model for the interactive TUI.
type Model struct {
	// Dependencies
	engine      *query.Engine
	store       *app.StateStore
	costTracker *model.CostTracker
	sessionSave func()

	// Components
	viewport   viewport.Model
	input      inputComponent
	perm       permissionDialog
	toolbar    toolbar

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
	ctx, cancel := context.WithCancel(context.Background())

	return Model{
		engine:      cfg.Engine,
		store:       cfg.Store,
		costTracker: cfg.CostTracker,
		sessionSave: cfg.SessionSave,
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
	return m.input.textarea.Focus()
}

// Update handles all messages in the bubbletea event loop.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleResize(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)

	case InputSubmittedMsg:
		return m.handleInputSubmitted(msg)

	case LoopEventMsg:
		return m.handleLoopEvent(msg)

	case PermRequestMsg:
		return m.handlePermRequest(msg)

	case PermResponseMsg:
		return m.handlePermResponse(msg)

	case sessionSavedMsg:
		return m, nil
	}

	// Pass unhandled messages to active components
	if m.perm.active {
		cmd := m.perm.Update(msg)
		return m, cmd
	}

	cmd := m.input.Update(msg)
	return m, cmd
}

// View renders the full TUI layout.
func (m Model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	var b strings.Builder

	// Viewport (scrollable output)
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	// Permission dialog (if active)
	if m.perm.active {
		b.WriteString(m.perm.View())
		b.WriteString("\n")
	}

	// Toolbar
	b.WriteString(m.toolbar.View(m.width))
	b.WriteString("\n")

	// Input
	b.WriteString(m.input.View())

	return b.String()
}

// handleResize adjusts all components to the new terminal size.
func (m Model) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height

	inputHeight := 3  // textarea default height
	toolbarHeight := 1
	headerHeight := inputHeight + toolbarHeight + 2 // +2 for newlines

	vpHeight := max(m.height-headerHeight, 1)

	if !m.ready {
		m.viewport = viewport.New(m.width, vpHeight)
		m.ready = true

		// If resuming a session, render existing conversation
		snap := m.store.Snapshot()
		if len(snap.Conversation.Messages) > 0 {
			m.outputBuf.WriteString(renderConversation(snap.Conversation.Messages))
			m.viewport.SetContent(m.outputBuf.String())
			m.viewport.GotoBottom()
		}
	} else {
		m.viewport.Width = m.width
		m.viewport.Height = vpHeight
	}

	m.input.SetWidth(m.width)
	return m, nil
}

// handleKey processes keyboard input.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		if m.streaming {
			m.interruptCount++
			if m.interruptCount >= 2 {
				return m.quit()
			}
			// First Ctrl+C during streaming: cancel the engine
			m.cancel()
			return m, nil
		}
		return m.quit()

	case tea.KeyEsc:
		if m.perm.active {
			cmd := m.perm.Update(msg)
			return m, cmd
		}
	}

	// Delegate to active component
	if m.perm.active {
		cmd := m.perm.Update(msg)
		return m, cmd
	}

	cmd := m.input.Update(msg)
	return m, cmd
}

// handleInputSubmitted processes user message submission.
func (m Model) handleInputSubmitted(msg InputSubmittedMsg) (tea.Model, tea.Cmd) {
	if m.streaming {
		return m, nil
	}

	// Render user message to viewport
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

	// Start streaming
	m.streaming = true
	m.interruptCount = 0
	m.input.SetActive(false)
	m.toolbar.SetStatus("streaming...")
	m.toolbar.IncrementTurn()

	// Fresh context for this turn
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.eventCh = m.engine.Run(m.ctx, msg.Text)

	// Write assistant label immediately
	m.outputBuf.WriteString(assistantLabelStyle.Render("Assistant"))
	m.outputBuf.WriteString("\n")

	return m, waitForEvent(m.eventCh)
}

// handleLoopEvent processes a streaming event from the query engine.
func (m Model) handleLoopEvent(msg LoopEventMsg) (tea.Model, tea.Cmd) {
	if msg.Event == nil {
		// Channel closed — turn is done (shouldn't normally happen without TurnComplete)
		return m.finishTurn(), nil
	}

	switch e := msg.Event.(type) {
	case query.TextEvent:
		m.streamBuf.WriteString(e.Text)
		m.viewport.SetContent(m.outputBuf.String() + m.streamBuf.String())
		m.viewport.GotoBottom()

	case query.ThinkingEvent:
		m.outputBuf.WriteString(thinkingStyle.Render(e.Text))
		m.viewport.SetContent(m.outputBuf.String() + m.streamBuf.String())
		m.viewport.GotoBottom()

	case query.ToolCallEvent:
		// Flush any pending stream text
		m.flushStreamBuf()
		m.outputBuf.WriteString(renderToolCall(e.Call))
		m.outputBuf.WriteString("\n")
		m.toolbar.SetStatus("executing: " + e.Call.Name)
		m.viewport.SetContent(m.outputBuf.String())
		m.viewport.GotoBottom()

	case query.ToolResultEvent:
		m.outputBuf.WriteString(renderToolResult(e.Result))
		m.outputBuf.WriteString("\n")
		m.toolbar.SetStatus("streaming...")
		m.viewport.SetContent(m.outputBuf.String())
		m.viewport.GotoBottom()

	case query.TurnCompleteEvent:
		m.flushStreamBuf()
		m.outputBuf.WriteString("\n")
		m.toolbar.UpdateCost(m.costTracker.TotalUSD())
		return m.finishTurn(), saveSessionCmd(m.sessionSave)

	case query.ErrorEvent:
		m.flushStreamBuf()
		if m.ctx.Err() != nil {
			// Context cancelled (Ctrl+C) — not an error, just stop
			m.outputBuf.WriteString("\n" + thinkingStyle.Render("[interrupted]") + "\n\n")
		} else {
			m.outputBuf.WriteString("\n" + errorStyle.Render("Error: "+e.Err.Error()) + "\n\n")
		}
		return m.finishTurn(), nil
	}

	return m, waitForEvent(m.eventCh)
}

// handlePermRequest shows the permission dialog.
func (m Model) handlePermRequest(msg PermRequestMsg) (tea.Model, tea.Cmd) {
	m.perm.Show(&msg)
	m.toolbar.SetStatus("waiting for permission...")
	return m, nil
}

// handlePermResponse hides the permission dialog after user decision.
func (m Model) handlePermResponse(_ PermResponseMsg) (tea.Model, tea.Cmd) {
	if m.streaming {
		m.toolbar.SetStatus("streaming...")
	} else {
		m.toolbar.SetStatus("ready")
	}
	return m, nil
}

// finishTurn resets streaming state and re-enables input.
func (m Model) finishTurn() Model {
	m.streaming = false
	m.eventCh = nil
	m.input.SetActive(true)
	m.toolbar.SetStatus("ready")
	m.viewport.SetContent(m.outputBuf.String())
	m.viewport.GotoBottom()
	return m
}

// flushStreamBuf moves accumulated streaming text into the permanent output buffer.
func (m Model) flushStreamBuf() {
	if m.streamBuf.Len() > 0 {
		m.outputBuf.WriteString(m.streamBuf.String())
		m.streamBuf.Reset()
	}
}

// quit saves the session and exits.
func (m Model) quit() (tea.Model, tea.Cmd) {
	m.cancel()
	if m.sessionSave != nil {
		m.sessionSave()
	}
	return m, tea.Quit
}

// waitForEvent returns a tea.Cmd that reads the next event from the channel.
func waitForEvent(ch <-chan query.LoopEvent) tea.Cmd {
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
	if saveFn == nil {
		return nil
	}
	return func() tea.Msg {
		saveFn()
		return sessionSavedMsg{}
	}
}
