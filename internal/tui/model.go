package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/interactive"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/slash"
	"github.com/artpar/pragma/internal/tui/render"
)

// Config holds all dependencies for the TUI model.
type Config struct {
	ParentCtx      context.Context // parent context for cancellation propagation (e.g., cmd.Context())
	RunInput       func(context.Context, string) <-chan interactive.Event
	Resume         func(sessionID string) error
	CloseSession   func() error
	Store          *app.StateStore
	CostTracker    *model.CostTracker
	ModelName      string
	Provider       string
	SlashCmds      *slash.Registry
	SlashDeps      slash.Deps
	TokenMonitor   *observe.TokenMonitor // nil if no token monitoring
	Metrics        *observe.Metrics      // always non-nil (created in deps.go)
	Workspace      string                // full workspace directory path (run.go passes d.Cwd)
	Version        string                // build version (from buildinfo.Version)
	StartedAt      time.Time             // original session start (for resume elapsed time)
	McpServerNames []string              // connected MCP server names for welcome banner
	PromptHistory  []string              // input history seeded from saved sessions
}

// segmentKind distinguishes text (pre-rendered) from thinking/tool (rendered on demand).
type segmentKind int

const (
	segText segmentKind = iota
	segThinking
	segTool  // raw tool result data, rendered on demand based on verbose
	segAgent // agent progress, updated in-place based on AgentProgressEvent (ADR-043)
	segGroup // collapsed read/search group, accumulates consecutive collapsible tools (ADR-044)
	segError // classified error with optional retry state, rendered on demand based on verbose
)

// toolSegData holds raw tool result data for on-demand rendering.
// Stored in segTool segments; re-rendered when verbose toggles.
type toolSegData struct {
	Name    string
	Input   json.RawMessage
	Content string
	IsError bool
}

// agentEntry tracks one agent's progress within a segAgent segment.
type agentEntry struct {
	AgentID     string
	Description string
	ToolCount   int
	TokenCount  int
	LastTool    string
	Status      string // "initializing", "running", "completed", "error"
	Background  bool
	Error       string
}

// agentSegData holds progress for all agents in a tool batch.
// Multiple agents are grouped into one segment because the orchestrator
// executes them in the same batch (serial for Agent since Concurrent=false).
type agentSegData struct {
	Agents []*agentEntry  // ordered by first appearance
	byID   map[string]int // AgentID → index in Agents
}

// isCollapsible returns the category of a tool for grouping purposes.
// Returns "" for non-collapsible tools that break groups.
// Categories: "read" (Read), "search" (Grep, Glob), "silent" (ToolSearch — absorbed, no count).
func isCollapsible(name string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch name {
	case "Read":
		observe.GlobalTrace("case: \"Read\"")
		return "read"
	case "Grep":
		observe.GlobalTrace("case: \"Grep\"")
		return "search"
	case "Glob":
		observe.GlobalTrace("case: \"Glob\"")
		return "search"
	case "ToolSearch":
		observe.GlobalTrace("case: \"ToolSearch\"")
		return "silent"
	default:
		observe.GlobalTrace("default")
		return ""
	}
}

// groupEntry holds one tool call + result pair within a collapsed group.
type groupEntry struct {
	CallHeader string      // pre-rendered "⏺ Read(file.go)\n"
	Tool       toolSegData // tool result data (filled on ToolResultEvent)
	Category   string      // "read", "search", "silent"
	CallID     string      // tool call ID for matching results to calls
	HasResult  bool        // false while waiting for ToolResultEvent
}

// groupSegData holds accumulated collapsible tool operations for on-demand rendering.
// Consecutive Read/Grep/Glob calls are grouped into a single segGroup segment.
// Non-verbose: shows summary badge. Verbose: shows individual tool calls with results.
type groupSegData struct {
	Entries     []groupEntry
	SearchCount int             // Grep + Glob count
	ReadPaths   map[string]bool // deduped Read file paths (for unique file count)
	Active      bool            // true while still accumulating (streaming)
	LatestHint  string          // last file path or "pattern" for display hint
}

