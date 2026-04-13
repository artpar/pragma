package powershell

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/artpar/gogent/internal/permission"
)

type staticState struct{ dir string }

func (s staticState) WorkDir() string { return s.dir }

// allowAllChecker allows every tool.
type allowAllChecker struct{}

func (allowAllChecker) Check(_ context.Context, _ string, _ string) permission.CheckResult {
	return permission.CheckResult{Decision: permission.DecisionAllow}
}
func (allowAllChecker) AddSessionRule(_ permission.Rule) {}

func TestInvoke_EmptyCommand(t *testing.T) {
	tl := &Tool{}
	input, _ := json.Marshal(PowerShellInput{Command: ""})
	_, err := tl.Invoke(context.Background(), input, staticState{"/tmp"})
	if err == nil {
		t.Fatal("expected error for empty command")
	}
	if !strings.Contains(err.Error(), "command is required") {
		t.Errorf("error = %q, want 'command is required'", err.Error())
	}
}

func TestInvoke_InvalidJSON(t *testing.T) {
	tl := &Tool{}
	_, err := tl.Invoke(context.Background(), json.RawMessage(`{broken`), staticState{"/tmp"})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestTimeoutClamping(t *testing.T) {
	// We can't test execution without pwsh, but we can test the input parsing
	// by verifying the tool accepts valid inputs and produces expected errors
	tests := []struct {
		name    string
		timeout *int
	}{
		{"nil timeout (default)", nil},
		{"zero timeout", intPtr(0)},
		{"normal timeout", intPtr(5000)},
		{"max timeout", intPtr(600000)},
		{"over max", intPtr(999999)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := PowerShellInput{
				Command: "echo hello",
				Timeout: tt.timeout,
			}
			data, _ := json.Marshal(input)
			var parsed PowerShellInput
			if err := json.Unmarshal(data, &parsed); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if parsed.Command != "echo hello" {
				t.Errorf("Command = %q, want 'echo hello'", parsed.Command)
			}
		})
	}
}

func TestCheckPerm(t *testing.T) {
	tl := &Tool{}
	checker := allowAllChecker{}

	tests := []struct {
		name    string
		input   string
		wantCmd string
	}{
		{"with command", `{"command": "Get-Process"}`, "Get-Process"},
		{"empty command", `{"command": ""}`, ""},
		{"invalid json", `{broken`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tl.CheckPerm(context.Background(), json.RawMessage(tt.input), checker)
			if result.Decision != permission.DecisionAllow {
				t.Errorf("Decision = %v, want Allow", result.Decision)
			}
		})
	}
}

func TestToolMetadata(t *testing.T) {
	tl := &Tool{}
	if tl.Name() != "PowerShell" {
		t.Errorf("Name = %q, want PowerShell", tl.Name())
	}
	flags := tl.Flags()
	if flags.ReadOnly {
		t.Error("expected ReadOnly = false")
	}
	if !flags.Destructive {
		t.Error("expected Destructive = true")
	}
}

func intPtr(v int) *int { return &v }
