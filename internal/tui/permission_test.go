package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artpar/gogent/internal/permission"
)

func TestPermissionDialogNavigation(t *testing.T) {
	d := newPermissionDialog()
	respCh := make(chan PermResponseMsg, 1)
	d.Show(&PermRequestMsg{
		ToolName: "Bash",
		Content:  "rm -rf /tmp/test",
		Reason:   "dangerous command",
		Response: respCh,
	})

	if !d.active {
		t.Fatal("dialog should be active")
	}
	if d.selected != permOptAllow {
		t.Errorf("initial selection should be Allow (0), got %d", d.selected)
	}

	// Navigate down
	d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.selected != permOptDeny {
		t.Errorf("after down, expected Deny (1), got %d", d.selected)
	}

	// Navigate down again
	d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.selected != permOptAlwaysAllow {
		t.Errorf("after second down, expected AlwaysAllow (2), got %d", d.selected)
	}

	// Wrap around
	d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.selected != permOptAllow {
		t.Errorf("after third down, expected wrap to Allow (0), got %d", d.selected)
	}

	// Navigate up wraps
	d.Update(tea.KeyMsg{Type: tea.KeyUp})
	if d.selected != permOptAlwaysAllow {
		t.Errorf("after up from 0, expected wrap to AlwaysAllow (2), got %d", d.selected)
	}
}

func TestPermissionDialogAllow(t *testing.T) {
	d := newPermissionDialog()
	respCh := make(chan PermResponseMsg, 1)
	d.Show(&PermRequestMsg{
		ToolName: "Bash",
		Content:  "ls",
		Reason:   "list files",
		Response: respCh,
	})

	// Select Allow and confirm
	cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if d.active {
		t.Fatal("dialog should be deactivated after confirm")
	}

	// Execute the cmd to send the response
	if cmd != nil {
		cmd()
	}

	resp := <-respCh
	if resp.Decision != permission.DecisionAllow {
		t.Errorf("expected Allow, got %s", resp.Decision)
	}
	if resp.Rule != nil {
		t.Error("Allow (once) should not create a rule")
	}
}

func TestPermissionDialogAlwaysAllow(t *testing.T) {
	d := newPermissionDialog()
	respCh := make(chan PermResponseMsg, 1)
	d.Show(&PermRequestMsg{
		ToolName: "FileWrite",
		Content:  "/tmp/test.go",
		Reason:   "write file",
		Response: respCh,
	})

	// Navigate to Always Allow
	d.Update(tea.KeyMsg{Type: tea.KeyDown}) // Deny
	d.Update(tea.KeyMsg{Type: tea.KeyDown}) // Always Allow
	cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd != nil {
		cmd()
	}

	resp := <-respCh
	if resp.Decision != permission.DecisionAllow {
		t.Errorf("expected Allow, got %s", resp.Decision)
	}
	if resp.Rule == nil {
		t.Fatal("Always Allow should create a session rule")
	}
	if resp.Rule.ToolName != "FileWrite" {
		t.Errorf("rule tool name should be FileWrite, got %s", resp.Rule.ToolName)
	}
	if resp.Rule.Source != permission.SourceSession {
		t.Errorf("rule source should be session, got %s", resp.Rule.Source)
	}
}

func TestPermissionDialogEscapeDenies(t *testing.T) {
	d := newPermissionDialog()
	respCh := make(chan PermResponseMsg, 1)
	d.Show(&PermRequestMsg{
		ToolName: "Bash",
		Content:  "danger",
		Reason:   "test",
		Response: respCh,
	})

	cmd := d.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if d.active {
		t.Fatal("dialog should be deactivated after escape")
	}

	if cmd != nil {
		cmd()
	}

	resp := <-respCh
	if resp.Decision != permission.DecisionDeny {
		t.Errorf("expected Deny on escape, got %s", resp.Decision)
	}
}

func TestPermissionDialogView(t *testing.T) {
	d := newPermissionDialog()

	// Inactive dialog should render empty
	if d.View() != "" {
		t.Error("inactive dialog should render empty string")
	}

	respCh := make(chan PermResponseMsg, 1)
	d.Show(&PermRequestMsg{
		ToolName: "Bash",
		Content:  "echo hello",
		Reason:   "test command",
		Response: respCh,
	})

	view := d.View()
	if !strings.Contains(view, "Permission Required") {
		t.Error("expected title in view")
	}
	if !strings.Contains(view, "Bash") {
		t.Error("expected tool name in view")
	}
	if !strings.Contains(view, "echo hello") {
		t.Error("expected content in view")
	}
	if !strings.Contains(view, "Allow") {
		t.Error("expected Allow option in view")
	}
	if !strings.Contains(view, "Deny") {
		t.Error("expected Deny option in view")
	}
}
