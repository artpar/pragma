package hook

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/artpar/pragma/internal/observe"
)

// Manager loads, matches, and executes hooks.
type Manager struct {
	mu        sync.RWMutex
	hooks     map[Event][]Entry
	workDir   string
	sessionID string
	bus       *observe.EventBus
}

// NewManager creates a Manager, loading hooks from all settings scopes.
func NewManager(workDir, sessionID string, bus *observe.EventBus) *Manager {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &Manager{\n\thooks:\t\tLoadHooks(workDir),\n\tworkDir:\tworkDir,\n\tsessionID:\tsession...")
	return &Manager{
		hooks:     LoadHooks(workDir),
		workDir:   workDir,
		sessionID: sessionID,
		bus:       bus,
	}
}

// SetSessionID updates the session ID used in hook input and env vars.
func (m *Manager) SetSessionID(id string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessionID = id
}

// Reload re-reads hooks from settings files. Call after config changes.
func (m *Manager) Reload() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hooks = LoadHooks(m.workDir)
}

// HasHooks returns true if any hooks are configured for the given event.
func (m *Manager) HasHooks(event Event) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.RLock()
	defer m.mu.RUnlock()
	entries := m.hooks[event]
	observe.GlobalTrace("return: len(entries) > 0")
	return len(entries) > 0
}

// Execute runs all matching hooks for an event sequentially and returns the aggregated result.
// For PreToolUse/PostToolUse, input.ToolName is used to filter by matcher.
// Hooks run sequentially — simpler than parallel, avoids race conditions.
func (m *Manager) Execute(ctx context.Context, event Event, input HookInput) AggregatedResult {
	observe.TraceCtx(ctx, "hook", "Manager.Execute", "enter")
	defer observe.TraceCtx(ctx, "hook", "Manager.Execute", "exit")
	observe.TraceCtx(ctx, "hook", "Manager.Execute", "return: m.ExecuteInWorkDir(ctx, event, input, \"\")")
	return m.ExecuteInWorkDir(ctx, event, input, "")
}

func (m *Manager) ExecuteInWorkDir(ctx context.Context, event Event, input HookInput, eventWorkDir string) AggregatedResult {
	observe.TraceCtx(ctx, "hook", "Manager.Execute", "enter")
	defer observe.TraceCtx(ctx, "hook", "Manager.Execute", "exit")
	m.mu.RLock()
	defaultWorkDir := m.workDir
	workDir := strings.TrimSpace(eventWorkDir)
	if workDir == "" {
		observe.TraceCtx(ctx, "hook", "Manager.ExecuteInWorkDir", "if: workDir == \"\"")
		workDir = defaultWorkDir
	}
	input.Event = event
	input.CWD = workDir
	input.SessionID = m.sessionID
	sessionID := m.sessionID
	var entries []Entry
	if workDir == defaultWorkDir {
		observe.TraceCtx(ctx, "hook", "Manager.ExecuteInWorkDir", "if: workDir == defaultWorkDir")
		entries = m.hooks[event]
	}
	m.mu.RUnlock()
	if workDir != defaultWorkDir {
		observe.TraceCtx(ctx, "hook", "Manager.ExecuteInWorkDir", "if: workDir != defaultWorkDir")
		entries = LoadHooks(workDir)[event]
	}
	if len(entries) == 0 {
		observe.TraceCtx(ctx, "hook", "Manager.Execute", "if: len(entries) == 0")
		observe.TraceCtx(ctx, "hook", "Manager.Execute", "return: AggregatedResult{}")
		return AggregatedResult{}
	}

	matchValue := ""
	switch event {
	case PreToolUse, PostToolUse:
		observe.TraceCtx(ctx, "hook", "Manager.Execute", "case: PreToolUse, PostToolUse")
		matchValue = input.ToolName
	}

	commands := MatchCommands(entries, matchValue)
	if len(commands) == 0 {
		observe.TraceCtx(ctx, "hook", "Manager.Execute", "if: len(commands) == 0")
		observe.TraceCtx(ctx, "hook", "Manager.Execute", "return: AggregatedResult{}")
		return AggregatedResult{}
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		observe.TraceCtx(ctx, "hook", "Manager.Execute", "if: err != nil")
		observe.TraceCtx(ctx, "hook", "Manager.Execute", "return: AggregatedResult{}")
		return AggregatedResult{}
	}

	envVars := map[string]string{
		"PRAGMA_HOOK_EVENT": string(event),
		"PRAGMA_SESSION_ID": sessionID,
		"PRAGMA_CWD":        workDir,
		"PRAGMA_TOOL_NAME":  input.ToolName,
	}

	var agg AggregatedResult

	for _, cmd := range commands {
		observe.TraceCtx(ctx, "hook", "Manager.Execute", "range commands")
		if cmd.Type != "" && cmd.Type != "command" {
			observe.TraceCtx(ctx, "hook", "Manager.Execute", "if: cmd.Type != \"\" && cmd.Type != \"command\"")
			continue
		}

		result := ExecCommand(ctx, cmd, inputJSON, workDir, envVars)

		m.emitHookEvent(event, cmd.Command, result)

		outcome := result.Outcome()

		switch outcome {
		case OutcomeBlock:
			observe.TraceCtx(ctx, "hook", "Manager.Execute", "case: OutcomeBlock")
			agg.Blocked = true
			agg.BlockMsg = hookBlockMessage(result, "hook blocked execution (exit code 2)")
			return agg

		case OutcomeOK:
			observe.TraceCtx(ctx, "hook", "Manager.Execute", "case: OutcomeOK")
			if applyJSONControl(event, result, &agg) {
				observe.TraceCtx(ctx, "hook", "Manager.Execute", "if: applyJSONControl")
				m.emitHookBlocked(event, cmd.Command, agg.BlockMsg)
				observe.TraceCtx(ctx, "hook", "Manager.ExecuteInWorkDir", "return: agg")
				return agg
			}
			if result.Stdout != "" {
				agg.Stdout += result.Stdout
			}
			if result.JSON != nil && result.JSON.AdditionalContext != "" {
				agg.Feedback = append(agg.Feedback, result.JSON.AdditionalContext)
			}

		case OutcomeError:
			observe.TraceCtx(ctx, "hook", "Manager.Execute", "case: OutcomeError")
			if applyJSONControl(event, result, &agg) {
				observe.TraceCtx(ctx, "hook", "Manager.Execute", "if: applyJSONControl")
				m.emitHookBlocked(event, cmd.Command, agg.BlockMsg)
				observe.TraceCtx(ctx, "hook", "Manager.ExecuteInWorkDir", "return: agg")
				return agg
			}

			if result.JSON != nil && result.JSON.AdditionalContext != "" {
				agg.Feedback = append(agg.Feedback, result.JSON.AdditionalContext)
			}

		case OutcomeTimeout:
			observe.TraceCtx(ctx, "hook", "Manager.Execute", "case: OutcomeTimeout")

		}
	}
	observe.TraceCtx(ctx, "hook", "Manager.Execute", "return: agg")

	return agg
}

