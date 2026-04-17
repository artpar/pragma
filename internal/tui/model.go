package tui

import (
	"context"
	"encoding/json"
	"strings"
	"time"

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

// segmentKind distinguishes text (pre-rendered) from thinking/tool (rendered on demand).
type segmentKind int

const (
	segText      segmentKind = iota
	segThinking
	segTool      // raw tool result data, rendered on demand based on verbose
	segLifecycle // lifecycle progress, updated in-place based on events
)

// toolSegData holds raw tool result data for on-demand rendering.
// Stored in segTool segments; re-rendered when verbose toggles.
type toolSegData struct {
	Name    string
	Input   json.RawMessage
	Content string
	IsError bool
	Display string
}

// lifecycleNodeResult holds the outcome of a single node execution.
type lifecycleNodeResult struct {
	Duration time.Duration
	Error    string
}

// lifecycleStep holds progress for a single superstep in a lifecycle graph.
type lifecycleStep struct {
	Step        int
	Nodes       []string
	Results     map[string]lifecycleNodeResult // node → result
	Transitions []lifecycleTransition          // edges traversed after this step
	Status      string                         // "running", "completed"
}

// lifecycleTransition records an edge traversal in the graph.
type lifecycleTransition struct {
	From     string
	To       string
	RouteKey string
}

// lifecycleSegData holds accumulated lifecycle progress for on-demand rendering.
type lifecycleSegData struct {
	Steps     []lifecycleStep
	Completed bool
	Error     string
}

// segment is a typed chunk of viewport output. Text segments are pre-rendered;
// thinking and tool segments store raw data and are rendered based on verbose.
type segment struct {
	kind      segmentKind
	content   string           // segText: pre-rendered; segThinking: raw thinking text
	redacted  bool             // only meaningful for segThinking
	tool      *toolSegData     // only meaningful for segTool
	lifecycle *lifecycleSegData // only meaningful for segLifecycle
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
	outputSegs       []segment        // typed segments for viewport content
	verbose bool             // Ctrl+O toggles verbose mode: thinking expanded + tool results full output
	streamBuf        *strings.Builder // current streaming text (not yet finalized)
	eventCh          <-chan query.LoopEvent
	streaming        bool
	parentCtx        context.Context // original parent context — never overwritten
	ctx              context.Context
	cancel           context.CancelFunc
	pendingInput     string           // queued message to submit after current turn completes
	quitPending      bool             // true after idle Ctrl+C, waiting for second to quit
	permQueue        []PermRequestMsg // queued permission requests when dialog is already visible

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

	case quitTimeoutMsg:
		observe.GlobalTrace("typecase: quitTimeoutMsg")
		if m.quitPending {
			m.quitPending = false
			if m.streaming {
				m.toolbar.SetStatus("streaming...")
			} else {
				m.toolbar.SetStatus("ready")
			}
		}
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
// Layout (top to bottom): viewport ── input ── toolbar
func (m Model) View() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !m.ready {
		observe.GlobalTrace("if: !m.ready")
		observe.GlobalTrace("return: \"Initializing...\"")
		return "Initializing..."
	}

	sep := strings.Repeat("─", m.width)
	var b strings.Builder

	b.WriteString(m.viewport.View())
	// \n terminates the last viewport line; sep fills the next line
	b.WriteString("\n" + sep + "\n")

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

	b.WriteString(m.input.View())
	// \n terminates the last input line; sep fills the next line
	b.WriteString("\n" + sep + "\n")
	b.WriteString(m.toolbar.View(m.width))
	observe.GlobalTrace("return: b.String()")

	return b.String()
}

// viewportContent returns the full viewport content including spinner.
// Thinking segments are rendered on demand based on verbose state.
// Shows a welcome message when the conversation is empty.
func (m Model) viewportContent() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var b strings.Builder
	for _, seg := range m.outputSegs {
		switch seg.kind {
		case segText:
			b.WriteString(seg.content)
		case segThinking:
			b.WriteString(render.RenderThinking(model.ThinkingPart{
				Text: seg.content, Redacted: seg.redacted,
			}, m.verbose) + "\n")
		case segTool:
			b.WriteString(render.RenderToolOutput(
				seg.tool.Name, seg.tool.Input, seg.tool.Content,
				seg.tool.IsError, m.width, seg.tool.Display, m.verbose,
			))
			b.WriteString("\n")
		case segLifecycle:
			if seg.lifecycle != nil {
				rSteps := convertLifecycleSteps(seg.lifecycle.Steps)
				b.WriteString(render.RenderLifecycleProgress(rSteps, seg.lifecycle.Completed, seg.lifecycle.Error, m.verbose, m.width))
				b.WriteString("\n")
			}
		}
	}
	b.WriteString(m.streamBuf.String())
	if m.spinnerActive {
		observe.GlobalTrace("if: m.spinnerActive")
		b.WriteString("\n" + m.spin.View() + " " + m.spinnerTool + "...")
	}
	if b.Len() == 0 {
		observe.GlobalTrace("if: b.Len() == 0")
		return welcomeMessage
	}
	observe.GlobalTrace("return: b.String()")
	return b.String()
}

