package query

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPragmaLoopInstancePromptIncludesTaskAndWorkdir(t *testing.T) {
	prompt := pragmaLoopInstancePrompt("do the task", "/work")
	if !strings.Contains(prompt, "Current working directory: /work") {
		t.Fatalf("instance prompt does not include current working directory: %q", prompt)
	}
	if !strings.Contains(prompt, "Please solve this task: do the task") {
		t.Fatalf("instance prompt does not include task: %q", prompt)
	}
	if strings.Contains(prompt, "MY_ENV_VAR=MY_VALUE cd /path/to/working/dir") {
		t.Fatalf("instance prompt contains invalid env/cd example: %q", prompt)
	}
	if strings.Contains(prompt, "THOUGHT") || strings.Contains(prompt, "Here are some thoughts") {
		t.Fatalf("instance prompt still contains reasoning prose examples: %q", prompt)
	}

	msg := fmt.Sprintf(pragmaLoopFormatErrorTemplate, 0)
	if strings.Contains(msg, "THOUGHT") || strings.Contains(msg, "thoughts") {
		t.Fatalf("format error still asks for reasoning prose: %q", msg)
	}
	if !strings.Contains(msg, "no prose outside the code block") {
		t.Fatalf("format error does not restate action-only contract: %q", msg)
	}
}

func TestRunPragmaLoopBashTimeoutUsesPragmaLoopTemplatePath(t *testing.T) {
	oldTimeout := pragmaLoopCommandTimeout
	pragmaLoopCommandTimeout = 50 * time.Millisecond
	defer func() { pragmaLoopCommandTimeout = oldTimeout }()

	result, timedOut, err := runPragmaLoopBash(t.Context(), t.TempDir(), "echo ready; sleep 5")
	if err != nil {
		t.Fatalf("runPragmaLoopBash: %v", err)
	}
	if !timedOut {
		t.Fatal("command did not time out")
	}
	if !strings.Contains(result.Output, "ready") {
		t.Fatalf("timeout output = %q, want captured stdout", result.Output)
	}
	if strings.Contains(result.Output, "WaitDelay") {
		t.Fatalf("timeout output contains WaitDelay: %q", result.Output)
	}
	message := formatPragmaLoopTimeout("echo ready; sleep 5", result.Output)
	if !strings.Contains(message, "timed out and has been killed") {
		t.Fatalf("timeout message = %q", message)
	}
}