// errorSegData holds classified error data for on-demand rendering.
// Stored in segError segments; re-rendered when verbose toggles or countdown ticks.
type errorSegData struct {
	Kind        string // ErrorKind as string (render pkg doesn't import query/)
	ErrorMsg    string // full error text (may be long)
	Guidance    string // actionable hint
	Attempt     int
	MaxAttempts int
	SecondsLeft int  // countdown (updated by tick)
	Retrying    bool // true while waiting for retry delay
}

// segment is a typed chunk of viewport output. Text segments are pre-rendered;
// thinking and tool segments store raw data and are rendered based on verbose.
type segment struct {
	kind      segmentKind
	content   string        // segText: pre-rendered; segThinking: raw thinking text
	redacted  bool          // only meaningful for segThinking
	forceShow bool          // TUI-001: thinking promoted for a textless final response renders expanded
	tool      *toolSegData  // only meaningful for segTool
	agent     *agentSegData // only meaningful for segAgent
	group     *groupSegData // only meaningful for segGroup (ADR-044)
	errData   *errorSegData // only meaningful for segError
}

// Model is the main bubbletea model for the interactive TUI.
type Model struct {
	// Dependencies
	runInput       func(context.Context, string) <-chan interactive.Event
	resume         func(sessionID string) error
	closeSession   func() error
	store          *app.StateStore
	costTracker    *model.CostTracker
	slashCmds      *slash.Registry
	slashDeps      slash.Deps
	tokenMonitor   *observe.TokenMonitor
	metrics        *observe.Metrics
	version        string   // build version for welcome display
	workspace      string   // full workspace path for welcome display
	mcpServerNames []string // connected MCP server names for welcome banner

	// Components
	viewport  viewport.Model
	input     inputComponent
	perm      permissionDialog
	modelDlg  modelDialog
	resumeDlg resumeDialog
	toolbar   toolbar
	spin      spinner.Model

	// Rendering
	mdRenderer      *render.MarkdownRenderer
	activeToolCalls map[string]model.ToolCallPart // correlate ToolCallEvent → ToolResultEvent

	// Spinner state
	spinnerActive bool
	spinnerTool   string

	// Streaming state
	outputSegs   []segment        // typed segments for viewport content
	verbose      bool             // Ctrl+O toggles verbose mode: thinking expanded + tool results full output
	streamBuf    *strings.Builder // current streaming text (not yet finalized)
	eventCh      <-chan interactive.Event
	streaming    bool
	parentCtx    context.Context // original parent context — never overwritten
	ctx          context.Context
	cancel       context.CancelFunc
	quitPending  bool             // true after idle Ctrl+C, waiting for second to quit
	permQueue    []PermRequestMsg // queued permission requests when dialog is already visible
	retryAttempt int              // generation counter for stale countdown tick detection
	// finalResponseHadText tracks whether the current request within the turn
	// emitted visible text (TUI-001). Reset at turn start and on every
	// ToolResultEvent (a new request boundary); read at TurnComplete.
	finalResponseHadText bool

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
	observe.GlobalTrace("return: Model{...}")

	tb := newToolbar(cfg.ModelName, cfg.Provider, cfg.Workspace, cfg.StartedAt)

	tb.UpdateCost(cfg.CostTracker.TotalUSD())
	msnap := cfg.Metrics.Snapshot()
	cache := msnap.TokenUsage.CacheCreationInputTokens + msnap.TokenUsage.CacheReadInputTokens
	budget := 0
	if cfg.TokenMonitor != nil {
		observe.GlobalTrace("if: cfg.TokenMonitor != nil")
		budget = cfg.TokenMonitor.Budget()
	}
	tb.UpdateTokens(msnap.TokenUsage.InputTokens, msnap.TokenUsage.OutputTokens, cache, budget, msnap.LatestContextFill)
	observe.GlobalTrace("return: Model{...}")

	input := newInputComponent()
	if len(cfg.PromptHistory) > 0 {
		observe.GlobalTrace("if: len(cfg.PromptHistory) > 0")
		input.SetHistory(cfg.PromptHistory)
	}
	observe.GlobalTrace("return: Model{\n\trunInput:\t\tcfg.RunInput,\n\tresume:\t\t\tcfg.Resume,\n\tcloseSession:\t\tcfg.C...")

	return Model{
		runInput:        cfg.RunInput,
		resume:          cfg.Resume,
		closeSession:    cfg.CloseSession,
		store:           cfg.Store,
		costTracker:     cfg.CostTracker,
		slashCmds:       cfg.SlashCmds,
		slashDeps:       cfg.SlashDeps,
		tokenMonitor:    cfg.TokenMonitor,
		metrics:         cfg.Metrics,
		version:         cfg.Version,
		workspace:       cfg.Workspace,
		mcpServerNames:  cfg.McpServerNames,
		input:           input,
		perm:            newPermissionDialog(),
		toolbar:         tb,
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

	case tea.MouseMsg:
		observe.GlobalTrace("typecase: tea.MouseMsg")
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

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

	case quitTimeoutMsg:
		observe.GlobalTrace("typecase: quitTimeoutMsg")
		if m.quitPending {
			m.quitPending = false
			if m.streaming {
				observe.GlobalTrace("if: m.streaming")
				m.toolbar.SetStatus("streaming...")
			} else {
				observe.GlobalTrace("else: m.streaming")
				m.toolbar.SetStatus("ready")
			}
		}
		return m, nil

	case retryCountdownMsg:
		observe.GlobalTrace("typecase: retryCountdownMsg")

		if msg.Attempt != m.retryAttempt {
			observe.GlobalTrace("return: m, nil")
			return m, nil
		}

		for i := len(m.outputSegs) - 1; i >= 0; i-- {
			if m.outputSegs[i].kind == segError && m.outputSegs[i].errData != nil && m.outputSegs[i].errData.Retrying {
				observe.GlobalTrace("if: m.outputSegs[i].kind == segError && m.outputSegs[i].errData != nil && m.outpu...")
				m.outputSegs[i].errData.SecondsLeft = msg.SecondsLeft
				break
			}
		}
		m.toolbar.SetStatus(fmt.Sprintf("retrying in %ds...", max(0, msg.SecondsLeft)))
		m.viewport.SetContent(m.viewportContent())
		m.viewport.GotoBottom()
		if msg.SecondsLeft > 0 {
			observe.GlobalTrace("return: m, tea.Tick(time.Second, func(t time.Time) tea.Msg {\n\treturn retryCountdownMs...")
			return m, tea.Tick(time.Second, func(t time.Time) tea.Msg {
				return retryCountdownMsg{SecondsLeft: msg.SecondsLeft - 1, Attempt: msg.Attempt}
			})
		}
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

	b.WriteString("\n" + sep + "\n")

	if m.perm.active {
		observe.GlobalTrace("if: m.perm.active")
		b.WriteString(m.perm.View())
		b.WriteString("\n")
	}

	b.WriteString(m.input.View())

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
		observe.GlobalTrace("range m.outputSegs")
		switch seg.kind {
		case segText:
			observe.GlobalTrace("case: segText")
			b.WriteString(seg.content)
		case segThinking:
			observe.GlobalTrace("case: segThinking")
			b.WriteString(render.RenderThinking(model.ThinkingPart{
				Text: seg.content, Redacted: seg.redacted,
			}, m.verbose || seg.forceShow) + "\n")
		case segTool:
			observe.GlobalTrace("case: segTool")
			b.WriteString(render.RenderToolOutput(
				seg.tool.Name, seg.tool.Input, seg.tool.Content,
				seg.tool.IsError, m.width, m.verbose,
			))
			b.WriteString("\n")
		case segAgent:
			observe.GlobalTrace("case: segAgent")
			if seg.agent != nil {
				entries := convertAgentEntries(seg.agent.Agents)
				b.WriteString(render.RenderAgentProgress(entries, m.verbose, m.width))
				b.WriteString("\n")
			}
		case segGroup:
			observe.GlobalTrace("case: segGroup")
			if seg.group != nil {
				data := convertGroupData(seg.group)
				b.WriteString(render.RenderToolGroup(data, m.verbose, m.width))
				b.WriteString("\n")
			}
		case segError:
			observe.GlobalTrace("case: segError")
			if seg.errData != nil {
				b.WriteString(render.RenderError(render.ErrorData{
					Kind:        seg.errData.Kind,
					ErrorMsg:    seg.errData.ErrorMsg,
					Guidance:    seg.errData.Guidance,
					Attempt:     seg.errData.Attempt,
					MaxAttempts: seg.errData.MaxAttempts,
					SecondsLeft: seg.errData.SecondsLeft,
					Retrying:    seg.errData.Retrying,
				}, m.verbose, m.width))
				b.WriteString("\n")
			}
		}
	}
	b.WriteString(m.streamBuf.String())
	if m.spinnerActive {
		observe.GlobalTrace("if: m.spinnerActive")
		b.WriteString("\n" + m.spin.View() + " " + m.spinnerTool + "...")
	}

	if m.modelDlg.active {
		observe.GlobalTrace("if: m.modelDlg.active")
		b.WriteString("\n")
		b.WriteString(m.modelDlg.View(m.width))
	}

	if m.resumeDlg.active {
		observe.GlobalTrace("if: m.resumeDlg.active")
		b.WriteString("\n")
		b.WriteString(m.resumeDlg.View(m.width))
	}
	observe.GlobalTrace("return: b.String()")
	return b.String()
}