func applyJSONControl(event Event, result Result, agg *AggregatedResult) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if result.JSON == nil || agg == nil {
		observe.GlobalTrace("if: result.JSON == nil || agg == nil")
		observe.GlobalTrace("return: false")
		return false
	}
	if eventSupportsDecisionBlock(event) && strings.EqualFold(strings.TrimSpace(result.JSON.Decision), "block") {
		observe.GlobalTrace("if: eventSupportsDecisionBlock(event) && strings.EqualFold(strings.TrimSpace(resu...")
		agg.Blocked = true
		agg.BlockMsg = hookBlockMessage(result, "hook blocked execution")
		observe.GlobalTrace("return: true")
		return true
	}
	if result.JSON.Continue != nil && !*result.JSON.Continue {
		observe.GlobalTrace("if: result.JSON.Continue != nil && !*result.JSON.Continue")
		agg.Blocked = true
		agg.BlockMsg = hookBlockMessage(result, "hook requested stop")
		observe.GlobalTrace("return: true")
		return true
	}
	observe.GlobalTrace("return: false")
	return false
}

func eventSupportsDecisionBlock(event Event) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch event {
	case PreToolUse, UserPromptSubmit, SessionStart:
		observe.GlobalTrace("case: PreToolUse, UserPromptSubmit, SessionStart")
		return true
	default:
		observe.GlobalTrace("default")
		return false
	}
}

func hookBlockMessage(result Result, fallback string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if result.JSON != nil {
		observe.GlobalTrace("if: result.JSON != nil")
		if msg := strings.TrimSpace(result.JSON.Reason); msg != "" {
			observe.GlobalTrace("if: msg != \"\"")
			observe.GlobalTrace("return: msg")
			return msg
		}
		if msg := strings.TrimSpace(result.JSON.StopReason); msg != "" {
			observe.GlobalTrace("if: msg != \"\"")
			observe.GlobalTrace("return: msg")
			return msg
		}
	}
	if msg := strings.TrimSpace(result.Stderr); msg != "" {
		observe.GlobalTrace("if: msg != \"\"")
		observe.GlobalTrace("return: msg")
		return msg
	}
	observe.GlobalTrace("return: fallback")
	return fallback
}

func (m *Manager) emitHookEvent(event Event, command string, result Result) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if m.bus == nil {
		observe.GlobalTrace("if: m.bus == nil")
		return
	}
	outcome := result.Outcome()
	m.bus.Emit(observe.HookExecuted{
		EventHeader: observe.NewEventHeader("HookExecuted", "", observe.NewSpanID(), ""),
		HookEvent:   string(event),
		Command:     command,
		ExitCode:    result.ExitCode,
		Outcome:     string(outcome),
		HasJSON:     result.JSON != nil,
	})

	if outcome == OutcomeBlock {
		observe.GlobalTrace("if: outcome == OutcomeBlock")
		m.emitHookBlocked(event, command, result.Stderr)
	}
}

func (m *Manager) emitHookBlocked(event Event, command string, message string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if m.bus == nil {
		observe.GlobalTrace("if: m.bus == nil")
		return
	}
	m.bus.Emit(observe.HookBlocked{
		EventHeader: observe.NewEventHeader("HookBlocked", "", observe.NewSpanID(), ""),
		HookEvent:   string(event),
		Command:     command,
		Message:     message,
	})
}