// appendText appends pre-rendered text to segments, merging into last text segment.
// Returns the updated slice — caller must assign: m.outputSegs = appendText(m.outputSegs, s)
func appendText(segs []segment, s string) []segment {
	if n := len(segs); n > 0 && segs[n-1].kind == segText {
		segs[n-1].content += s
		return segs
	}
	return append(segs, segment{kind: segText, content: s})
}

// appendThinking appends a raw thinking block to segments.
// Returns the updated slice — caller must assign.
func appendThinking(segs []segment, text string, redacted bool) []segment {
	return append(segs, segment{kind: segThinking, content: text, redacted: redacted})
}

// appendTool appends a raw tool result to segments for on-demand rendering.
// Returns the updated slice — caller must assign.
func appendTool(segs []segment, data toolSegData) []segment {
	return append(segs, segment{kind: segTool, tool: &data})
}

// updateLifecycleProgress finds or creates the active segLifecycle segment
// and updates it in-place based on the lifecycle progress event.
func (m *Model) updateLifecycleProgress(e query.LifecycleProgressEvent) {
	// Find the last segLifecycle segment, or create one.
	var data *lifecycleSegData
	for i := len(m.outputSegs) - 1; i >= 0; i-- {
		if m.outputSegs[i].kind == segLifecycle && m.outputSegs[i].lifecycle != nil {
			data = m.outputSegs[i].lifecycle
			break
		}
	}
	if data == nil {
		data = &lifecycleSegData{}
		m.outputSegs = append(m.outputSegs, segment{kind: segLifecycle, lifecycle: data})
	}

	switch e.Status {
	case "step_started":
		data.Steps = append(data.Steps, lifecycleStep{
			Step:    e.Step,
			Nodes:   e.Nodes,
			Results: make(map[string]lifecycleNodeResult),
			Status:  "running",
		})
	case "node_completed":
		if len(data.Steps) > 0 {
			step := &data.Steps[len(data.Steps)-1]
			step.Results[e.Node] = lifecycleNodeResult{
				Duration: e.Duration,
				Error:    e.Error,
			}
			// Mark step completed when all nodes have results
			if len(step.Results) == len(step.Nodes) {
				step.Status = "completed"
			}
		}
	case "transition":
		if len(data.Steps) > 0 {
			step := &data.Steps[len(data.Steps)-1]
			step.Transitions = append(step.Transitions, lifecycleTransition{
				From:     e.FromNode,
				To:       e.ToNode,
				RouteKey: e.RouteKey,
			})
		}
	case "completed":
		data.Completed = true
		data.Error = e.Error
	}
}

// segByteSize returns the estimated byte size of a single segment.
func segByteSize(seg segment) int {
	switch {
	case seg.kind == segTool && seg.tool != nil:
		return len(seg.tool.Content) + len(seg.tool.Input) + len(seg.tool.Display)
	case seg.kind == segLifecycle && seg.lifecycle != nil:
		return len(seg.lifecycle.Steps) * 50
	default:
		return len(seg.content)
	}
}

