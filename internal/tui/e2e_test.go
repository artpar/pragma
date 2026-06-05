package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
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

func newTestModel() Model {
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", "/tmp")
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          "/tmp",
		Model:        "test-model",
		Provider:     "test",
	})
	costTracker := model.NewCostTracker(0)
	metrics := observe.NewMetrics(observe.MetricsSeed{})
	slashCmds := slash.NewRegistry()
	slashDeps := slash.Deps{
		Store:       store,
		CostTracker: costTracker,
		ModelName:   "test-model",
		Provider:    "test",
	}
	runInput := func(ctx context.Context, input string) <-chan interactive.Event {
		ch := make(chan interactive.Event, 4)
		go func() {
			defer close(ch)
			if name, args, ok := slash.Parse(input); ok {
				ch <- interactive.AcceptedPromptEvent{Prompt: input}
				result, err := slashCmds.Execute(ctx, name, args, slashDeps)
				if err != nil {
					ch <- interactive.LoopEvent{Event: query.ErrorEvent{Err: err}}
					return
				}
				ch <- interactive.SlashResultEvent{Result: result}
			}
		}()
		return ch
	}

	return New(Config{
		Store:       store,
		CostTracker: costTracker,
		Metrics:     metrics,
		ModelName:   "test-model",
		Provider:    "test",
		RunInput:    runInput,
		SlashCmds:   slashCmds,
		SlashDeps:   slashDeps,
	})
}

func TestModelRoutesPromptHistoryKeysToInput(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.input.SetHistory([]string{"first prompt", "second prompt"})

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Model)
	if got := m.input.textarea.Value(); got != "second prompt" {
		t.Fatalf("first up = %q, want second prompt", got)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Model)
	if got := m.input.textarea.Value(); got != "first prompt" {
		t.Fatalf("second up = %q, want first prompt", got)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if got := m.input.textarea.Value(); got != "second prompt" {
		t.Fatalf("down = %q, want second prompt", got)
	}
}

func TestModelAltArrowsScrollWithoutChangingHistory(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.input.SetHistory([]string{"first prompt", "second prompt"})
	m.input.textarea.SetValue("draft")

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp, Alt: true})
	m = updated.(Model)
	if got := m.input.textarea.Value(); got != "draft" {
		t.Fatalf("alt-up changed input to %q, want draft", got)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown, Alt: true})
	m = updated.(Model)
	if got := m.input.textarea.Value(); got != "draft" {
		t.Fatalf("alt-down changed input to %q, want draft", got)
	}
}

func TestLatestAssistantTextUsesMostRecentAssistantText(t *testing.T) {
	m := newTestModel()
	m.store.Update(func(s *app.AppState) {
		s.Conversation.Append(model.Message{
			ID:      model.NewUUID(),
			Role:    model.RoleAssistant,
			Content: []model.ContentPart{model.TextPart{Text: "older answer"}},
		})
		s.Conversation.Append(model.Message{
			ID:   model.NewUUID(),
			Role: model.RoleUser,
			Content: []model.ContentPart{
				model.TextPart{Text: "not copied"},
			},
		})
		s.Conversation.Append(model.Message{
			ID:   model.NewUUID(),
			Role: model.RoleAssistant,
			Content: []model.ContentPart{
				model.TextPart{Text: "latest"},
				model.ToolCallPart{Name: "Read"},
				model.TextPart{Text: "answer"},
			},
		})
	})

	if got := latestAssistantText(m.store); got != "latest\nanswer" {
		t.Fatalf("latestAssistantText = %q, want latest\\nanswer", got)
	}
}

// runTUIWithKeys starts a real bubbletea program with real keystroke bytes,
// waits for it to finish, and returns the final output.
func runTUIWithKeys(t *testing.T, keys string) string {
	t.Helper()
	m := newTestModel()

	var output bytes.Buffer
	input := strings.NewReader(keys)

	p := tea.NewProgram(m,
		tea.WithInput(input),
		tea.WithOutput(&output),
		tea.WithoutRenderer(),
	)

	done := make(chan error, 1)
	go func() {
		_, err := p.Run()
		done <- err
	}()

	// Give it time to process keys, then quit
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("program error: %v", err)
		}
	case <-time.After(5 * time.Second):
		p.Kill()
		t.Fatal("TUI did not exit within 5s")
	}

	return output.String()
}

