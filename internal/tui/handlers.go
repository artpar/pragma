package tui

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/interactive"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/slash"
	"github.com/artpar/pragma/internal/tui/render"
)

// ExtractUserPrompts extracts the text of user messages from a conversation.
// Used to pre-populate input history when opening or resuming a session.
func ExtractUserPrompts(messages []model.Message) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	prompts := model.ExtractUserTextPrompts(messages)
	observe.GlobalTrace("return: prompts")
	return prompts
}

func extractUserPrompts(messages []model.Message) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: ExtractUserPrompts(messages)")
	return ExtractUserPrompts(messages)
}

func latestAssistantText(store *app.StateStore) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if store == nil {
		observe.GlobalTrace("if: store == nil")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	messages := store.Snapshot().Conversation.Messages
	for i := len(messages) - 1; i >= 0; i-- {
		observe.GlobalTrace("for: i >= 0")
		msg := messages[i]
		if msg.Role != model.RoleAssistant || msg.Flags.IsInternal || msg.Flags.IsMeta {
			observe.GlobalTrace("if: msg.Role != model.RoleAssistant || msg.Flags.IsInternal || msg.Flags.IsMeta")
			continue
		}
		var parts []string
		for _, part := range msg.Content {
			observe.GlobalTrace("range msg.Content")
			if tp, ok := part.(model.TextPart); ok && strings.TrimSpace(tp.Text) != "" {
				observe.GlobalTrace("if: ok && strings.TrimSpace(tp.Text) != \"\"")
				parts = append(parts, tp.Text)
			}
		}
		if len(parts) > 0 {
			observe.GlobalTrace("if: len(parts) > 0")
			observe.GlobalTrace("return: strings.Join(parts, \"\\n\")")
			return strings.Join(parts, "\n")
		}
	}
	observe.GlobalTrace("return: \"\"")
	return ""
}

func (m Model) handleRuntimeSlashResult(result slash.Result) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if result.ClearConversation {
		observe.GlobalTrace("if: result.ClearConversation")
		m.outputSegs = m.outputSegs[:0]
	}
	if result.DisplayText != "" {
		observe.GlobalTrace("if: result.DisplayText != \"\"")
		rendered := m.mdRenderer.Render(result.DisplayText)
		m.outputSegs = appendText(m.outputSegs, rendered+"\n\n")
	}
	if result.OpenTeams {
		observe.GlobalTrace("if: result.OpenTeams")
		m.teams.Show(m.taskReg)
	}
	if result.OpenModelPicker {
		observe.GlobalTrace("if: result.OpenModelPicker")
		var models []string
		currentModel := m.toolbar.modelName
		if m.slashDeps.ModelLister != nil {
			observe.GlobalTrace("if: m.slashDeps.ModelLister != nil")
			models = m.slashDeps.ModelLister()
		}
		m.modelDlg.Show(models, currentModel)
	}
	if result.OpenResumePicker {
		observe.GlobalTrace("if: result.OpenResumePicker")
		if len(result.ResumeCandidates) > 0 {
			observe.GlobalTrace("if: len(result.ResumeCandidates) > 0")
			m.resumeDlg.Show(result.ResumeCandidates, result.ResumeScope)
		}
	}
	if result.ResumeSessionID != "" {
		observe.GlobalTrace("if: result.ResumeSessionID != \"\"")
		m.reloadConversationFromStore()
		m.toolbar.SetStartTime(m.store.Snapshot().Conversation.CreatedAt)
	}
	if snap := m.store.Snapshot(); snap.Model != "" {
		observe.GlobalTrace("if: snap.Model != \"\"")
		m.toolbar.SetModel(snap.Model)
	}
	m.viewport.SetContent(m.viewportContent())
	m.viewport.GotoBottom()
	observe.GlobalTrace("return: m, nil")
	return m, nil
}

func (m *Model) reloadConversationFromStore() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	snap := m.store.Snapshot()
	conv := snap.Conversation
	m.outputSegs = nil
	for _, msg := range conv.Messages {
		observe.GlobalTrace("range conv.Messages")
		m.outputSegs = loadMessageSegments(m.outputSegs, msg, m.mdRenderer)
	}
	m.input.SetHistory(promptHistoryFromSnapshot(snap))
}

