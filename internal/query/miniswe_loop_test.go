package query

import (
	"strings"
	"testing"
	"time"

	"github.com/artpar/pragma/internal/model"
)

func TestMiniSWESystemUsesServerGuidanceFromExistingPrompt(t *testing.T) {
	existing := model.SystemPrompt{Blocks: []model.SystemBlock{
		{Text: "codex prompt"},
		{Text: "## Server Commands\n\nTo start the server, run:\n```bash\n./run.sh\n```"},
	}}

	system := miniSWESystemFromExisting(existing)
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
		t.Fatalf("server guidance separator did not match Mini-SWE YAML append semantics: %q", text)
	}
}

func TestExtractMiniSWECommandMatchesMiniSWERegex(t *testing.T) {
	command, count := extractMiniSWECommand("THOUGHT: x\n\n```bash\nls -la\n```")
	if count != 1 || command != "ls -la" {
		t.Fatalf("command=%q count=%d", command, count)
	}

	_, count = extractMiniSWECommand("```sh\nls\n```")
	if count != 0 {
		t.Fatalf("sh action count = %d, want 0", count)
	}

	_, count = extractMiniSWECommand("```bash\nls\n```\n```bash\npwd\n```")
	if count != 2 {
		t.Fatalf("multi action count = %d, want 2", count)
	}
}

func TestMiniSWESubmittedUsesOutputSentinel(t *testing.T) {
	ok, message := miniSWESubmitted(miniSWEBashResult{
		ReturnCode: 0,
		Output:     "COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT\nfinal\n",
	})
	if !ok || message != "final\n" {
		t.Fatalf("submitted=%v message=%q", ok, message)
	}

	ok, _ = miniSWESubmitted(miniSWEBashResult{
		ReturnCode: 1,
		Output:     "COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT\n",
	})
	if ok {
		t.Fatal("failed sentinel command should not submit")
	}
}

func TestRunMiniSWEBashTimeoutUsesMiniSWETemplatePath(t *testing.T) {
	oldTimeout := miniSWECommandTimeout
	miniSWECommandTimeout = 50 * time.Millisecond
	defer func() { miniSWECommandTimeout = oldTimeout }()

	result, timedOut := runMiniSWEBash(t.Context(), t.TempDir(), "echo ready; sleep 5")
	if !timedOut {
		t.Fatal("command did not time out")
	}
	if !strings.Contains(result.Output, "ready") {
		t.Fatalf("timeout output = %q, want captured stdout", result.Output)
	}
	if strings.Contains(result.Output, "WaitDelay") {
		t.Fatalf("timeout output contains WaitDelay: %q", result.Output)
	}
	message := formatMiniSWETimeout("echo ready; sleep 5", result.Output)
	if !strings.Contains(message, "timed out and has been killed") {
		t.Fatalf("timeout message = %q", message)
	}
}