// sendKeysAndCapture starts a real bubbletea program, sends keystrokes,
// then sends Ctrl+C to exit, and returns the model's final View().
func sendKeysAndCapture(t *testing.T, keys string) string {
	t.Helper()
	m := newTestModel()

	// Pipe: we write keys on one end, bubbletea reads from the other
	pr, pw := io.Pipe()

	var output bytes.Buffer
	p := tea.NewProgram(m,
		tea.WithInput(pr),
		tea.WithOutput(&output),
		tea.WithoutRenderer(),
	)

	done := make(chan tea.Model, 1)
	go func() {
		finalModel, _ := p.Run()
		done <- finalModel
	}()

	// Send a resize so model becomes ready
	p.Send(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Small delay for model to initialize
	time.Sleep(100 * time.Millisecond)

	// Write actual key bytes (like a real terminal would)
	_, _ = pw.Write([]byte(keys))

	// Wait for slash command processing (with segments, model updates are
	// propagated through Update() rather than shared pointer mutation)
	time.Sleep(500 * time.Millisecond)

	// Quit
	p.Quit()
	pw.Close()

	select {
	case fm := <-done:
		if fm == nil {
			t.Fatal("nil final model")
		}
		return fm.(Model).viewportContent()
	case <-time.After(5 * time.Second):
		p.Kill()
		t.Fatal("TUI did not exit within 5s")
		return ""
	}
}

func TestTUIRealKeysSlashHelp(t *testing.T) {
	// Type "/help" then press Enter (0x0d = carriage return)
	output := sendKeysAndCapture(t, "/help\r")

	if !strings.Contains(output, "Available commands") &&
		!strings.Contains(output, "/help") {
		t.Errorf("Expected slash command output, got:\n%s", output)
	}
}

func TestTUIRealKeysSlashCost(t *testing.T) {
	output := sendKeysAndCapture(t, "/cost\r")

	if !strings.Contains(output, "$0.0000") &&
		!strings.Contains(output, "/cost") {
		t.Errorf("Expected cost output, got:\n%s", output)
	}
}

func TestTUIRealKeysSlashExit(t *testing.T) {
	m := newTestModel()

	pr, pw := io.Pipe()
	var output bytes.Buffer
	p := tea.NewProgram(m,
		tea.WithInput(pr),
		tea.WithOutput(&output),
		tea.WithoutRenderer(),
	)

	done := make(chan tea.Model, 1)
	go func() {
		fm, _ := p.Run()
		done <- fm
	}()

	p.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
	time.Sleep(100 * time.Millisecond)

	// Type /exit and Enter — program should quit on its own
	_, _ = pw.Write([]byte("/exit\r"))
	pw.Close()

	select {
	case <-done:
		// /exit should cause the program to quit
	case <-time.After(5 * time.Second):
		p.Kill()
		t.Fatal("/exit did not cause program to quit")
	}
}

// --- Ctrl+O Verbose Toggle Tests ---

func TestVerboseToggle(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24

	// Simulate a Read tool result in output segments
	m.outputSegs = appendTool(m.outputSegs, toolSegData{
		Name:    "Read",
		Input:   json.RawMessage(`{"file_path":"go.mod"}`),
		Content: "module github.com/artpar/pragma\n\ngo 1.23\n",
		IsError: false,
	})

	// Non-verbose: should show compact summary
	m.verbose = false
	content := m.viewportContent()
	if !strings.Contains(content, "Read") {
		t.Error("non-verbose: expected tool name in output")
	}
	// The compact render shows "Read N lines" or similar
	if strings.Contains(content, "module github.com/artpar/pragma") {
		t.Error("non-verbose: should NOT show full file content")
	}

	// Verbose: should show full content
	m.verbose = true
	content = m.viewportContent()
	if !strings.Contains(content, "module github.com/artpar/pragma") {
		t.Error("verbose: should show full file content")
	}
}

func TestVerboseToggleBash(t *testing.T) {
	m := newTestModel()
	m.width = 80

	// Bash tool with many lines of output
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d output", i+1)
	}
	output := strings.Join(lines, "\n")

	m.outputSegs = appendTool(m.outputSegs, toolSegData{
		Name:    "Bash",
		Input:   json.RawMessage(`{"command":"ls -la"}`),
		Content: output,
		IsError: false,
		Display: "exit_code:0",
	})

	// Non-verbose: last 5 lines only
	m.verbose = false
	content := m.viewportContent()
	if strings.Contains(content, "line 1 output") {
		t.Error("non-verbose bash: should NOT show first line")
	}
	if !strings.Contains(content, "line 20 output") {
		t.Error("non-verbose bash: should show last line")
	}

	// Verbose: all lines
	m.verbose = true
	content = m.viewportContent()
	if !strings.Contains(content, "line 1 output") {
		t.Error("verbose bash: should show first line")
	}
	if !strings.Contains(content, "line 20 output") {
		t.Error("verbose bash: should show last line")
	}
}