func promptHistoryFromSnapshot(snap app.AppState) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(snap.PromptHistory) > 0 {
		observe.GlobalTrace("if: len(snap.PromptHistory) > 0")
		out := make([]string, len(snap.PromptHistory))
		copy(out, snap.PromptHistory)
		observe.GlobalTrace("return: out")
		return out
	}
	observe.GlobalTrace("return: extractUserPrompts(snap.Conversation.Messages)")
	return extractUserPrompts(snap.Conversation.Messages)
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

	var loopEvent query.LoopEvent
	switch ev := msg.Event.(type) {
	case interactive.AcceptedPromptEvent:
		observe.GlobalTrace("typecase: interactive.AcceptedPromptEvent")
		m.input.remember(ev.Prompt)
		m.renderAcceptedPrompt(ev.Prompt)
		return m, waitForEvent(m.eventCh)
	case interactive.RejectedPromptEvent:
		observe.GlobalTrace("typecase: interactive.RejectedPromptEvent")
		m.toolbar.SetStatus("busy")
		m.outputSegs = appendText(m.outputSegs, errorStyle.Render("Prompt rejected: another turn is still running")+"\n\n")
		m.viewport.SetContent(m.viewportContent())
		m.viewport.GotoBottom()
		return m, waitForEvent(m.eventCh)
	case interactive.SlashResultEvent:
		observe.GlobalTrace("typecase: interactive.SlashResultEvent")
		next, cmd := m.handleRuntimeSlashResult(ev.Result)
		if cmd != nil {
			observe.GlobalTrace("return: next, cmd")
			return next, cmd
		}
		return next, waitForEvent(m.eventCh)
	case interactive.RuntimeTerminatedEvent:
		observe.GlobalTrace("typecase: interactive.RuntimeTerminatedEvent")
		return m.finishRuntimeTermination()
	case interactive.LoopEvent:
		observe.GlobalTrace("typecase: interactive.LoopEvent")
		loopEvent = ev.Event
	default:
		observe.GlobalTrace("typedefault")
		return m, waitForEvent(m.eventCh)
	}
	if loopEvent == nil {
		observe.GlobalTrace("if: loopEvent == nil")
		observe.GlobalTrace("return: m, waitForEvent(m.eventCh)")
		return m, waitForEvent(m.eventCh)
	}

	switch e := loopEvent.(type) {
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

	case query.CompactionFailedEvent:
		observe.GlobalTrace("typecase: query.CompactionFailedEvent")
		m.outputSegs = appendText(m.outputSegs, "\n"+thinkingStyle.Render(
			fmt.Sprintf("[auto-compaction failed (attempt %d/%d)]", e.Attempt, e.MaxRetry))+"\n")
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

	case query.OrchestrationStartedEvent:
		observe.GlobalTrace("typecase: query.OrchestrationStartedEvent")
		m.closeActiveGroup()
		m.outputSegs = m.flushStreamBuf()
		m.outputSegs = appendText(m.outputSegs, fmt.Sprintf("[orchestration: %s initial=%s]\n", e.Name, e.Initial))
		m.toolbar.SetStatus("orchestration...")
		m.viewport.SetContent(m.viewportContent())
		m.viewport.GotoBottom()

	case query.OrchestrationStateStartedEvent:
		observe.GlobalTrace("typecase: query.OrchestrationStateStartedEvent")
		m.closeActiveGroup()
		m.outputSegs = m.flushStreamBuf()
		if e.Control != "" {
			m.outputSegs = appendText(m.outputSegs, fmt.Sprintf("\n[control: %s (%s)]\n", e.StateID, e.Control))
		} else {
			m.outputSegs = appendText(m.outputSegs, fmt.Sprintf("\n[orchestration: %s persona=%s]\n", e.StateID, e.PersonaID))
		}
		m.viewport.SetContent(m.viewportContent())
		m.viewport.GotoBottom()

	case query.OrchestrationStateCompletedEvent:
		observe.GlobalTrace("typecase: query.OrchestrationStateCompletedEvent")
		m.closeActiveGroup()
		m.outputSegs = m.flushStreamBuf()
		m.outputSegs = appendText(m.outputSegs, fmt.Sprintf("\n[state %s complete in %s]\n", e.StateID, e.Duration.Round(time.Second)))
		m.viewport.SetContent(m.viewportContent())
		m.viewport.GotoBottom()

	case query.OrchestrationControlEvent:
		observe.GlobalTrace("typecase: query.OrchestrationControlEvent")
		if e.Event != "" {
			m.closeActiveGroup()
			m.outputSegs = m.flushStreamBuf()
			m.outputSegs = appendText(m.outputSegs, fmt.Sprintf("[control: %s emitted %s]\n", e.StateID, e.Event))
			m.viewport.SetContent(m.viewportContent())
			m.viewport.GotoBottom()
		}

	case query.OrchestrationTransitionEvent:
		observe.GlobalTrace("typecase: query.OrchestrationTransitionEvent")
		m.closeActiveGroup()
		m.outputSegs = m.flushStreamBuf()
		m.outputSegs = appendText(m.outputSegs, fmt.Sprintf("\n[transition: %s --%s--> %s]\n", e.From, e.Event, e.To))
		m.viewport.SetContent(m.viewportContent())
		m.viewport.GotoBottom()

	case query.OrchestrationCompletedEvent:
		observe.GlobalTrace("typecase: query.OrchestrationCompletedEvent")
		m.closeActiveGroup()
		m.outputSegs = m.flushStreamBuf()
		m.outputSegs = appendText(m.outputSegs, "\n[orchestration: done]\n")
		m.toolbar.SetStatus("streaming...")
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

				m.fillGroupResult(call, e.Result)
			} else if hasProgressSegment(m.outputSegs, call.Name) {
				observe.GlobalTrace("else-if: hasProgressSegment — skip segTool")
			} else {
				observe.GlobalTrace("else: append segTool")
				m.outputSegs = appendTool(m.outputSegs, toolSegData{
					Name:    call.Name,
					Input:   call.Input,
					Content: e.Result.Content,
					IsError: e.Result.IsError,
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

	case query.UserMessageEvent:
		observe.GlobalTrace("typecase: query.UserMessageEvent")
		m.closeActiveGroup()
		m.outputSegs = m.flushStreamBuf()
		if strings.TrimSpace(e.Message) != "" {
			m.outputSegs = appendText(m.outputSegs, m.mdRenderer.Render(e.Message)+"\n")
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
		return finished, pendingCmd

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

	if m.modelDlg.active && msg.Type != tea.KeyCtrlC {
		observe.GlobalTrace("if: m.modelDlg.active && msg.Type != tea.KeyCtrlC")
		if selected := m.modelDlg.Update(msg); selected != "" {
			observe.GlobalTrace("if: selected != \"\" — model chosen: " + selected)
			if m.runInput == nil {
				observe.GlobalTrace("if: m.runInput == nil")
				m.outputSegs = appendText(m.outputSegs, errorStyle.Render("Error: model command is not available")+"\n\n")
				m.viewport.SetContent(m.viewportContent())
				observe.GlobalTrace("return: m, nil")
				return m, nil
			}
			observe.GlobalTrace("return: m.submitPrompt(\"/model \" + selected)")
			return m.submitPrompt("/model " + selected)
		}
		m.viewport.SetContent(m.viewportContent())
		observe.GlobalTrace("return: m, nil")
		return m, nil
	}

	if m.resumeDlg.active && msg.Type != tea.KeyCtrlC {
		observe.GlobalTrace("if: m.resumeDlg.active && msg.Type != tea.KeyCtrlC")
		if selectedID := m.resumeDlg.Update(msg); selectedID != "" {
			observe.GlobalTrace("if: selectedID != \"\" — session chosen")
			if m.runInput != nil {
				observe.GlobalTrace("if: m.runInput != nil")
				observe.GlobalTrace("return: m.submitPrompt(\"/resume \" + selectedID)")
				return m.submitPrompt("/resume " + selectedID)
			}
			m.outputSegs = appendText(m.outputSegs, errorStyle.Render("Error: interactive runtime is not available")+"\n\n")
			m.viewport.SetContent(m.viewportContent())
			observe.GlobalTrace("return: m, nil")
			return m, nil
		}
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

	switch msg.Type {
	case tea.KeyCtrlO:
		observe.GlobalTrace("case: tea.KeyCtrlO — toggle verbose")
		m.verbose = !m.verbose
		m.viewport.SetContent(m.viewportContent())
		return m, nil
	case tea.KeyTab:
		observe.GlobalTrace("case: tea.KeyTab — slash completion")
		if m.slashCmds != nil && m.input.CompleteSlash(m.slashCommands(), m.completionWorkspace()) {
			m.input.RefreshSlashCompletions(m.slashCommands(), m.completionWorkspace())
			m.syncViewportHeight()
			observe.GlobalTrace("return: m, nil")
			return m, nil
		}
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
	if m.slashCmds != nil {
		observe.GlobalTrace("if: m.slashCmds != nil")
		m.input.RefreshSlashCompletions(m.slashCommands(), m.completionWorkspace())
	}
	m.syncViewportHeight()
	observe.GlobalTrace("return: m, cmd")
	return m, cmd
}

func (m Model) slashCommands() []slash.Command {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if m.slashCmds == nil {
		observe.GlobalTrace("if: m.slashCmds == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	observe.GlobalTrace("return: m.slashCmds.CommandsWithDeps(m.slashDeps)")
	return m.slashCmds.CommandsWithDeps(m.slashDeps)
}

func (m Model) completionWorkspace() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if m.slashDeps.Store != nil {
		observe.GlobalTrace("if: m.slashDeps.Store != nil")
		if cwd := m.slashDeps.Store.Snapshot().CWD; cwd != "" {
			observe.GlobalTrace("if: cwd != \"\"")
			observe.GlobalTrace("return: cwd")
			return cwd
		}
	}
	observe.GlobalTrace("return: m.workspace")
	return m.workspace
}

// handleInputSubmitted processes user message submission.
func (m Model) handleInputSubmitted(msg InputSubmittedMsg) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if m.runInput == nil {
		observe.GlobalTrace("if: m.runInput == nil")
		m.outputSegs = appendText(m.outputSegs, errorStyle.Render("Error: interactive runtime is not available")+"\n\n")
		m.viewport.SetContent(m.viewportContent())
		m.viewport.GotoBottom()
		observe.GlobalTrace("return: m, nil")
		return m, nil
	}
	if m.streaming {
		observe.GlobalTrace("if: m.streaming — asking runtime admission")
		events := m.runInput(m.parentCtx, msg.Text)
		observe.GlobalTrace("return: m, waitForEvent(events)")
		return m, waitForEvent(events)
	}
	observe.GlobalTrace("return: m.submitPrompt(msg.Text)")

	return m.submitPrompt(msg.Text)
}

// submitPrompt sends a user prompt to the engine and starts streaming.
func (m Model) submitPrompt(text string) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	m.streaming = true
	m.input.SetStreaming(true)
	m.toolbar.SetStatus("streaming...")

	m.cancel()
	m.ctx, m.cancel = context.WithCancel(m.parentCtx)
	m.eventCh = m.runInput(m.ctx, text)

	observe.GlobalTrace("return: m, waitForEvent(m.eventCh)")

	return m, waitForEvent(m.eventCh)
}

func (m *Model) renderAcceptedPrompt(text string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
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
}

// finishTurn resets streaming state after the active runtime turn closes.
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

	observe.GlobalTrace("return: m, nil")
	return m, nil
}

// interruptTurn cancels the streaming context, shows an immediate "Interrupted" message,
// and finishes the turn.
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
	observe.GlobalTrace("return: m.quitWithSessionClose(true)")
	return m.quitWithSessionClose(true)
}

func (m Model) finishRuntimeTermination() (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: m.quitWithSessionClose(false)")
	return m.quitWithSessionClose(false)
}

func (m Model) quitWithSessionClose(closeSession bool) (tea.Model, tea.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.cancel()
	if closeSession && m.closeSession != nil {
		observe.GlobalTrace("if: closeSession && m.closeSession != nil")
		if err := m.closeSession(); err != nil {
			observe.GlobalTrace("if: err != nil")
			m.outputSegs = appendText(m.outputSegs, "\n"+errorStyle.Render("Error: "+err.Error())+"\n\n")
			m.viewport.SetContent(m.viewportContent())
			observe.GlobalTrace("return: m, nil")
			return m, nil
		}
	}

	m.outputSegs = appendText(m.outputSegs, "\n"+thinkingStyle.Render(m.toolbar.CostSummary())+"\n")
	m.viewport.SetContent(m.viewportContent())

	snap := m.store.Snapshot()
	if snap.Conversation.ID != "" {
		observe.GlobalTrace("if: snap.Conversation.ID != \"\"")
		shortID := snap.Conversation.ID
		if len(shortID) > 8 {
			observe.GlobalTrace("if: len(shortID) > 8")
			shortID = shortID[:8]
		}
		fmt.Fprintf(os.Stderr, "\nResume: pragma --resume %s\n", shortID)
	}

	observe.GlobalTrace("return: m, tea.Quit")
	return m, tea.Quit
}

// waitForEvent returns a tea.Cmd that reads the next event from the channel.
func waitForEvent(ch <-chan interactive.Event) tea.Cmd {
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
