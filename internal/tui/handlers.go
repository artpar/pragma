package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artpar/gogent/internal/hook"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/query"
	"github.com/artpar/gogent/internal/slash"
	"github.com/artpar/gogent/internal/tui/render"
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
	observe.GlobalTrace("return: m, func() tea.Msg {\n\tresult, err := slashCmds.Execute(m.ctx, name, trimmedArg...")
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
		if msg.Result.InjectPrompt != "" {
			observe.GlobalTrace("if: msg.Result.InjectPrompt != \"\"")
			observe.GlobalTrace("return: m.startEngineFromPrompt(msg.Result.InjectPrompt)")
			return m.startEngineFromPrompt(msg.Result.InjectPrompt)
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
		m.viewport.SetContent(m.viewportContent())
		m.viewport.GotoBottom()

	case query.TextEvent:
		observe.GlobalTrace("typecase: query.TextEvent")
		m.streamBuf.WriteString(e.Text)

		raw := m.streamBuf.String()
		boundary := strings.LastIndex(raw, "\n\n")
		if boundary > 0 {
			complete := raw[:boundary+2]
			pending := raw[boundary+2:]
			rendered := m.mdRenderer.Render(complete)
			m.outputBuf.WriteString(rendered + "\n")
			m.streamBuf.Reset()
			m.streamBuf.WriteString(pending)
		}
		m.viewport.SetContent(m.viewportContent())
		m.viewport.GotoBottom()

	case query.ThinkingEvent:
		observe.GlobalTrace("typecase: query.ThinkingEvent")
		m.flushStreamBuf()
		m.outputBuf.WriteString(render.RenderThinking(model.ThinkingPart{Text: e.Text}) + "\n")
		m.viewport.SetContent(m.viewportContent())
		m.viewport.GotoBottom()

	case query.ToolCallEvent:
		observe.GlobalTrace("typecase: query.ToolCallEvent")
		m.flushStreamBuf()
		m.outputBuf.WriteString(render.RenderToolCall(e.Call, m.width))
		m.outputBuf.WriteString("\n")

		m.activeToolCalls[e.Call.ID] = e.Call

		m.spinnerActive = true
		m.spinnerTool = e.Call.Name
		m.toolbar.SetStatus("executing: " + e.Call.Name)
		m.viewport.SetContent(m.viewportContent())
		m.viewport.GotoBottom()
		return m, tea.Batch(waitForEvent(m.eventCh), m.spin.Tick)

	case query.ToolResultEvent:
		observe.GlobalTrace("typecase: query.ToolResultEvent")

		m.spinnerActive = false

		call, ok := m.activeToolCalls[e.Result.ToolCallID]
		if ok {
			m.outputBuf.WriteString(render.RenderToolOutput(call.Name, call.Input, e.Result.Content, e.Result.IsError, m.width))
			delete(m.activeToolCalls, e.Result.ToolCallID)
		} else {
			m.outputBuf.WriteString(render.WrapWithBracket(e.Result.Content, e.Result.IsError, m.width))
		}
		m.outputBuf.WriteString("\n")
		m.toolbar.SetStatus("streaming...")
		m.viewport.SetContent(m.viewportContent())
		m.viewport.GotoBottom()

	case query.TurnCompleteEvent:
		observe.GlobalTrace("typecase: query.TurnCompleteEvent")
		m.flushStreamBuf()
		if e.StopReason == model.StopMaxTokens {
			m.outputBuf.WriteString("\n" + thinkingStyle.Render("[response truncated — hit max_tokens limit]") + "\n")
		}
		m.outputBuf.WriteString("\n")
		m.toolbar.UpdateCost(m.costTracker.TotalUSD())

		if m.tokenMonitor != nil {
			in, out := m.tokenMonitor.Usage()
			m.toolbar.UpdateTokens(in, out, m.tokenMonitor.Budget())
		}
		return m.finishTurn(), saveSessionCmd(m.sessionSave)

	case query.ErrorEvent:
		observe.GlobalTrace("typecase: query.ErrorEvent")
		m.flushStreamBuf()
		m.spinnerActive = false
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

// startEngineFromPrompt submits a prompt to the engine as a user message.
func (m Model) startEngineFromPrompt(prompt string) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	m.viewport.SetContent(m.outputBuf.String())
	m.viewport.GotoBottom()

	m.streaming = true
	m.interruptCount = 0
	m.input.SetActive(false)
	m.toolbar.SetStatus("streaming...")
	m.toolbar.IncrementTurn()

	m.cancel()
	m.ctx, m.cancel = context.WithCancel(m.parentCtx)
	m.eventCh = m.engine.Run(m.ctx, prompt)

	m.outputBuf.WriteString(assistantLabelStyle.Render("Assistant"))
	m.outputBuf.WriteString("\n")
	observe.GlobalTrace("return: m, waitForEvent(m.eventCh)")

	return m, waitForEvent(m.eventCh)
}

// handlePermRequest shows the permission dialog, or queues if one is already visible.
func (m Model) handlePermRequest(msg PermRequestMsg) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if m.perm.active {
		observe.GlobalTrace("if: m.perm.active")
		m.permQueue = append(m.permQueue, msg)
		observe.GlobalTrace("return: m, nil")
		return m, nil
	}
	m.perm.Show(&msg)
	m.toolbar.SetStatus("waiting for permission...")
	observe.GlobalTrace("return: m, nil")
	return m, nil
}

// handlePermResponse hides the permission dialog after user decision.
func (m Model) handlePermResponse(_ PermResponseMsg) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(m.permQueue) > 0 {
		observe.GlobalTrace("if: len(m.permQueue) > 0")
		next := m.permQueue[0]
		m.permQueue = m.permQueue[1:]
		m.perm.Show(&next)
		observe.GlobalTrace("return: m, nil")
		return m, nil
	}
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
	m.outputBuf.WriteString(render.RenderMessage(userMsg, m.mdRenderer))
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

	return m, waitForEvent(m.eventCh)
}

// finishTurn resets streaming state and re-enables input.
func (m Model) finishTurn() Model {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.streaming = false
	m.spinnerActive = false
	m.eventCh = nil
	m.input.SetActive(true)
	m.toolbar.SetStatus("ready")
	m.viewport.SetContent(m.viewportContent())
	m.viewport.GotoBottom()
	observe.GlobalTrace("return: m")
	return m
}

// maxOutputBufBytes is the maximum size of the output buffer before trimming.
const maxOutputBufBytes = 512 * 1024 // 512KB

// flushStreamBuf moves accumulated streaming text into the permanent output buffer.
// Renders remaining text through markdown before flushing.
func (m Model) flushStreamBuf() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if m.streamBuf.Len() > 0 {
		observe.GlobalTrace("if: m.streamBuf.Len() > 0")
		rendered := m.mdRenderer.Render(m.streamBuf.String())
		m.outputBuf.WriteString(rendered + "\n")
		m.streamBuf.Reset()
	}
	m.trimOutputBuf()
}

// trimOutputBuf trims the output buffer to maxOutputBufBytes, keeping the tail.
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