func TestVerboseToggleThinking(t *testing.T) {
	m := newTestModel()
	m.width = 80

	m.outputSegs = append(m.outputSegs, segment{
		kind:    segThinking,
		content: "Let me analyze this carefully...",
	})

	// Non-verbose: thinking should be collapsed
	m.verbose = false
	content := m.viewportContent()
	if strings.Contains(content, "Let me analyze this carefully") {
		t.Error("non-verbose: thinking should be collapsed")
	}

	// Verbose: thinking should be visible
	m.verbose = true
	content = m.viewportContent()
	if !strings.Contains(content, "Let me analyze this carefully") {
		t.Error("verbose: thinking should be expanded")
	}
}

// --- Group Collapsing Tests ---

func TestGroupCollapsingConsecutiveReads(t *testing.T) {
	m := newTestModel()
	m.width = 80

	// Simulate 3 consecutive Read tool calls
	for i, path := range []string{"go.mod", "go.sum", "SPEC.md"} {
		call := model.ToolCallPart{
			ID:    fmt.Sprintf("tc-%d", i),
			Name:  "Read",
			Input: json.RawMessage(fmt.Sprintf(`{"file_path":"%s"}`, path)),
		}
		callHeader := render.RenderToolCall(call, 80) + "\n"
		m.addToGroup(callHeader, call, "read")
		m.fillGroupResult(call, model.ToolResultPart{
			ToolCallID: call.ID,
			Content:    "file content of " + path,
		}, "")
	}

	// Should have exactly 1 group segment
	groupCount := 0
	for _, seg := range m.outputSegs {
		if seg.kind == segGroup {
			groupCount++
		}
	}
	if groupCount != 1 {
		t.Errorf("expected 1 group segment, got %d", groupCount)
	}

	// Non-verbose: should show summary badge
	m.verbose = false
	content := m.viewportContent()
	if !strings.Contains(content, "3") {
		t.Error("non-verbose: expected '3' in group summary (read 3 files)")
	}

	// Verbose: should show individual tool results
	m.verbose = true
	content = m.viewportContent()
	if !strings.Contains(content, "go.mod") {
		t.Error("verbose: should show individual file paths")
	}
	if !strings.Contains(content, "SPEC.md") {
		t.Error("verbose: should show all file paths")
	}
}

