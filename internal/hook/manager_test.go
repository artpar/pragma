package hook

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func setupHookConfig(t *testing.T, dir string, hooks map[string][]Entry) {
	t.Helper()
	gogentDir := filepath.Join(dir, ".pragma")
	if err := os.MkdirAll(gogentDir, 0o755); err != nil {
		t.Fatal(err)
	}

	hooksJSON, _ := json.Marshal(hooks)
	settings := `{"hooks":` + string(hooksJSON) + `}`
	if err := os.WriteFile(filepath.Join(gogentDir, "settings.json"), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestManagerPreToolUseBlock(t *testing.T) {
	dir := t.TempDir()

	// Create a hook script that blocks rm commands
	scriptPath := filepath.Join(dir, "check-bash.sh")
	script := `#!/bin/bash
read input
tool=$(echo "$input" | grep -o '"tool_name":"[^"]*"' | cut -d'"' -f4)
if [ "$tool" = "Bash" ]; then
  echo "dangerous command blocked" >&2
  exit 2
fi
exit 0
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	setupHookConfig(t, dir, map[string][]Entry{
		"PreToolUse": {
			{Matcher: "Bash", Hooks: []Command{{Type: "command", Command: scriptPath}}},
		},
	})

	mgr := NewManager(dir, "test-session", nil)

	result := mgr.Execute(context.Background(), PreToolUse, HookInput{
		ToolName:  "Bash",
		ToolInput: json.RawMessage(`{"command":"rm -rf /"}`),
	})

	if !result.Blocked {
		t.Error("expected hook to block Bash tool")
	}
	if result.BlockMsg == "" {
		t.Error("expected non-empty block message")
	}
}

func TestManagerPreToolUseAllow(t *testing.T) {
	dir := t.TempDir()

	scriptPath := filepath.Join(dir, "allow.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/bash\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	setupHookConfig(t, dir, map[string][]Entry{
		"PreToolUse": {
			{Matcher: "Bash", Hooks: []Command{{Type: "command", Command: scriptPath}}},
		},
	})

	mgr := NewManager(dir, "test-session", nil)

	result := mgr.Execute(context.Background(), PreToolUse, HookInput{
		ToolName:  "Bash",
		ToolInput: json.RawMessage(`{"command":"ls"}`),
	})

	if result.Blocked {
		t.Error("expected hook to allow")
	}
}

func TestManagerMatcherFiltering(t *testing.T) {
	dir := t.TempDir()

	scriptPath := filepath.Join(dir, "block.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/bash\necho 'blocked' >&2; exit 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	setupHookConfig(t, dir, map[string][]Entry{
		"PreToolUse": {
			{Matcher: "Bash", Hooks: []Command{{Type: "command", Command: scriptPath}}},
		},
	})

	mgr := NewManager(dir, "test-session", nil)

	// Read tool should NOT match the Bash matcher
	result := mgr.Execute(context.Background(), PreToolUse, HookInput{
		ToolName:  "Read",
		ToolInput: json.RawMessage(`{"file_path":"/etc/passwd"}`),
	})

	if result.Blocked {
		t.Error("Read tool should not be blocked by Bash matcher")
	}
}

func TestManagerNoHooksConfigured(t *testing.T) {
	dir := t.TempDir()

	mgr := NewManager(dir, "test-session", nil)

	result := mgr.Execute(context.Background(), PreToolUse, HookInput{
		ToolName: "Bash",
	})

	if result.Blocked {
		t.Error("should not block when no hooks configured")
	}
}

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		pattern string
		value   string
		want    bool
	}{
		{"", "anything", true},             // empty = match all
		{"Bash", "Bash", true},             // exact match
		{"Bash", "Read", false},            // no match
		{"Bash|Read", "Read", true},        // pipe OR
		{"Bash|Read", "Write", false},      // pipe OR miss
		{"/^Bash/", "BashTool", true},      // regex
		{"/^Bash$/", "BashTool", false},    // regex anchored
	}

	for _, tt := range tests {
		got := matchesPattern(tt.pattern, tt.value)
		if got != tt.want {
			t.Errorf("matchesPattern(%q, %q) = %v, want %v", tt.pattern, tt.value, got, tt.want)
		}
	}
}

func TestManagerStopHook(t *testing.T) {
	dir := t.TempDir()

	scriptPath := filepath.Join(dir, "stop.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/bash\necho 'session ending'\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	setupHookConfig(t, dir, map[string][]Entry{
		"Stop": {
			{Hooks: []Command{{Type: "command", Command: scriptPath}}},
		},
	})

	mgr := NewManager(dir, "test-session", nil)

	result := mgr.Execute(context.Background(), Stop, HookInput{})

	if result.Blocked {
		t.Error("Stop hook with exit 0 should not block")
	}
	if result.Stdout == "" {
		t.Error("expected stdout from stop hook")
	}
}
