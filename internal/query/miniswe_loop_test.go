package query

import (
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

func TestPragmaLoopInstancePromptMatchesPragmaLoopYamlSedBoundary(t *testing.T) {
	prompt := pragmaLoopInstancePrompt("do the task", "/work")
	if !strings.Contains(prompt, "Current working directory: /work") {
		t.Fatalf("instance prompt does not include current working directory: %q", prompt)
	}
	if strings.Contains(prompt, "MY_ENV_VAR=MY_VALUE cd /path/to/working/dir") {
		t.Fatalf("instance prompt contains invalid env/cd example: %q", prompt)
	}
	if !strings.Contains(prompt, "### Edit files with sed:```bash\n# Replace all occurrences") {
		t.Fatalf("instance prompt does not match Pragma loop YAML whitespace at sed boundary")
	}
	if strings.Contains(prompt, "### Edit files with sed:\n\n```bash") {
		t.Fatalf("instance prompt has hand-copied whitespace drift before sed example")
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