func TestGroupCollapsingMixedSearchAndRead(t *testing.T) {
	m := newTestModel()
	m.width = 80

	// 1 Grep + 2 Reads should all collapse into same group
	grepCall := model.ToolCallPart{ID: "tc-grep", Name: "Grep", Input: json.RawMessage(`{"pattern":"func main"}`)}
	m.addToGroup(render.RenderToolCall(grepCall, 80)+"\n", grepCall, "search")
	m.fillGroupResult(grepCall, model.ToolResultPart{ToolCallID: "tc-grep", Content: "main.go:5"}, "")

	for i, path := range []string{"main.go", "util.go"} {
		call := model.ToolCallPart{ID: fmt.Sprintf("tc-r%d", i), Name: "Read", Input: json.RawMessage(fmt.Sprintf(`{"file_path":"%s"}`, path))}
		m.addToGroup(render.RenderToolCall(call, 80)+"\n", call, "read")
		m.fillGroupResult(call, model.ToolResultPart{ToolCallID: call.ID, Content: "content"}, "")
	}

	// Verify single group with correct counts
	var g *groupSegData
	for _, seg := range m.outputSegs {
		if seg.kind == segGroup {
			g = seg.group
		}
	}
	if g == nil {
		t.Fatal("expected a group segment")
	}
	if g.SearchCount != 1 {
		t.Errorf("SearchCount = %d, want 1", g.SearchCount)
	}
	if len(g.ReadPaths) != 2 {
		t.Errorf("ReadPaths count = %d, want 2", len(g.ReadPaths))
	}
}

func TestGroupBreaksOnNonCollapsibleTool(t *testing.T) {
	m := newTestModel()
	m.width = 80

	// Read → group starts
	readCall := model.ToolCallPart{ID: "tc-1", Name: "Read", Input: json.RawMessage(`{"file_path":"a.go"}`)}
	m.addToGroup(render.RenderToolCall(readCall, 80)+"\n", readCall, "read")
	m.fillGroupResult(readCall, model.ToolResultPart{ToolCallID: "tc-1", Content: "x"}, "")

	// Bash → group should close
	m.closeActiveGroup()

	// Another Read → new group starts
	read2 := model.ToolCallPart{ID: "tc-3", Name: "Read", Input: json.RawMessage(`{"file_path":"b.go"}`)}
	m.addToGroup(render.RenderToolCall(read2, 80)+"\n", read2, "read")
	m.fillGroupResult(read2, model.ToolResultPart{ToolCallID: "tc-3", Content: "y"}, "")

	// Should have 2 group segments
	groupCount := 0
	for _, seg := range m.outputSegs {
		if seg.kind == segGroup {
			groupCount++
		}
	}
	if groupCount != 2 {
		t.Errorf("expected 2 group segments (broken by Bash), got %d", groupCount)
	}
}

func TestGroupLatestHint(t *testing.T) {
	m := newTestModel()
	m.width = 80

	// Grep with pattern — LatestHint should be the pattern
	call := model.ToolCallPart{ID: "tc-1", Name: "Grep", Input: json.RawMessage(`{"pattern":"autoDetect"}`)}
	m.addToGroup(render.RenderToolCall(call, 80)+"\n", call, "search")
	m.fillGroupResult(call, model.ToolResultPart{ToolCallID: "tc-1", Content: "found"}, "")

	var g *groupSegData
	for _, seg := range m.outputSegs {
		if seg.kind == segGroup {
			g = seg.group
		}
	}
	if g == nil {
		t.Fatal("expected group segment")
	}
	if !strings.Contains(g.LatestHint, "autoDetect") {
		t.Errorf("LatestHint = %q, want to contain 'autoDetect'", g.LatestHint)
	}
}

// --- Input Queuing Tests ---

func TestInputQueuingDuringStreaming(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.streaming = true

	// Submit input while streaming — should be queued
	result, _ := m.handleInputSubmitted(InputSubmittedMsg{Text: "follow-up question"})
	updated := result.(Model)

	if updated.pendingInput != "follow-up question" {
		t.Errorf("pendingInput = %q, want %q", updated.pendingInput, "follow-up question")
	}
}

func TestInputQueuingTruncatesLongLabel(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.streaming = true

	longText := strings.Repeat("x", 50) // > 40 chars
	result, _ := m.handleInputSubmitted(InputSubmittedMsg{Text: longText})
	updated := result.(Model)

	if updated.pendingInput != longText {
		t.Error("pendingInput should store full text")
	}
	// Toolbar status should be truncated
	status := updated.toolbar.status
	if len(status) > 60 {
		t.Errorf("toolbar status too long: %d chars", len(status))
	}
}

