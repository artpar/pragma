package hook

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/artpar/gogent/internal/observe"
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
	return &Manager{
		hooks:     LoadHooks(workDir),
		workDir:   workDir,
		sessionID: sessionID,
		bus:       bus,
	}
}

// SetSessionID updates the session ID used in hook input and env vars.
func (m *Manager) SetSessionID(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessionID = id
}

// Reload re-reads hooks from settings files. Call after config changes.
func (m *Manager) Reload() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hooks = LoadHooks(m.workDir)
}

// HasHooks returns true if any hooks are configured for the given event.
func (m *Manager) HasHooks(event Event) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entries := m.hooks[event]
	return len(entries) > 0
}

// Execute runs all matching hooks for an event sequentially and returns the aggregated result.
// For PreToolUse/PostToolUse, input.ToolName is used to filter by matcher.
// Hooks run sequentially — simpler than parallel, avoids race conditions.
func (m *Manager) Execute(ctx context.Context, event Event, input HookInput) AggregatedResult {
	m.mu.RLock()
	input.Event = event
	input.CWD = m.workDir
	input.SessionID = m.sessionID
	sessionID := m.sessionID // snapshot under lock for env vars
	workDir := m.workDir
	entries := m.hooks[event]
	m.mu.RUnlock()
	if len(entries) == 0 {
		return AggregatedResult{}
	}

	// For tool-related events, filter by matcher against tool name
	matchValue := ""
	switch event {
	case PreToolUse, PostToolUse:
		matchValue = input.ToolName
	}

	commands := MatchCommands(entries, matchValue)
	if len(commands) == 0 {
		return AggregatedResult{}
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return AggregatedResult{}
	}

	envVars := map[string]string{
		"GOGENT_HOOK_EVENT":  string(event),
		"GOGENT_SESSION_ID":  sessionID,
		"GOGENT_CWD":         workDir,
		"GOGENT_TOOL_NAME":   input.ToolName,
	}

	var agg AggregatedResult

	for _, cmd := range commands {
		if cmd.Type != "" && cmd.Type != "command" {
			continue // only shell commands supported
		}

		result := ExecCommand(ctx, cmd, inputJSON, m.workDir, envVars)

		m.emitHookEvent(event, cmd.Command, result)

		outcome := result.Outcome()

		switch outcome {
		case OutcomeBlock:
			agg.Blocked = true
			agg.BlockMsg = result.Stderr
			if agg.BlockMsg == "" {
				agg.BlockMsg = "hook blocked execution (exit code 2)"
			}
			return agg // first block wins, stop processing

		case OutcomeOK:
			if result.Stdout != "" {
				agg.Stdout += result.Stdout
			}
			if result.JSON != nil && result.JSON.AdditionalContext != "" {
				agg.Feedback = append(agg.Feedback, result.JSON.AdditionalContext)
			}

		case OutcomeError:
			// Non-blocking: stderr shown to user but execution continues
			if result.JSON != nil && result.JSON.AdditionalContext != "" {
				agg.Feedback = append(agg.Feedback, result.JSON.AdditionalContext)
			}

		case OutcomeTimeout:
			// Timeout: treat as non-blocking error
		}
	}

	return agg
}

func (m *Manager) emitHookEvent(event Event, command string, result Result) {
	if m.bus == nil {
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
		m.bus.Emit(observe.HookBlocked{
			EventHeader: observe.NewEventHeader("HookBlocked", "", observe.NewSpanID(), ""),
			HookEvent:   string(event),
			Command:     command,
			Message:     result.Stderr,
		})
	}
}