// appendText appends pre-rendered text to segments, merging into last text segment.
// Returns the updated slice — caller must assign: m.outputSegs = appendText(m.outputSegs, s)
func appendText(segs []segment, s string) []segment {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if n := len(segs); n > 0 && segs[n-1].kind == segText {
		observe.GlobalTrace("if: n > 0 && segs[n-1].kind == segText")
		segs[n-1].content += s
		observe.GlobalTrace("return: segs")
		return segs
	}
	observe.GlobalTrace("return: append(segs, segment{kind: segText, content: s})")
	return append(segs, segment{kind: segText, content: s})
}

// appendThinking appends a raw thinking block to segments.
// Returns the updated slice — caller must assign.
func appendThinking(segs []segment, text string, redacted bool) []segment {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: append(segs, segment{kind: segThinking, content: text, redacted: redacted})")
	return append(segs, segment{kind: segThinking, content: text, redacted: redacted})
}

// promoteTrailingThinking marks the trailing run of thinking segments
// forceShow (TUI-001): a final response that emitted no text otherwise
// renders as a single collapsed hint, hiding the turn's entire output from a
// default-mode operator. Trailing whitespace-only text is skipped; any text
// (non-whitespace), tool, group, agent, or error segment behind the run
// stops the scan, so only the final response's thinking is promoted.
func promoteTrailingThinking(segs []segment) []segment {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	i := len(segs) - 1
	for i >= 0 && segs[i].kind == segText && strings.TrimSpace(segs[i].content) == "" {
		observe.GlobalTrace("for: i >= 0 && segs[i].kind == segText && strings.TrimSpace(segs[i].content) == \"\"")
		i--
	}
	if i < 0 || segs[i].kind != segThinking {
		observe.GlobalTrace("if: i < 0 || segs[i].kind != segThinking")
		observe.GlobalTrace("return: segs")
		return segs
	}
	for ; i >= 0 && segs[i].kind == segThinking; i-- {
		observe.GlobalTrace("for: i >= 0 && segs[i].kind == segThinking")
		segs[i].forceShow = true
	}
	observe.GlobalTrace("return: segs")
	return segs
}

