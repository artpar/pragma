package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/hook"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/slash"
	"github.com/artpar/pragma/internal/tui/render"
)

// Config holds all dependencies for the TUI model.
type Config struct {
	ParentCtx    context.Context // parent context for cancellation propagation (e.g., cmd.Context())
	Engine       *query.Engine
	Store        *app.StateStore
	CostTracker  *model.CostTracker
	ModelName    string
	Provider     string
	SessionSave  func()
	SlashCmds    *slash.Registry
	SlashDeps    slash.Deps
	HookMgr      *hook.Manager         // nil if no hooks configured
	TokenMonitor *observe.TokenMonitor // nil if no token monitoring
	Workspace    string                // workspace directory basename
}

// Model is the main bubbletea model for the interactive TUI.
type Model struct {
	// Dependencies
	engine       *query.Engine
	store        *app.StateStore
	costTracker  *model.CostTracker
	sessionSave  func()
	slashCmds    *slash.Registry
	slashDeps    slash.Deps
	hookMgr      *hook.Manager
	tokenMonitor *observe.TokenMonitor

	// Components
	viewport viewport.Model
	input    inputComponent
	perm     permissionDialog
	ask      askDialog
	toolbar  toolbar
	spin     spinner.Model

	// Rendering
	mdRenderer      *render.MarkdownRenderer
	activeToolCalls map[string]model.ToolCallPart // correlate ToolCallEvent → ToolResultEvent

	// Spinner state
	spinnerActive bool
	spinnerTool   string

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

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "130", Dark: "214"})
	observe.GlobalTrace("return: Model{\n\tengine:\t\t\tcfg.Engine,\n\tstore:\t\t\tcfg.Store,\n\tcostTracker:\t\tcfg.CostTra...")

	return Model{
		engine:          cfg.Engine,
		store:           cfg.Store,
		costTracker:     cfg.CostTracker,
		sessionSave:     cfg.SessionSave,
		slashCmds:       cfg.SlashCmds,
		slashDeps:       cfg.SlashDeps,
		hookMgr:         cfg.HookMgr,
		tokenMonitor:    cfg.TokenMonitor,
		input:           newInputComponent(),
		perm:            newPermissionDialog(),
		toolbar:         newToolbar(cfg.ModelName, cfg.Provider, cfg.Workspace),
		spin:            s,
		mdRenderer:      render.NewMarkdownRenderer(80),
		activeToolCalls: make(map[string]model.ToolCallPart),
		outputBuf:       &strings.Builder{},
		streamBuf:       &strings.Builder{},
		parentCtx:       parentCtx,
		ctx:             ctx,
		cancel:          cancel,
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

	case spinner.TickMsg:
		observe.GlobalTrace("typecase: spinner.TickMsg")
		if m.spinnerActive {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)

			m.viewport.SetContent(m.viewportContent())
			observe.GlobalTrace("return: m, cmd")
			return m, cmd
		}
		return m, nil

	case sessionSavedMsg:
		observe.GlobalTrace("typecase: sessionSavedMsg")
		return m, nil
	}

	if m.ask.active {
		observe.GlobalTrace("if: m.ask.active")
		cmd := m.ask.Update(msg)
		observe.GlobalTrace("return: m, cmd")
		return m, cmd
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

// viewportContent returns the full viewport content including spinner.
func (m Model) viewportContent() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	content := m.outputBuf.String() + m.streamBuf.String()
	if m.spinnerActive {
		observe.GlobalTrace("if: m.spinnerActive")
		content += "\n" + m.spin.View() + " " + m.spinnerTool + "..."
	}
	observe.GlobalTrace("return: content")
	return content
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

	m.mdRenderer.UpdateWidth(m.width)

	if !m.ready {
		observe.GlobalTrace("if: !m.ready")
		m.viewport = viewport.New(m.width, vpHeight)
		m.ready = true

		snap := m.store.Snapshot()
		if len(snap.Conversation.Messages) > 0 {
			observe.GlobalTrace("if: len(snap.Conversation.Messages) > 0")
			m.outputBuf.WriteString(render.RenderConversation(snap.Conversation.Messages, m.mdRenderer))
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