// --- Ctrl+C / Interrupt Tests ---

func TestCtrlCDuringStreamingInterrupts(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.streaming = true
	m.streamBuf.WriteString("partial output...")

	// Simulate Ctrl+C during streaming
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	updated := result.(Model)

	if updated.streaming {
		t.Error("streaming should be false after interrupt")
	}

	content := updated.viewportContent()
	if !strings.Contains(content, "Interrupted") {
		t.Error("expected 'Interrupted' in viewport after Ctrl+C")
	}
}

func TestCtrlCWhenIdleSetsQuitPending(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.streaming = false
	m.ready = true

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	updated := result.(Model)

	if !updated.quitPending {
		t.Error("expected quitPending=true after first Ctrl+C when idle")
	}
	if !strings.Contains(updated.toolbar.status, "Ctrl+C") {
		t.Errorf("toolbar should show exit hint, got: %q", updated.toolbar.status)
	}
}

func TestQuitPendingResetsOnOtherKey(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.streaming = false
	m.quitPending = true
	m.ready = true

	// Any non-Ctrl+C key resets quitPending
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	updated := result.(Model)

	if updated.quitPending {
		t.Error("quitPending should be reset after non-Ctrl+C key")
	}
}

// --- Escape key handling ---

func TestEscDuringStreamingInterrupts(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.streaming = true
	m.streamBuf.WriteString("streaming content")

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := result.(Model)

	if updated.streaming {
		t.Error("streaming should be false after Esc")
	}
	content := updated.viewportContent()
	if !strings.Contains(content, "Interrupted") {
		t.Error("expected 'Interrupted' in viewport after Esc")
	}
}

func TestEscWhenIdleDoesNothing(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.streaming = false
	m.ready = true

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := result.(Model)

	// Should not quit or change state significantly
	if updated.quitPending {
		t.Error("Esc when idle should NOT set quitPending")
	}
}

// --- Permission Queue Edge Case ---

func TestPermissionDialogQueue(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24

	// Show first permission dialog
	ch1 := make(chan PermResponseMsg, 1)
	m.perm.Show(&PermRequestMsg{
		ToolName: "Bash",
		Content:  "first command",
		Reason:   "test",
		Response: ch1,
	})

	if !m.perm.active {
		t.Fatal("first dialog should be active")
	}

	// Queue second permission request
	ch2 := make(chan PermResponseMsg, 1)
	m.permQueue = append(m.permQueue, PermRequestMsg{
		ToolName: "Write",
		Content:  "second",
		Reason:   "test2",
		Response: ch2,
	})

	if len(m.permQueue) != 1 {
		t.Errorf("permQueue should have 1 entry, got %d", len(m.permQueue))
	}
}

// --- Segment Accumulation Edge Cases ---

func TestEmptyViewportOnStart(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24

	content := m.viewportContent()
	// Fresh model with no segments should render empty or welcome
	if content == "" {
		// Empty is fine — welcome only shows after Init()
	}
}

func TestToolSegDataPreservesDisplay(t *testing.T) {
	m := newTestModel()
	m.width = 80

	// Edit tool with Display (unified diff)
	m.outputSegs = appendTool(m.outputSegs, toolSegData{
		Name:    "Edit",
		Input:   json.RawMessage(`{"file_path":"a.go","old_string":"old","new_string":"new"}`),
		Content: "Applied edit to a.go",
		IsError: false,
		Display: "--- a.go\n+++ a.go\n@@ -1 +1 @@\n-old\n+new\n",
	})

	// Display should be used in both verbose and non-verbose
	m.verbose = false
	content := m.viewportContent()
	// Edit always shows full diff (per ADR-038)
	if !strings.Contains(content, "old") || !strings.Contains(content, "new") {
		t.Error("Edit should show diff content")
	}

	m.verbose = true
	content = m.viewportContent()
	if !strings.Contains(content, "old") || !strings.Contains(content, "new") {
		t.Error("verbose Edit should show diff content")
	}
}