// isThinkingOnlyAssistant reports whether msg is an assistant message whose
// entire content is thinking blocks — the persisted shape of a TUI-001 final
// response. Used on conversation load to surface the missed instruction.
func isThinkingOnlyAssistant(msg model.Message) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if msg.Role != model.RoleAssistant || len(msg.Content) == 0 {
		observe.GlobalTrace("if: msg.Role != model.RoleAssistant || len(msg.Content) == 0")
		observe.GlobalTrace("return: false")
		return false
	}
	for _, part := range msg.Content {
		observe.GlobalTrace("range msg.Content")
		if _, ok := part.(model.ThinkingPart); !ok {
			observe.GlobalTrace("if: !ok")
			observe.GlobalTrace("return: false")
			return false
		}
	}
	observe.GlobalTrace("return: true")
	return true
}

// appendTool appends a raw tool result to segments for on-demand rendering.
// Returns the updated slice — caller must assign.
func appendTool(segs []segment, data toolSegData) []segment {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: append(segs, segment{kind: segTool, tool: &data})")
	return append(segs, segment{kind: segTool, tool: &data})
}

// appendError appends a classified error to segments for on-demand rendering.
// Returns the updated slice — caller must assign.
func appendError(segs []segment, data errorSegData) []segment {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: append(segs, segment{kind: segError, errData: &data})")
	return append(segs, segment{kind: segError, errData: &data})
}

