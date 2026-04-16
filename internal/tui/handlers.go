package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artpar/pragma/internal/hook"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/slash"
	"github.com/artpar/pragma/internal/tui/render"
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
	m.outputBuf.WriteString(userLabelStyle.Render("❯") + " " + label + "\n\n")
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
	// Ignore stale events after an interrupt (interruptTurn already called finishTurn).
	if !m.streaming {
		observe.GlobalTrace("if: !m.streaming — ignoring stale event")
		return m, nil
	}
	if msg.Event == nil {
		observe.GlobalTrace("if: msg.Event == nil")
		finished, cmd := m.finishTurn()
		return finished, cmd
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
		finished, pendingCmd := m.finishTurn()
		return finished, tea.Batch(saveSessionCmd(m.sessionSave), pendingCmd)

	case query.ErrorEvent:
		observe.GlobalTrace("typecase: query.ErrorEvent")
		m.flushStreamBuf()
		m.spinnerActive = false
		if m.ctx.Err() != nil {
			m.outputBuf.WriteString("\n" + thinkingStyle.Render("[interrupted]") + "\n\n")
		} else {
			m.outputBuf.WriteString("\n" + errorStyle.Render("Error: "+e.Err.Error()) + "\n\n")
		}
		finished, pendingCmd := m.finishTurn()
		return finished, pendingCmd
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
	m.input.SetStreaming(true)
	m.toolbar.SetStatus("streaming...")
	m.toolbar.IncrementTurn()

	m.cancel()
	m.ctx, m.cancel = context.WithCancel(m.parentCtx)
	m.eventCh = m.engine.Run(m.ctx, prompt)

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

	// Any non-Ctrl+C key resets the quit pending state and restores toolbar.
	if msg.Type != tea.KeyCtrlC && m.quitPending {
		m.quitPending = false
		if m.streaming {
			m.toolbar.SetStatus("streaming...")
		} else {
			m.toolbar.SetStatus("ready")
		}
	}

	switch msg.Type {
	case tea.KeyCtrlC:
		observe.GlobalTrace("case: tea.KeyCtrlC")
		if m.streaming {
			// Interrupt current turn immediately.
			return m.interruptTurn()
		}
		// Idle: double-press to exit. First press shows warning, second quits.
		// Input text is never cleared — preserves user work (see issue #5817).
		if m.quitPending {
			observe.GlobalTrace("return: m.quit()")
			return m.quit()
		}
		m.quitPending = true
		m.toolbar.SetStatus("press Ctrl+C again to exit")
		return m, nil

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
		// Esc during streaming interrupts, matching pragma.
		if m.streaming {
			return m.interruptTurn()
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

	switch msg.Type {
	case tea.KeyPgUp, tea.KeyPgDown:
		observe.GlobalTrace("scroll key → viewport")
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	case tea.KeyHome:
		observe.GlobalTrace("Home → viewport top")
		m.viewport.GotoTop()
		return m, nil
	case tea.KeyEnd:
		observe.GlobalTrace("End → viewport bottom")
		m.viewport.GotoBottom()
		return m, nil
	case tea.KeyUp:
		observe.GlobalTrace("case: tea.KeyUp")
		if msg.Alt {
			observe.GlobalTrace("Alt+Up → viewport line up")
			m.viewport.ScrollUp(1)
			observe.GlobalTrace("return: m, nil")
			return m, nil
		}
	case tea.KeyDown:
		observe.GlobalTrace("case: tea.KeyDown")
		if msg.Alt {
			observe.GlobalTrace("Alt+Down → viewport line down")
			m.viewport.ScrollDown(1)
			observe.GlobalTrace("return: m, nil")
			return m, nil
		}
	}

	cmd := m.input.Update(msg)
	observe.GlobalTrace("return: m, cmd")
	return m, cmd
}

// handleInputSubmitted processes user message submission.
// If streaming, queues the message to auto-submit after the current turn.
func (m Model) handleInputSubmitted(msg InputSubmittedMsg) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if m.streaming {
		observe.GlobalTrace("if: m.streaming — queuing message")
		m.pendingInput = msg.Text
		label := msg.Text
		if len(label) > 40 {
			label = label[:37] + "..."
		}
		m.toolbar.SetStatus("queued: " + label)
		return m, nil
	}

	return m.submitPrompt(msg.Text)
}

// submitPrompt sends a user prompt to the engine and starts streaming.
func (m Model) submitPrompt(text string) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	if name, args, ok := slash.Parse(text); ok {
		observe.GlobalTrace("if: ok")
		observe.GlobalTrace("return: m.handleSlashCommand(name, args)")
		return m.handleSlashCommand(name, args)
	}

	if m.hookMgr != nil {
		observe.GlobalTrace("if: m.hookMgr != nil")
		hookResult := m.hookMgr.Execute(m.ctx, hook.UserPromptSubmit, hook.HookInput{
			PromptText: text,
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
			model.TextPart{Text: text},
		},
	}
	m.outputBuf.WriteString(render.RenderMessage(userMsg, m.mdRenderer))
	m.viewport.SetContent(m.outputBuf.String())
	m.viewport.GotoBottom()

	m.streaming = true
	m.input.SetStreaming(true)
	m.toolbar.SetStatus("streaming...")
	m.toolbar.IncrementTurn()

	m.cancel()
	m.ctx, m.cancel = context.WithCancel(m.parentCtx)
	m.eventCh = m.engine.Run(m.ctx, text)

	observe.GlobalTrace("return: m, waitForEvent(m.eventCh)")

	return m, waitForEvent(m.eventCh)
}

// finishTurn resets streaming state. If a message was queued during streaming,
// it returns a tea.Cmd to auto-submit it as the next turn.
func (m Model) finishTurn() (Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.streaming = false
	m.input.SetStreaming(false)
	m.spinnerActive = false
	m.eventCh = nil
	m.toolbar.SetStatus("ready")
	m.viewport.SetContent(m.viewportContent())
	m.viewport.GotoBottom()

	// Chain queued message if present.
	if m.pendingInput != "" {
		observe.GlobalTrace("if: m.pendingInput != \"\"")
		text := m.pendingInput
		m.pendingInput = ""
		return m, func() tea.Msg {
			return InputSubmittedMsg{Text: text}
		}
	}
	observe.GlobalTrace("return: m, nil")
	return m, nil
}

// interruptTurn cancels the streaming context, shows an immediate "Interrupted" message,
// and finishes the turn. Matches pragma's Esc/Ctrl+C behavior.
func (m Model) interruptTurn() (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.cancel()
	m.flushStreamBuf()
	m.spinnerActive = false
	m.outputBuf.WriteString("\n" + render.BracketPrefix + thinkingStyle.Render("Interrupted · What should pragma do instead?") + "\n\n")
	finished, cmd := m.finishTurn()
	return finished, cmd
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
