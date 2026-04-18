package tui

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

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
	m.outputSegs = appendText(m.outputSegs, userLabelStyle.Render("❯")+" "+label+"\n\n")
	m.viewport.SetContent(m.viewportContent())
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
		m.outputSegs = appendText(m.outputSegs, errorStyle.Render("Error: "+msg.Err.Error())+"\n\n")
	} else {
		observe.GlobalTrace("else: msg.Err != nil")
		if msg.Result.Quit {
			observe.GlobalTrace("if: msg.Result.Quit")
			observe.GlobalTrace("return: m.quit()")
			return m.quit()
		}
		if msg.Result.ClearConversation {
			observe.GlobalTrace("if: msg.Result.ClearConversation")
			m.outputSegs = m.outputSegs[:0]
		}
		if msg.Result.DisplayText != "" {
			observe.GlobalTrace("if: msg.Result.DisplayText != \"\"")
			rendered := m.mdRenderer.Render(msg.Result.DisplayText)
			m.outputSegs = appendText(m.outputSegs, rendered+"\n\n")
		}
		if msg.Result.InjectPrompt != "" {
			observe.GlobalTrace("if: msg.Result.InjectPrompt != \"\"")
			observe.GlobalTrace("return: m.startEngineFromPrompt(msg.Result.InjectPrompt)")
			return m.startEngineFromPrompt(msg.Result.InjectPrompt)
		}
		if msg.Result.ShowTeamsDialog {
			observe.GlobalTrace("if: msg.Result.ShowTeamsDialog")
			m.teams.Show(m.taskReg)
		}
	}
	m.viewport.SetContent(m.viewportContent())
	m.viewport.GotoBottom()
	observe.GlobalTrace("return: m, nil")
	return m, nil
}

