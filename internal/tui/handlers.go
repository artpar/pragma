package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/query"
)

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
	return m, func() tea.Msg {
		result, err := slashCmds.Execute(m.ctx, name, trimmedArgs, slashDeps)
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
	return m, nil
}

// handleLoopEvent processes a streaming event from the query engine.
func (m Model) handleLoopEvent(msg LoopEventMsg) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if msg.Event == nil {
		observe.GlobalTrace("if: msg.Event == nil")
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

	return m, waitForEvent(m.eventCh)
}

// handleAskRequest shows the ask dialog for a tool question.
func (m Model) handleAskRequest(msg AskRequestMsg) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.ask.Show(&msg)
	m.toolbar.SetStatus("waiting for answer...")
	return m, nil
}

// handlePermRequest shows the permission dialog, or queues if one is already visible.
func (m Model) handlePermRequest(msg PermRequestMsg) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if m.perm.active {
		observe.GlobalTrace("if: m.perm.active — queuing permission request")
		m.permQueue = append(m.permQueue, msg)
		return m, nil
	}
	m.perm.Show(&msg)
	m.toolbar.SetStatus("waiting for permission...")
	return m, nil
}

// handlePermResponse hides the permission dialog after user decision.
// If queued requests exist, shows the next one immediately.
func (m Model) handlePermResponse(_ PermResponseMsg) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(m.permQueue) > 0 {
		observe.GlobalTrace("if: len(m.permQueue) > 0 — showing next queued request")
		next := m.permQueue[0]
		m.permQueue = m.permQueue[1:]
		m.perm.Show(&next)
		return m, nil
	}
	if m.streaming {
		observe.GlobalTrace("if: m.streaming")
		m.toolbar.SetStatus("streaming...")
	} else {
		observe.GlobalTrace("else: m.streaming")
		m.toolbar.SetStatus("ready")
	}
	return m, nil
}
