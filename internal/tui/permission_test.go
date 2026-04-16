package tui

import (
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artpar/pragma/internal/permission"
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
	if d.selected != permOptAlwaysAllow {
		t.Errorf("after down, expected AlwaysAllow (1), got %d", d.selected)
	}

	// Navigate down again
	d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.selected != permOptDeny {
		t.Errorf("after second down, expected Deny (2), got %d", d.selected)
	}

	// Wrap around
	d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.selected != permOptAllow {
		t.Errorf("after third down, expected wrap to Allow (0), got %d", d.selected)
	}

	// Navigate up wraps
	d.Update(tea.KeyMsg{Type: tea.KeyUp})
	if d.selected != permOptDeny {
		t.Errorf("after up from 0, expected wrap to Deny (2), got %d", d.selected)
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

	// Navigate to Always Allow (index 1)
	d.Update(tea.KeyMsg{Type: tea.KeyDown}) // AlwaysAllow
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

func TestPermissionDialogDeny(t *testing.T) {
	d := newPermissionDialog()
	respCh := make(chan PermResponseMsg, 1)
	d.Show(&PermRequestMsg{
		ToolName: "Bash",
		Content:  "danger",
		Reason:   "test",
		Response: respCh,
	})

	// Navigate to No (index 2)
	d.Update(tea.KeyMsg{Type: tea.KeyDown}) // AlwaysAllow
	d.Update(tea.KeyMsg{Type: tea.KeyDown}) // Deny
	cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if d.active {
		t.Fatal("dialog should be deactivated after confirm")
	}

	if cmd != nil {
		cmd()
	}

	resp := <-respCh
	if resp.Decision != permission.DecisionDeny {
		t.Errorf("expected Deny, got %s", resp.Decision)
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

func TestPermissionDialogViewBash(t *testing.T) {
	d := newPermissionDialog()

	// Inactive dialog should render empty
	if d.View() != "" {
		t.Error("inactive dialog should render empty string")
	}

	respCh := make(chan PermResponseMsg, 1)
	d.Show(&PermRequestMsg{
		ToolName:  "Bash",
		ToolInput: json.RawMessage(`{"command":"echo hello"}`),
		Content:   "echo hello",
		Reason:    "test command",
		Response:  respCh,
	})

	view := d.View()
	if !strings.Contains(view, "Run command") {
		t.Error("expected 'Run command' title in view")
	}
	if !strings.Contains(view, "echo hello") {
		t.Error("expected command in view")
	}
	if !strings.Contains(view, "Yes") {
		t.Error("expected Yes option in view")
	}
	if !strings.Contains(view, "No") {
		t.Error("expected No option in view")
	}
	if !strings.Contains(view, "session") {
		t.Error("expected session option in view")
	}
}

func TestPermissionDialogViewEdit(t *testing.T) {
	d := newPermissionDialog()
	respCh := make(chan PermResponseMsg, 1)
	d.Show(&PermRequestMsg{
		ToolName:  "Edit",
		ToolInput: json.RawMessage(`{"file_path":"src/main.go","old_string":"foo","new_string":"bar"}`),
		Content:   "src/main.go",
		Reason:    "edit file",
		Response:  respCh,
	})

	view := d.View()
	if !strings.Contains(view, "Edit file") {
		t.Error("expected 'Edit file' title")
	}
	if !strings.Contains(view, "src/main.go") {
		t.Error("expected file path subtitle")
	}
	if !strings.Contains(view, "foo") {
		t.Error("expected old string in diff")
	}
	if !strings.Contains(view, "bar") {
		t.Error("expected new string in diff")
	}
}

func TestPermissionDialogViewWrite(t *testing.T) {
	d := newPermissionDialog()
	respCh := make(chan PermResponseMsg, 1)
	d.Show(&PermRequestMsg{
		ToolName:  "Write",
		ToolInput: json.RawMessage(`{"file_path":"new.go","content":"package main\n\nfunc main() {}"}`),
		Content:   "new.go",
		Reason:    "create file",
		Response:  respCh,
	})

	view := d.View()
	if !strings.Contains(view, "Write file") {
		t.Error("expected 'Write file' title")
	}
	if !strings.Contains(view, "new.go") {
		t.Error("expected file path subtitle")
	}
	if !strings.Contains(view, "package main") {
		t.Error("expected content preview")
	}
}

func TestPermissionDialogViewDefault(t *testing.T) {
	d := newPermissionDialog()
	respCh := make(chan PermResponseMsg, 1)
	d.Show(&PermRequestMsg{
		ToolName: "WebFetch",
		Content:  "https://example.com",
		Reason:   "fetch url",
		Response: respCh,
	})

	view := d.View()
	if !strings.Contains(view, "Tool use") {
		t.Error("expected 'Tool use' title for unknown tool")
	}
	if !strings.Contains(view, "WebFetch") {
		t.Error("expected tool name in content")
	}
	if !strings.Contains(view, "https://example.com") {
		t.Error("expected content in view")
	}
}

func TestPermissionDialogViewNilInput(t *testing.T) {
	// Verify graceful degradation when ToolInput is nil
	d := newPermissionDialog()
	respCh := make(chan PermResponseMsg, 1)
	d.Show(&PermRequestMsg{
		ToolName:  "Edit",
		ToolInput: nil,
		Content:   "src/main.go",
		Reason:    "edit file",
		Response:  respCh,
	})

	view := d.View()
	if !strings.Contains(view, "Edit file") {
		t.Error("expected 'Edit file' title even with nil input")
	}
	// Should still render without panic
	if view == "" {
		t.Error("expected non-empty view")
	}
}

func TestParseToolPreview(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		input    json.RawMessage
		want     toolPreview
	}{
		{
			name:     "Edit",
			toolName: "Edit",
			input:    json.RawMessage(`{"file_path":"a.go","old_string":"old","new_string":"new"}`),
			want:     toolPreview{FilePath: "a.go", OldString: "old", NewString: "new"},
		},
		{
			name:     "Write",
			toolName: "Write",
			input:    json.RawMessage(`{"file_path":"b.go","content":"hello"}`),
			want:     toolPreview{FilePath: "b.go", Content: "hello"},
		},
		{
			name:     "Bash",
			toolName: "Bash",
			input:    json.RawMessage(`{"command":"ls -la"}`),
			want:     toolPreview{Command: "ls -la"},
		},
		{
			name:     "nil input",
			toolName: "Edit",
			input:    nil,
			want:     toolPreview{},
		},
		{
			name:     "invalid JSON",
			toolName: "Edit",
			input:    json.RawMessage(`not json`),
			want:     toolPreview{},
		},
		{
			name:     "unknown tool",
			toolName: "Unknown",
			input:    json.RawMessage(`{"foo":"bar"}`),
			want:     toolPreview{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseToolPreview(tt.toolName, tt.input)
			if got != tt.want {
				t.Errorf("parseToolPreview(%q, %s) = %+v, want %+v", tt.toolName, tt.input, got, tt.want)
			}
		})
	}
}

func TestRenderEditPreviewTruncation(t *testing.T) {
	// Create a preview with many lines to verify truncation
	oldLines := make([]string, 15)
	for i := range oldLines {
		oldLines[i] = "old line"
	}
	newLines := make([]string, 15)
	for i := range newLines {
		newLines[i] = "new line"
	}

	p := toolPreview{
		OldString: strings.Join(oldLines, "\n"),
		NewString: strings.Join(newLines, "\n"),
	}
	result := renderEditPreview(p)

	// Should contain truncation indicators
	if !strings.Contains(result, "...") {
		t.Error("expected truncation indicator for large diff")
	}
	// Should not be empty
	if result == "" {
		t.Error("expected non-empty result")
	}
}