// outputLen returns the total content length across all segments.
func outputLen(segs []segment) int {
	n := 0
	for _, seg := range segs {
		n += segByteSize(seg)
	}
	return n
}

// convertLifecycleSteps converts internal lifecycleStep types to render package types.
func convertLifecycleSteps(steps []lifecycleStep) []render.LifecycleStep {
	out := make([]render.LifecycleStep, len(steps))
	for i, s := range steps {
		results := make(map[string]render.LifecycleNodeResult, len(s.Results))
		for k, v := range s.Results {
			results[k] = render.LifecycleNodeResult{Duration: v.Duration, Error: v.Error}
		}
		transitions := make([]render.LifecycleTransition, len(s.Transitions))
		for j, t := range s.Transitions {
			transitions[j] = render.LifecycleTransition{From: t.From, To: t.To, RouteKey: t.RouteKey}
		}
		out[i] = render.LifecycleStep{
			Step:        s.Step,
			Nodes:       s.Nodes,
			Results:     results,
			Transitions: transitions,
			Status:      s.Status,
		}
	}
	return out
}

// loadMessageSegments loads a single message into output segments.
// Thinking parts become thinking segments; everything else becomes text.
func loadMessageSegments(segs []segment, msg model.Message, md *render.MarkdownRenderer) []segment {
	if msg.Role == model.RoleUser {
		rendered := render.RenderMessage(msg, md)
		if rendered != "" {
			segs = appendText(segs, rendered)
		}
		return segs
	}
	// Assistant messages: split thinking/tool parts into segments for on-demand rendering.
	// Track last ToolCallPart to pair with its ToolResultPart into segTool segments.
	var lastCall *model.ToolCallPart
	for _, part := range msg.Content {
		switch p := part.(type) {
		case model.ThinkingPart:
			segs = appendThinking(segs, p.Text, p.Redacted)
		case model.ToolCallPart:
			segs = appendText(segs, render.RenderToolCall(p, 80)+"\n")
			call := p
			lastCall = &call
		case model.ToolResultPart:
			if lastCall != nil {
				segs = appendTool(segs, toolSegData{
					Name:    lastCall.Name,
					Input:   lastCall.Input,
					Content: p.Content,
					IsError: p.IsError,
					Display: "", // Display is TUI-only, not persisted
				})
				lastCall = nil
			} else {
				segs = appendText(segs, render.RenderToolResultGeneric(p, 80)+"\n")
			}
		default:
			rendered := render.RenderContentPart(part, md, 80)
			if rendered != "" {
				segs = appendText(segs, rendered+"\n")
			}
		}
	}
	segs = appendText(segs, "\n")
	return segs
}

// welcomeMessage is shown in the viewport when the conversation is empty.
var welcomeMessage = lipgloss.NewStyle().Faint(true).Render(
	"\n  pragma\n  Type a message and press Enter · Alt+Enter for newlines · /help for commands\n",
)

// handleResize adjusts all components to the new terminal size.
func (m Model) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.width = msg.Width
	m.height = msg.Height

	inputHeight := 3
	toolbarHeight := 1
	separatorHeight := 2 // two ─ separator lines (above input, above toolbar)
	headerHeight := inputHeight + toolbarHeight + separatorHeight

	vpHeight := max(m.height-headerHeight, 1)

	m.mdRenderer.UpdateWidth(m.width)

	if !m.ready {
		observe.GlobalTrace("if: !m.ready")
		m.viewport = viewport.New(m.width, vpHeight)
		m.ready = true

		snap := m.store.Snapshot()
		if len(snap.Conversation.Messages) > 0 {
			observe.GlobalTrace("if: len(snap.Conversation.Messages) > 0")
			for _, msg := range snap.Conversation.Messages {
				m.outputSegs = loadMessageSegments(m.outputSegs, msg, m.mdRenderer)
			}
		}
		m.viewport.SetContent(m.viewportContent())
		m.viewport.GotoBottom()
	} else {
		observe.GlobalTrace("else: !m.ready")
		m.viewport.Width = m.width
		m.viewport.Height = vpHeight
	}

	m.input.SetWidth(m.width)
	observe.GlobalTrace("return: m, nil")
	return m, nil
}