// hasProgressSegment returns true if a recent segAgent segment already covers
// this tool's visual output. This prevents creating a duplicate segTool when
// the progress segment is the authoritative display.
func hasProgressSegment(segs []segment, toolName string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for i := len(segs) - 1; i >= 0; i-- {
		observe.GlobalTrace("for: i >= 0")
		switch {
		case segs[i].kind == segAgent && segs[i].agent != nil && toolName == "Agent":
			observe.GlobalTrace("case: segs[i].kind == segAgent && segs[i].agent != nil && toolName == \"Agent\"")
			return true
		case segs[i].kind == segText:
			observe.GlobalTrace("case: segs[i].kind == segText")
			continue
		default:
			observe.GlobalTrace("default")
			return false
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

// updateAgentProgress finds or creates the active segAgent segment
// and updates it in-place based on the agent progress event.
func (m *Model) updateAgentProgress(e query.AgentProgressEvent) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var data *agentSegData
	for i := len(m.outputSegs) - 1; i >= 0; i-- {
		observe.GlobalTrace("for: i >= 0")
		if m.outputSegs[i].kind == segAgent && m.outputSegs[i].agent != nil {
			observe.GlobalTrace("if: m.outputSegs[i].kind == segAgent && m.outputSegs[i].agent != nil")
			data = m.outputSegs[i].agent
			break
		}
	}
	if data == nil {
		observe.GlobalTrace("if: data == nil")
		data = &agentSegData{byID: make(map[string]int)}
		m.outputSegs = append(m.outputSegs, segment{kind: segAgent, agent: data})
	}

	idx, ok := data.byID[e.AgentID]
	if !ok {
		observe.GlobalTrace("if: !ok")
		idx = len(data.Agents)
		data.Agents = append(data.Agents, &agentEntry{AgentID: e.AgentID})
		data.byID[e.AgentID] = idx
	}
	entry := data.Agents[idx]
	entry.Description = e.Description
	entry.ToolCount = e.ToolCount
	entry.TokenCount = e.TokenCount
	entry.LastTool = e.LastTool
	entry.Status = e.Status
	entry.Background = e.Background
	entry.Error = e.Error
}

// convertAgentEntries converts internal agentEntry slice to render types.
func convertAgentEntries(agents []*agentEntry) []render.AgentProgressEntry {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	result := make([]render.AgentProgressEntry, len(agents))
	for i, a := range agents {
		observe.GlobalTrace("range agents")
		result[i] = render.AgentProgressEntry{
			AgentID:     a.AgentID,
			Description: a.Description,
			ToolCount:   a.ToolCount,
			TokenCount:  a.TokenCount,
			LastTool:    a.LastTool,
			Status:      a.Status,
			Background:  a.Background,
			Error:       a.Error,
		}
	}
	observe.GlobalTrace("return: result")
	return result
}

// closeActiveGroup marks the last active segGroup as no longer accumulating.
// Called before any group-breaking event (text, thinking, non-collapsible tool, turn complete, error).
func (m *Model) closeActiveGroup() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for i := len(m.outputSegs) - 1; i >= 0; i-- {
		observe.GlobalTrace("for: i >= 0")
		seg := &m.outputSegs[i]
		if seg.kind == segGroup && seg.group != nil && seg.group.Active {
			observe.GlobalTrace("if: seg.kind == segGroup && seg.group != nil && seg.group.Active")
			seg.group.Active = false
			return
		}
	}
}

// addToGroup finds or creates the active segGroup and appends a new entry for a collapsible tool call.
func (m *Model) addToGroup(callHeader string, call model.ToolCallPart, category string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var g *groupSegData
	for i := len(m.outputSegs) - 1; i >= 0; i-- {
		observe.GlobalTrace("for: i >= 0")
		if m.outputSegs[i].kind == segGroup && m.outputSegs[i].group != nil && m.outputSegs[i].group.Active {
			observe.GlobalTrace("if: m.outputSegs[i].kind == segGroup && m.outputSegs[i].group != nil && m.outputS...")
			g = m.outputSegs[i].group
			break
		}
	}
	if g == nil {
		observe.GlobalTrace("if: g == nil")
		g = &groupSegData{ReadPaths: make(map[string]bool), Active: true}
		m.outputSegs = append(m.outputSegs, segment{kind: segGroup, group: g})
	}
	g.Entries = append(g.Entries, groupEntry{
		CallHeader: callHeader,
		Category:   category,
		CallID:     call.ID,
		Tool:       toolSegData{Name: call.Name, Input: call.Input},
	})
}

// fillGroupResult fills the result data into the matching entry of the active group by call ID.
func (m *Model) fillGroupResult(call model.ToolCallPart, result model.ToolResultPart) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for i := len(m.outputSegs) - 1; i >= 0; i-- {
		observe.GlobalTrace("for: i >= 0")
		seg := &m.outputSegs[i]
		if seg.kind == segGroup && seg.group != nil && seg.group.Active {
			observe.GlobalTrace("if: seg.kind == segGroup && seg.group != nil && seg.group.Active")
			g := seg.group
			for j := range g.Entries {
				observe.GlobalTrace("range g.Entries")
				if g.Entries[j].CallID == call.ID && !g.Entries[j].HasResult {
					observe.GlobalTrace("if: g.Entries[j].CallID == call.ID")
					g.Entries[j].Tool.Content = result.Content
					g.Entries[j].Tool.IsError = result.IsError
					g.Entries[j].HasResult = true
					updateGroupCounts(g, &g.Entries[j], call.Input)
					return
				}
			}
			return
		}
	}
}

