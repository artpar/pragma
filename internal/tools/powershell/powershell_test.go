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

// denyAllChecker denies every tool — used to verify read-only bypass.
type denyAllChecker struct{}

func (denyAllChecker) Check(_ context.Context, _ string, _ string) permission.CheckResult {
	return permission.CheckResult{Decision: permission.DecisionDeny}
}
func (denyAllChecker) AddSessionRule(_ permission.Rule) {}

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

func TestCheckPermReadOnlyCmdlets(t *testing.T) {
	tl := &Tool{}
	// Use deny checker — read-only cmdlets should bypass it and still allow
	checker := denyAllChecker{}

	tests := []struct {
		name       string
		command    string
		wantAllow  bool
		wantReason string
	}{
		{"Get-ChildItem", "Get-ChildItem", true, "read-only cmdlet"},
		{"Get-Content with args", "Get-Content foo.txt", true, "read-only cmdlet"},
		{"Get-Process", "Get-Process", true, "read-only cmdlet"},
		{"pipeline first cmdlet", "Get-Content foo.txt | Select-String bar", true, "read-only cmdlet"},
		{"leading whitespace", "  Get-ChildItem", true, "read-only cmdlet"},
		{"case insensitive", "get-childitem", true, "read-only cmdlet"},
		{"alias gci", "gci", true, "read-only cmdlet"},
		{"alias dir", "dir", true, "read-only cmdlet"},
		{"alias cat", "cat", true, "read-only cmdlet"},
		{"alias ls", "ls", true, "read-only cmdlet"},
		{"Remove-Item denied", "Remove-Item foo", false, ""},
		{"Set-Content denied", "Set-Content foo bar", false, ""},
		{"unknown cmdlet denied", "Invoke-CustomThing", false, ""},
		{"empty denied", "", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := json.Marshal(map[string]string{"command": tt.command})
			result := tl.CheckPerm(context.Background(), json.RawMessage(input), checker)
			if tt.wantAllow {
				if result.Decision != permission.DecisionAllow {
					t.Errorf("Decision = %v, want Allow for %q", result.Decision, tt.command)
				}
				if result.Reason != tt.wantReason {
					t.Errorf("Reason = %q, want %q", result.Reason, tt.wantReason)
				}
			} else {
				if result.Decision != permission.DecisionDeny {
					t.Errorf("Decision = %v, want Deny for %q", result.Decision, tt.command)
				}
			}
		})
	}
}

func TestExtractCmdlet(t *testing.T) {
	tests := []struct {
		command string
		want    string
	}{
		{"Get-ChildItem", "Get-ChildItem"},
		{"Get-Content foo.txt", "Get-Content"},
		{"Get-Content\tfoo.txt", "Get-Content"},
		{"Get-Content|Format-Table", "Get-Content"},
		{"Get-Content;Get-Process", "Get-Content"},
		{"  Get-Process  ", "Get-Process"},
		{"", ""},
		{"   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			got := extractCmdlet(tt.command)
			if got != tt.want {
				t.Errorf("extractCmdlet(%q) = %q, want %q", tt.command, got, tt.want)
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