// handleLoopEvent processes a streaming event from the query engine.
func (m Model) handleLoopEvent(msg LoopEventMsg) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	if !m.streaming {
		observe.GlobalTrace("if: !m.streaming — ignoring stale event")
		observe.GlobalTrace("return: m, nil")
		return m, nil
	}
	if msg.Event == nil {
		observe.GlobalTrace("if: msg.Event == nil")
		finished, cmd := m.finishTurn()
		observe.GlobalTrace("return: finished, cmd")
		return finished, cmd
	}

	switch e := msg.Event.(type) {
	case query.CompactionStartedEvent:
		observe.GlobalTrace("typecase: query.CompactionStartedEvent")
		m.toolbar.SetStatus("compacting conversation...")

	case query.CompactionEvent:
		observe.GlobalTrace("typecase: query.CompactionEvent")
		m.closeActiveGroup()
		m.outputSegs = m.flushStreamBuf()

		m.outputSegs = appendText(m.outputSegs, "\n"+thinkingStyle.Render(
			render.TeardropAsterisk+" Conversation compacted (ctrl+o for history)")+"\n\n")
		m.toolbar.SetStatus("streaming...")
		m.viewport.SetContent(m.viewportContent())
		m.viewport.GotoBottom()

	case query.CompactionDisabledEvent:
		observe.GlobalTrace("typecase: query.CompactionDisabledEvent")
		m.closeActiveGroup()
		m.outputSegs = m.flushStreamBuf()

		m.outputSegs = appendText(m.outputSegs, "\n"+errorStyle.Render(
			fmt.Sprintf("%s Auto-compaction disabled after %d consecutive failures — context will not be compacted",
				render.BlackCircle, e.ConsecutiveFailures))+"\n\n")
		m.toolbar.SetStatus("streaming...")
		m.viewport.SetContent(m.viewportContent())
		m.viewport.GotoBottom()

	case query.TextEvent:
		observe.GlobalTrace("typecase: query.TextEvent")
		m.closeActiveGroup()
		m.streamBuf.WriteString(e.Text)

		raw := m.streamBuf.String()
		boundary := strings.LastIndex(raw, "\n\n")
		if boundary > 0 {
			complete := raw[:boundary+2]
			pending := raw[boundary+2:]
			rendered := m.mdRenderer.Render(complete)
			m.outputSegs = appendText(m.outputSegs, rendered+"\n")
			m.streamBuf.Reset()
			m.streamBuf.WriteString(pending)
		}
		m.viewport.SetContent(m.viewportContent())
		m.viewport.GotoBottom()

	case query.ThinkingEvent:
		observe.GlobalTrace("typecase: query.ThinkingEvent")
		m.closeActiveGroup()
		m.outputSegs = m.flushStreamBuf()
		m.outputSegs = appendThinking(m.outputSegs, e.Text, false)
		m.viewport.SetContent(m.viewportContent())
		m.viewport.GotoBottom()

	case query.LifecycleProgressEvent:
		observe.GlobalTrace("typecase: query.LifecycleProgressEvent")
		m.closeActiveGroup()
		m.outputSegs = m.flushStreamBuf()
		m.updateLifecycleProgress(e)

		switch e.Status {
		case "step_started":
			if len(e.Nodes) > 0 {
				m.toolbar.SetStatus(fmt.Sprintf("lifecycle: step %d — %s", e.Step, strings.Join(e.Nodes, ", ")))
			}
		case "node_completed":
			m.toolbar.SetStatus(fmt.Sprintf("lifecycle: %s completed", e.Node))
		case "completed":
			m.toolbar.SetStatus("streaming...")
		}
		m.viewport.SetContent(m.viewportContent())
		m.viewport.GotoBottom()

	case query.AgentProgressEvent:
		observe.GlobalTrace("typecase: query.AgentProgressEvent")
		m.closeActiveGroup()
		m.outputSegs = m.flushStreamBuf()
		m.updateAgentProgress(e)
		switch e.Status {
		case "initializing":
			status := "agent: " + truncateToolbar(e.Description, 40) + " initializing…"
			if e.Background {
				status = "agent: " + truncateToolbar(e.Description, 40) + " (background)"
			}
			m.toolbar.SetStatus(status)
		case "running":
			if e.LastTool != "" {
				m.toolbar.SetStatus("agent: " + truncateToolbar(e.Description, 30) + " — " + e.LastTool)
			}
		case "completed", "error":
			m.toolbar.SetStatus("streaming...")
		}
		m.viewport.SetContent(m.viewportContent())
		m.viewport.GotoBottom()

	case query.ToolCallEvent:
		observe.GlobalTrace("typecase: query.ToolCallEvent")
		m.outputSegs = m.flushStreamBuf()

		category := isCollapsible(e.Call.Name)
		if category != "" {

			callHeader := render.RenderToolCall(e.Call, m.width) + "\n"
			m.addToGroup(callHeader, e.Call, category)
		} else {

			m.closeActiveGroup()
			m.outputSegs = appendText(m.outputSegs, render.RenderToolCall(e.Call, m.width))
			m.outputSegs = appendText(m.outputSegs, "\n")
		}

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
			delete(m.activeToolCalls, e.Result.ToolCallID)
			if isCollapsible(call.Name) != "" {
				observe.GlobalTrace("if: isCollapsible(call.Name) != \"\"")

				m.fillGroupResult(call, e.Result, e.Display)
			} else if hasProgressSegment(m.outputSegs, call.Name) {
				observe.GlobalTrace("else-if: hasProgressSegment — skip segTool")
			} else {
				observe.GlobalTrace("else: append segTool")
				m.outputSegs = appendTool(m.outputSegs, toolSegData{
					Name:    call.Name,
					Input:   call.Input,
					Content: e.Result.Content,
					IsError: e.Result.IsError,
					Display: e.Display,
				})
				m.outputSegs = appendText(m.outputSegs, "\n")
			}
		} else {
			m.outputSegs = appendText(m.outputSegs, render.WrapWithBracket(e.Result.Content, e.Result.IsError, m.width, false))
			m.outputSegs = appendText(m.outputSegs, "\n")
		}
		m.toolbar.SetStatus("streaming...")
		m.viewport.SetContent(m.viewportContent())
		m.viewport.GotoBottom()

	case query.TurnCompleteEvent:
		observe.GlobalTrace("typecase: query.TurnCompleteEvent")
		m.closeActiveGroup()
		m.outputSegs = m.flushStreamBuf()
		if e.StopReason == model.StopMaxTokens {
			m.outputSegs = appendText(m.outputSegs, "\n"+thinkingStyle.Render("[response truncated — hit max_tokens limit]")+"\n")
		}
		if e.StopReason == model.StopContentFiltered {
			m.outputSegs = appendText(m.outputSegs, "\n"+thinkingStyle.Render("[response blocked by content filter — you can rephrase and try again]")+"\n")
		}
		if e.StopReason == model.StopError {
			m.outputSegs = appendText(m.outputSegs, "\n"+thinkingStyle.Render("[response ended due to a provider error — you can try again or switch models with /model]")+"\n")
		}
		m.outputSegs = appendText(m.outputSegs, "\n")
		m.toolbar.UpdateCost(m.costTracker.TotalUSD())

		snap := m.metrics.Snapshot()
		cache := snap.TokenUsage.CacheCreationInputTokens + snap.TokenUsage.CacheReadInputTokens
		budget := 0
		if m.tokenMonitor != nil {
			budget = m.tokenMonitor.Budget()
		}
		m.toolbar.UpdateTokens(snap.TokenUsage.InputTokens, snap.TokenUsage.OutputTokens, cache, budget, snap.LatestContextFill)
		finished, pendingCmd := m.finishTurn()
		return finished, tea.Batch(saveSessionCmd(m.sessionSave), pendingCmd)

	case query.RetryEvent:
		observe.GlobalTrace("typecase: query.RetryEvent")
		m.closeActiveGroup()
		m.outputSegs = m.flushStreamBuf()

		// Hidden retries: toolbar only, no viewport segment.
		// Matches TS: retryAttempt < 4 returns null — hides attempts 1,2,3; shown from attempt 4.
		const hiddenRetryThreshold = 4
		if e.Attempt < hiddenRetryThreshold {
			m.toolbar.SetStatus(fmt.Sprintf("retrying... (attempt %d/%d)", e.Attempt, e.MaxAttempts))
			observe.GlobalTrace("return: m, waitForEvent(m.eventCh)")
			return m, waitForEvent(m.eventCh)
		}

		secondsLeft := int(math.Ceil(e.Delay.Seconds()))
		m.retryAttempt = e.Attempt
		data := errorSegData{
			Kind:        string(e.Kind),
			ErrorMsg:    e.ErrorMsg,
			Attempt:     e.Attempt,
			MaxAttempts: e.MaxAttempts,
			SecondsLeft: secondsLeft,
			Retrying:    true,
		}
		m.outputSegs = appendError(m.outputSegs, data)
		m.toolbar.SetStatus(fmt.Sprintf("retrying in %ds... (attempt %d/%d)", secondsLeft, e.Attempt, e.MaxAttempts))
		m.viewport.SetContent(m.viewportContent())
		m.viewport.GotoBottom()

		return m, tea.Batch(
			waitForEvent(m.eventCh),
			tea.Tick(time.Second, func(t time.Time) tea.Msg {
				return retryCountdownMsg{SecondsLeft: secondsLeft - 1, Attempt: e.Attempt}
			}),
		)

	case query.ErrorEvent:
		observe.GlobalTrace("typecase: query.ErrorEvent")
		m.closeActiveGroup()
		m.outputSegs = m.flushStreamBuf()
		m.spinnerActive = false
		m.retryAttempt = 0

		if m.ctx.Err() != nil {

			m.outputSegs = appendText(m.outputSegs, "\n"+thinkingStyle.Render("[interrupted]")+"\n\n")
		} else if e.Kind != "" {

			data := errorSegData{
				Kind:     string(e.Kind),
				ErrorMsg: e.Err.Error(),
				Guidance: e.Guidance,
			}
			m.outputSegs = appendError(m.outputSegs, data)
			m.outputSegs = appendText(m.outputSegs, "\n")
		} else {

			m.outputSegs = appendText(m.outputSegs, "\n"+errorStyle.Render("Error: "+e.Err.Error())+"\n\n")
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

	m.viewport.SetContent(m.viewportContent())
	m.viewport.GotoBottom()

	m.streaming = true
	m.input.SetStreaming(true)
	m.toolbar.SetStatus("streaming...")

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

	if msg.Type != tea.KeyCtrlC && m.quitPending {
		observe.GlobalTrace("if: msg.Type != tea.KeyCtrlC && m.quitPending")
		m.quitPending = false
		if m.streaming {
			observe.GlobalTrace("if: m.streaming")
			m.toolbar.SetStatus("streaming...")
		} else {
			observe.GlobalTrace("else: m.streaming")
			m.toolbar.SetStatus("ready")
		}
	}

	if m.teams.active && msg.Type != tea.KeyCtrlC {
		observe.GlobalTrace("if: m.teams.active && msg.Type != tea.KeyCtrlC")
		m.teams.Update(msg)
		m.viewport.SetContent(m.viewportContent())
		observe.GlobalTrace("return: m, nil")
		return m, nil
	}

	switch msg.Type {
	case tea.KeyCtrlC:
		observe.GlobalTrace("case: tea.KeyCtrlC")
		if m.streaming {
			observe.GlobalTrace("return: m.interruptTurn()")

			return m.interruptTurn()
		}

		if m.quitPending {
			observe.GlobalTrace("return: m.quit()")
			return m.quit()
		}
		m.quitPending = true
		m.toolbar.SetStatus("press Ctrl+C again to exit")
		return m, tea.Tick(800*time.Millisecond, func(t time.Time) tea.Msg {
			return quitTimeoutMsg{}
		})

	case tea.KeyEsc:
		observe.GlobalTrace("case: tea.KeyEsc")
		if m.perm.active {
			cmd := m.perm.Update(msg)
			observe.GlobalTrace("return: m, cmd")
			return m, cmd
		}
		if m.ask.active {
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

		if m.streaming {
			observe.GlobalTrace("return: m.interruptTurn()")
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
	case tea.KeyCtrlO:
		observe.GlobalTrace("case: tea.KeyCtrlO — toggle verbose")
		m.verbose = !m.verbose
		m.viewport.SetContent(m.viewportContent())
		return m, nil
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
		m.input.SetQueued(true)
		label := msg.Text
		if len(label) > 40 {
			observe.GlobalTrace("if: len(label) > 40")
			label = label[:37] + "..."
		}
		m.toolbar.SetStatus("queued: " + label)
		observe.GlobalTrace("return: m, nil")
		return m, nil
	}
	observe.GlobalTrace("return: m.submitPrompt(msg.Text)")

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
			m.outputSegs = appendText(m.outputSegs, errorStyle.Render("Blocked: "+hookResult.BlockMsg)+"\n")
			m.viewport.SetContent(m.viewportContent())
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
	m.outputSegs = appendText(m.outputSegs, render.RenderMessage(userMsg, m.mdRenderer))
	m.viewport.SetContent(m.viewportContent())
	m.viewport.GotoBottom()

	m.streaming = true
	m.input.SetStreaming(true)
	m.toolbar.SetStatus("streaming...")

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
	m.input.SetQueued(false)
	m.spinnerActive = false
	m.eventCh = nil
	m.toolbar.SetStatus("ready")
	m.viewport.SetContent(m.viewportContent())
	m.viewport.GotoBottom()

	if m.pendingInput != "" {
		observe.GlobalTrace("if: m.pendingInput != \"\"")
		text := m.pendingInput
		m.pendingInput = ""
		observe.GlobalTrace("return: m, func() tea.Msg {\n\treturn InputSubmittedMsg{Text: text}\n}")
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
	m.outputSegs = m.flushStreamBuf()
	m.spinnerActive = false
	m.outputSegs = appendText(m.outputSegs, "\n"+render.BracketPrefix+thinkingStyle.Render("Interrupted · What should pragma do instead?")+"\n\n")
	finished, cmd := m.finishTurn()
	observe.GlobalTrace("return: finished, cmd")
	return finished, cmd
}

// maxOutputBufBytes is the maximum size of the output buffer before trimming.
const maxOutputBufBytes = 512 * 1024 // 512KB

// flushStreamBuf moves accumulated streaming text into the permanent output segments.
// Renders remaining text through markdown before flushing, then trims excess.
// Returns the updated segment slice — caller must assign:
//
//	m.outputSegs = m.flushStreamBuf()
func (m Model) flushStreamBuf() []segment {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	segs := m.outputSegs
	if m.streamBuf.Len() > 0 {
		observe.GlobalTrace("if: m.streamBuf.Len() > 0")
		rendered := m.mdRenderer.Render(m.streamBuf.String())
		segs = appendText(segs, rendered+"\n")
		m.streamBuf.Reset()
	}
	observe.GlobalTrace("return: trimOutputSegs(segs)")
	return trimOutputSegs(segs)
}

// trimOutputSegs trims output segments to maxOutputBufBytes, keeping the tail.
// Returns the trimmed slice — caller must assign: m.outputSegs = trimOutputSegs(m.outputSegs)
func trimOutputSegs(segs []segment) []segment {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	total := outputLen(segs)
	if total <= maxOutputBufBytes {
		observe.GlobalTrace("if: total <= maxOutputBufBytes")
		observe.GlobalTrace("return: segs")
		return segs
	}
	excess := total - maxOutputBufBytes
	trimmed := 0
	cutIdx := 0
	for i, seg := range segs {
		observe.GlobalTrace("range segs")
		segSize := segByteSize(seg)
		if trimmed+segSize > excess {
			observe.GlobalTrace("if: trimmed+segSize > excess")

			if seg.kind == segText {
				observe.GlobalTrace("if: seg.kind == segText")
				remainder := excess - trimmed
				idx := strings.IndexByte(seg.content[remainder:], '\n')
				if idx >= 0 {
					observe.GlobalTrace("if: idx >= 0")
					segs[i].content = seg.content[remainder+idx+1:]
				} else {
					observe.GlobalTrace("else: idx >= 0")
					segs[i].content = seg.content[remainder:]
				}
			}
			cutIdx = i
			break
		}
		trimmed += segSize
		cutIdx = i + 1
	}
	observe.GlobalTrace("return: segs[cutIdx:]")
	return segs[cutIdx:]
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

	m.outputSegs = appendText(m.outputSegs, "\n"+thinkingStyle.Render(m.toolbar.CostSummary())+"\n")
	m.viewport.SetContent(m.viewportContent())
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

// truncateToolbar truncates a string to max runes for toolbar display.
func truncateToolbar(s string, max int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	runes := []rune(s)
	if len(runes) <= max {
		observe.GlobalTrace("if: len(runes) <= max")
		observe.GlobalTrace("return: s")
		return s
	}
	observe.GlobalTrace("return: string(runes[:max-3]) + \"...\"")
	return string(runes[:max-3]) + "..."
}