// updateGroupCounts increments the appropriate counter on a group based on the entry's category.
func updateGroupCounts(g *groupSegData, entry *groupEntry, input json.RawMessage) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch entry.Category {
	case "search":
		observe.GlobalTrace("case: \"search\"")
		g.SearchCount++
		var p struct {
			Pattern string `json:"pattern"`
		}
		json.Unmarshal(input, &p)
		if p.Pattern != "" {
			g.LatestHint = `"` + p.Pattern + `"`
		}
	case "read":
		observe.GlobalTrace("case: \"read\"")
		var p struct {
			FilePath string `json:"file_path"`
		}
		json.Unmarshal(input, &p)
		if p.FilePath != "" {
			g.ReadPaths[p.FilePath] = true
			g.LatestHint = p.FilePath
		}

	}
}

// convertGroupData converts internal groupSegData to render package types.
func convertGroupData(g *groupSegData) render.GroupData {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	entries := make([]render.GroupEntry, len(g.Entries))
	for i, e := range g.Entries {
		observe.GlobalTrace("range g.Entries")
		entries[i] = render.GroupEntry{
			CallHeader: e.CallHeader,
			Name:       e.Tool.Name,
			Input:      e.Tool.Input,
			Content:    e.Tool.Content,
			IsError:    e.Tool.IsError,
			HasResult:  e.HasResult,
		}
	}
	observe.GlobalTrace("return: render.GroupData{\n\tEntries:\tentries,\n\tSearchCount:\tg.SearchCount,\n\tReadCount:...")
	return render.GroupData{
		Entries:     entries,
		SearchCount: g.SearchCount,
		ReadCount:   len(g.ReadPaths),
		Active:      g.Active,
		LatestHint:  g.LatestHint,
	}
}

