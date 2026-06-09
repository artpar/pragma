package query

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/artpar/pragma/internal/model"
)

func TestPragmaLoopSystemUsesServerGuidanceFromExistingPrompt(t *testing.T) {
	existing := model.SystemPrompt{Blocks: []model.SystemBlock{
		{Text: "codex prompt"},
		{Text: "## Server Commands\n\nTo start the server, run:\n```bash\n./run.sh\n```"},
	}}

	system := pragmaLoopSystemFromExisting(existing)
	if len(system.Blocks) != 1 {
		t.Fatalf("blocks = %d, want 1", len(system.Blocks))
	}
	text := system.Blocks[0].Text
	if strings.Contains(text, "codex prompt") {
		t.Fatal("system prompt included Codex prompt")
	}
	if strings.Contains(text, "THOUGHT") || strings.Contains(text, "reasoning process") || strings.Contains(text, "Your reasoning and analysis") {
		t.Fatalf("system prompt still teaches prose reasoning before actions: %q", text)
	}
	if !strings.Contains(text, "## Server Commands") {
		t.Fatal("system prompt did not include server guidance")
	}
	if strings.Count(text, "## Server Commands") != 1 {
		t.Fatalf("server guidance count = %d, want 1", strings.Count(text, "## Server Commands"))
	}
	if !strings.Contains(text, "rejected.\n\n\n## Server Commands") {
		t.Fatalf("server guidance separator did not match Pragma loop YAML append semantics: %q", text)
	}
}

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

func TestExtractPragmaLoopCommandMatchesPragmaLoopRegex(t *testing.T) {
	command, count := extractPragmaLoopCommand("THOUGHT: x\n\n```bash\nls -la\n```")
	if count != 1 || command != "ls -la" {
		t.Fatalf("command=%q count=%d", command, count)
	}

	_, count = extractPragmaLoopCommand("```sh\nls\n```")
	if count != 0 {
		t.Fatalf("sh action count = %d, want 0", count)
	}

	_, count = extractPragmaLoopCommand("```bash\nls\n```\n```bash\npwd\n```")
	if count != 2 {
		t.Fatalf("multi action count = %d, want 2", count)
	}
}

func TestPragmaLoopReplayContentDropsReasoning(t *testing.T) {
	content := pragmaLoopReplayContent([]model.ContentPart{
		model.ThinkingPart{Text: "provider reasoning"},
		model.TextPart{Text: "THOUGHT: visible\n```bash\nls\n```"},
	})
	if len(content) != 1 {
		t.Fatalf("content length = %d, want 1", len(content))
	}
	if text, ok := content[0].(model.TextPart); !ok || !strings.Contains(text.Text, "THOUGHT: visible") {
		t.Fatalf("content[0] = %#v, want visible text", content[0])
	}
}

func TestPragmaLoopSubmittedUsesOutputSentinel(t *testing.T) {
	ok, message := pragmaLoopSubmitted(pragmaLoopBashResult{
		ReturnCode: 0,
		Output:     "COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT\nfinal\n",
	})
	if !ok || message != "final\n" {
		t.Fatalf("submitted=%v message=%q", ok, message)
	}

	ok, _ = pragmaLoopSubmitted(pragmaLoopBashResult{
		ReturnCode: 1,
		Output:     "COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT\n",
	})
	if ok {
		t.Fatal("failed sentinel command should not submit")
	}
}

func TestRunPragmaLoopBashTimeoutUsesPragmaLoopTemplatePath(t *testing.T) {
	oldTimeout := pragmaLoopCommandTimeout
	pragmaLoopCommandTimeout = 50 * time.Millisecond
	defer func() { pragmaLoopCommandTimeout = oldTimeout }()

	result, timedOut := runPragmaLoopBash(t.Context(), t.TempDir(), "echo ready; sleep 5")
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

	result, timedOut := runPragmaLoopBash(t.Context(), t.TempDir(), "echo before; sleep 1; echo done")
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
	result, timedOut := runPragmaLoopBash(t.Context(), t.TempDir(), "false | true")
	if timedOut {
		t.Fatal("command timed out")
	}
	if result.ReturnCode == 0 {
		t.Fatalf("return code = 0, want non-zero with pipefail; output=%q", result.Output)
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
	result, timedOut := runPragmaLoopBash(t.Context(), dir, patchCommand)
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

func TestRunPragmaLoopBashRejectsDirectRepoSourceMutation(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir+"/internal", 0o755); err != nil {
		t.Fatal(err)
	}
	path := dir + "/internal/example.go"
	if err := os.WriteFile(path, []byte("package internal\n\nconst oldValue = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, timedOut := runPragmaLoopBash(t.Context(), dir, `sed -i 's/oldValue/newValue/' internal/example.go`)
	if timedOut {
		t.Fatal("command timed out")
	}
	if result.ReturnCode == 0 {
		t.Fatalf("return code = 0, want rejection; output=%q", result.Output)
	}
	if !strings.Contains(result.Output, "Use apply_patch") {
		t.Fatalf("output = %q, want apply_patch guidance", result.Output)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "newValue") {
		t.Fatalf("direct source mutation was applied: %q", data)
	}
}

func TestRunPragmaLoopBashAllowsRuntimeArtifactWrites(t *testing.T) {
	result, timedOut := runPragmaLoopBash(t.Context(), t.TempDir(), `mkdir -p /tmp/pragma && cat > /tmp/pragma/pragma-loop-artifact-test.txt <<'EOF'
artifact
EOF
cat /tmp/pragma/pragma-loop-artifact-test.txt`)
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

func pragmaLoopFieldPath(content, prefix string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}