func TestRunPragmaLoopBashSoftWaitReturnsRunningProcess(t *testing.T) {
	oldWait := pragmaLoopForegroundWait
	oldTimeout := pragmaLoopCommandTimeout
	pragmaLoopForegroundWait = 50 * time.Millisecond
	pragmaLoopCommandTimeout = 2 * time.Second
	defer func() {
		pragmaLoopForegroundWait = oldWait
		pragmaLoopCommandTimeout = oldTimeout
	}()

	result, timedOut, err := runPragmaLoopBash(t.Context(), t.TempDir(), "echo before; sleep 1; echo done")
	if err != nil {
		t.Fatalf("runPragmaLoopBash: %v", err)
	}
	if timedOut {
		t.Fatal("command hard-timed out")
	}
	for _, want := range []string{"Console tail (last 100 lines):", "before", "Command is still running", "Active processes:", "Status:", "Console:"} {
		if !strings.Contains(result.Output, want) {
			t.Fatalf("output missing %q: %q", want, result.Output)
		}
	}

	statusPath := pragmaLoopFieldPath(result.Output, "Status:")
	consolePath := pragmaLoopFieldPath(result.Output, "Console:")
	if statusPath == "" || consolePath == "" {
		t.Fatalf("missing paths in output: %s", result.Output)
	}

	var status string
	for i := 0; i < 30; i++ {
		data, _ := os.ReadFile(statusPath)
		status = string(data)
		if strings.Contains(status, "exited") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(status, "exited") || !strings.Contains(status, "exit_code=0") {
		t.Fatalf("status = %q", status)
	}
	console, err := os.ReadFile(consolePath)
	if err != nil {
		t.Fatalf("read console: %v", err)
	}
	if !strings.Contains(string(console), "done") {
		t.Fatalf("console = %q, want command to complete after soft wait", console)
	}
}

func TestRunPragmaLoopBashUsesPipefail(t *testing.T) {
	result, timedOut, err := runPragmaLoopBash(t.Context(), t.TempDir(), "false | true")
	if err != nil {
		t.Fatalf("runPragmaLoopBash: %v", err)
	}
	if timedOut {
		t.Fatal("command timed out")
	}
	if result.ReturnCode == 0 {
		t.Fatalf("return code = 0, want non-zero with pipefail; output=%q", result.Output)
	}
}

func TestRunPragmaLoopBashReturnsShellStartupError(t *testing.T) {
	missingWorkDir := t.TempDir() + "/missing"
	result, timedOut, err := runPragmaLoopBash(t.Context(), missingWorkDir, "echo should-not-run")
	if err == nil {
		t.Fatalf("runPragmaLoopBash error = nil, want startup error; result=%+v timedOut=%v", result, timedOut)
	}
	if timedOut {
		t.Fatal("timedOut = true, want false for startup error")
	}
	for _, want := range []string{"shell command failed before execution", "start command"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
}

func TestRunPragmaLoopBashRoutesShellApplyPatch(t *testing.T) {
	dir := t.TempDir()
	path := "internal/example.go"
	full := dir + "/" + path
	if err := os.MkdirAll(dir+"/internal", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("package internal\n\nconst oldValue = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	patchCommand := strings.Join([]string{
		"apply_patch <<'PATCH'",
		"*** Begin Patch",
		"*** Update File: internal/example.go",
		"@@",
		" package internal",
		" ",
		"-const oldValue = 1",
		"+const oldValue = 2",
		"*** End Patch",
		"PATCH",
	}, "\n")
	result, timedOut, err := runPragmaLoopBash(t.Context(), dir, patchCommand)
	if err != nil {
		t.Fatalf("runPragmaLoopBash: %v", err)
	}
	if timedOut {
		t.Fatal("command timed out")
	}
	if result.ReturnCode != 0 {
		t.Fatalf("return code = %d, output=%q", result.ReturnCode, result.Output)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "const oldValue = 2") {
		t.Fatalf("file was not patched: %q", data)
	}
}

func TestRunPragmaLoopBashAllowsDirectRepoSourceWrites(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir+"/internal", 0o755); err != nil {
		t.Fatal(err)
	}
	path := dir + "/internal/example.go"
	if err := os.WriteFile(path, []byte("package internal\n\nconst oldValue = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, timedOut, err := runPragmaLoopBash(t.Context(), dir, `cat > internal/example.go <<'EOF'
package internal

const newValue = 2
EOF`)
	if err != nil {
		t.Fatalf("runPragmaLoopBash: %v", err)
	}
	if timedOut {
		t.Fatal("command timed out")
	}
	if result.ReturnCode != 0 {
		t.Fatalf("return code = %d, output=%q", result.ReturnCode, result.Output)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "const newValue = 2") {
		t.Fatalf("direct source write was not applied: %q", data)
	}
}

func TestRunPragmaLoopBashAllowsRuntimeArtifactWrites(t *testing.T) {
	result, timedOut, err := runPragmaLoopBash(t.Context(), t.TempDir(), `mkdir -p /tmp/pragma && cat > /tmp/pragma/pragma-loop-artifact-test.txt <<'EOF'
artifact
EOF
cat /tmp/pragma/pragma-loop-artifact-test.txt`)
	if err != nil {
		t.Fatalf("runPragmaLoopBash: %v", err)
	}
	if timedOut {
		t.Fatal("command timed out")
	}
	if result.ReturnCode != 0 {
		t.Fatalf("return code = %d, output=%q", result.ReturnCode, result.Output)
	}
	if !strings.Contains(result.Output, "artifact") {
		t.Fatalf("output = %q, want artifact content", result.Output)
	}
}

func TestExtractPragmaLoopCommandKeepsNestedMarkdownFenceInHeredoc(t *testing.T) {
	text := strings.Join([]string{
		"```bash",
		"cat > /tmp/pragma/handoff-prompts/next_item/current-item.md <<'EOF'",
		"# Current Item Handoff",
		"",
		"## Selected Item JSON",
		"```json",
		"{",
		`  "id": "kubernetes-config-struct"`,
		"}",
		"```",
		"EOF",
		"echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT",
		"```",
	}, "\n")

	command, count := extractPragmaLoopCommand(text)
	if count != 1 {
		t.Fatalf("count = %d, want 1; command=%q", count, command)
	}
	if !strings.Contains(command, "```json") {
		t.Fatalf("command lost nested json fence: %q", command)
	}
	if !strings.Contains(command, "\n```\nEOF\n") {
		t.Fatalf("command lost heredoc closing content: %q", command)
	}
	if !strings.Contains(command, "echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT") {
		t.Fatalf("command lost completion marker: %q", command)
	}
}

func TestExtractPragmaLoopCommandParserCases(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		wantCount int
		want      string
	}{
		{
			name: "simple bash block",
			text: strings.Join([]string{
				"```bash",
				"echo ok",
				"```",
			}, "\n"),
			wantCount: 1,
			want:      "echo ok",
		},
		{
			name: "multiple bash blocks rejected by count",
			text: strings.Join([]string{
				"```bash",
				"echo one",
				"```",
				"```bash",
				"echo two",
				"```",
			}, "\n"),
			wantCount: 2,
		},
		{
			name: "double quoted heredoc delimiter",
			text: strings.Join([]string{
				"```bash",
				`cat <<"JSON"`,
				"```json",
				"{}",
				"```",
				"JSON",
				"echo done",
				"```",
			}, "\n"),
			wantCount: 1,
			want: strings.Join([]string{
				`cat <<"JSON"`,
				"```json",
				"{}",
				"```",
				"JSON",
				"echo done",
			}, "\n"),
		},
		{
			name: "tab stripping heredoc delimiter",
			text: strings.Join([]string{
				"```bash",
				"cat <<-EOF",
				"```json",
				"{}",
				"```",
				"\tEOF",
				"echo done",
				"```",
			}, "\n"),
			wantCount: 1,
			want: strings.Join([]string{
				"cat <<-EOF",
				"```json",
				"{}",
				"```",
				"\tEOF",
				"echo done",
			}, "\n"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, count := extractPragmaLoopCommand(tt.text)
			if count != tt.wantCount {
				t.Fatalf("count = %d, want %d; command=%q", count, tt.wantCount, command)
			}
			if tt.want != "" && command != tt.want {
				t.Fatalf("command = %q, want %q", command, tt.want)
			}
		})
	}
}

func pragmaLoopFieldPath(content, prefix string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}