// segByteSize returns the estimated byte size of a single segment.
func segByteSize(seg segment) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch {
	case seg.kind == segTool && seg.tool != nil:
		observe.GlobalTrace("case: seg.kind == segTool && seg.tool != nil")
		return len(seg.tool.Content) + len(seg.tool.Input)
	case seg.kind == segAgent && seg.agent != nil:
		observe.GlobalTrace("case: seg.kind == segAgent && seg.agent != nil")
		return len(seg.agent.Agents) * 80
	case seg.kind == segGroup && seg.group != nil:
		observe.GlobalTrace("case: seg.kind == segGroup && seg.group != nil")
		size := 0
		for _, e := range seg.group.Entries {
			size += len(e.CallHeader) + len(e.Tool.Content) + len(e.Tool.Input)
		}
		return size
	case seg.kind == segError && seg.errData != nil:
		observe.GlobalTrace("case: seg.kind == segError && seg.errData != nil")
		return len(seg.errData.ErrorMsg)
	default:
		observe.GlobalTrace("default")
		return len(seg.content)
	}
}

// outputLen returns the total content length across all segments.
func outputLen(segs []segment) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	n := 0
	for _, seg := range segs {
		observe.GlobalTrace("range segs")
		n += segByteSize(seg)
	}
	observe.GlobalTrace("return: n")
	return n
}

