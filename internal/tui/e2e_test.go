package tui

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/slash"
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

	return New(Config{
		Store:       store,
		CostTracker: costTracker,
		Metrics:     metrics,
		ModelName:   "test-model",
		Provider:    "test",
		SlashCmds:   slashCmds,
		SlashDeps:   slashDeps,
	})
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