// loadMessageSegments loads a single message into output segments.
// Thinking parts become thinking segments; everything else becomes text.
// Consecutive collapsible tools (Read, Grep, Glob) are grouped into segGroup segments.
func loadMessageSegments(segs []segment, msg model.Message, md *render.MarkdownRenderer) []segment {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if msg.Role == model.RoleUser {
		observe.GlobalTrace("if: msg.Role == model.RoleUser")
		rendered := render.RenderMessage(msg, md)
		if rendered != "" {
			observe.GlobalTrace("if: rendered != \"\"")
			segs = appendText(segs, rendered)
		}
		observe.GlobalTrace("return: segs")
		return segs
	}
	// Assistant messages: split thinking/tool parts into segments for on-demand rendering.
	// Track last ToolCallPart to pair with its ToolResultPart into segTool segments.
	// Consecutive collapsible tools accumulate into a groupSegData.
	var lastCall *model.ToolCallPart
	var activeGroup *groupSegData

	flushGroup := func() {
		if activeGroup != nil {
			segs = append(segs, segment{kind: segGroup, group: activeGroup})
			activeGroup = nil
		}
	}

	for _, part := range msg.Content {
		observe.GlobalTrace("range msg.Content")
		switch p := part.(type) {
		case model.ThinkingPart:
			observe.GlobalTrace("typecase: model.ThinkingPart")
			flushGroup()
			segs = appendThinking(segs, p.Text, p.Redacted)
		case model.ToolCallPart:
			observe.GlobalTrace("typecase: model.ToolCallPart")
			cat := isCollapsible(p.Name)
			if cat != "" {

				if activeGroup == nil {
					observe.GlobalTrace("if: activeGroup == nil")
					activeGroup = &groupSegData{ReadPaths: make(map[string]bool)}
				}
				activeGroup.Entries = append(activeGroup.Entries, groupEntry{
					CallHeader: render.RenderToolCall(p, 80) + "\n",
					Category:   cat,
					CallID:     p.ID,
					Tool:       toolSegData{Name: p.Name, Input: p.Input},
				})
			} else {

				flushGroup()
				segs = appendText(segs, render.RenderToolCall(p, 80)+"\n")
			}
			call := p
			lastCall = &call
		case model.ToolResultPart:
			observe.GlobalTrace("typecase: model.ToolResultPart")
			if lastCall != nil && activeGroup != nil && isCollapsible(lastCall.Name) != "" {

				entry := &activeGroup.Entries[len(activeGroup.Entries)-1]
				entry.Tool.Content = p.Content
				entry.Tool.IsError = p.IsError
				entry.HasResult = true
				updateGroupCounts(activeGroup, entry, lastCall.Input)
				lastCall = nil
			} else if lastCall != nil {
				flushGroup()
				segs = appendTool(segs, toolSegData{
					Name:    lastCall.Name,
					Input:   lastCall.Input,
					Content: p.Content,
					IsError: p.IsError,
				})
				lastCall = nil
			} else {
				flushGroup()
				segs = appendText(segs, render.RenderToolResultGeneric(p, 80)+"\n")
			}
		default:
			observe.GlobalTrace("typedefault")
			flushGroup()
			rendered := render.RenderContentPart(part, md, 80)
			if rendered != "" {
				segs = appendText(segs, rendered+"\n")
			}
		}
	}
	flushGroup()
	segs = appendText(segs, "\n")
	observe.GlobalTrace("return: segs")
	return segs
}

// handleResize adjusts all components to the new terminal size.
func (m Model) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.width = msg.Width
	m.height = msg.Height

	inputHeight := m.input.ViewHeight()
	toolbarHeight := 1
	separatorHeight := 2
	headerHeight := inputHeight + toolbarHeight + separatorHeight

	vpHeight := max(m.height-headerHeight, 1)

	m.mdRenderer.UpdateWidth(m.width)

	if !m.ready {
		observe.GlobalTrace("if: !m.ready")
		m.viewport = viewport.New(m.width, vpHeight)
		m.ready = true

		welcomeText := render.RenderWelcome(
			m.version, m.toolbar.modelName, m.toolbar.provider,
			m.workspace, m.mcpServerNames, m.width,
		)
		m.outputSegs = append(m.outputSegs, segment{kind: segText, content: welcomeText})

		snap := m.store.Snapshot()
		if len(snap.Conversation.Messages) > 0 {
			observe.GlobalTrace("if: len(snap.Conversation.Messages) > 0")

			m.outputSegs = appendText(m.outputSegs, "\n"+thinkingStyle.Render(
				render.TeardropAsterisk+" Resuming conversation")+"\n\n")
			for _, msg := range snap.Conversation.Messages {
				observe.GlobalTrace("range snap.Conversation.Messages")
				m.outputSegs = loadMessageSegments(m.outputSegs, msg, m.mdRenderer)
			}

			if n := len(snap.Conversation.Messages); n > 0 && isThinkingOnlyAssistant(snap.Conversation.Messages[n-1]) {
				observe.GlobalTrace("if: n > 0 && isThinkingOnlyAssistant(snap.Conversation.Messages[n-1])")
				m.outputSegs = promoteTrailingThinking(m.outputSegs)
			}
			m.input.SetHistory(promptHistoryFromSnapshot(snap))
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

func (m *Model) syncViewportHeight() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !m.ready {
		observe.GlobalTrace("if: !m.ready")
		return
	}
	headerHeight := m.input.ViewHeight() + 1 + 2
	m.viewport.Height = max(m.height-headerHeight, 1)
}
