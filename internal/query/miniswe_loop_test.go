package query

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
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

func TestPragmaLoopTextOnlyResponseCompletesWithoutRetry(t *testing.T) {
	response := model.Response{
		ID:         "resp-text-only",
		Model:      "test-model",
		StopReason: model.StopEndTurn,
		Content: []model.ContentPart{
			model.TextPart{Text: "I can help with that."},
		},
	}

	prov := &pragmaLoopTestProvider{responses: []model.Response{response}}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          conv.WorkDir,
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	engine := NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(16), EngineConfig{
		Model:     "test-model",
		MaxTokens: 4096,
		MaxTurns:  5,
	})

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "hello"))

	var sawText, sawComplete, sawTool bool
	for _, ev := range events {
		switch e := ev.(type) {
		case ErrorEvent:
			t.Fatalf("unexpected ErrorEvent: %v", e.Err)
		case TextEvent:
			if e.Text == "I can help with that." {
				sawText = true
			}
		case TurnCompleteEvent:
			sawComplete = true
			if e.StopReason != model.StopEndTurn {
				t.Fatalf("turn complete stop reason = %q, want %q", e.StopReason, model.StopEndTurn)
			}
		case ToolCallEvent, ToolResultEvent:
			sawTool = true
		}
	}
	if !sawText {
		t.Fatal("missing TextEvent for final assistant response")
	}
	if !sawComplete {
		t.Fatal("missing TurnCompleteEvent")
	}
	if sawTool {
		t.Fatal("unexpected tool event for text-only response")
	}
	if prov.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", prov.calls)
	}
}

func TestPragmaLoopNoActionFinalRunsCompletionCheck(t *testing.T) {
	responses := []model.Response{
		{
			ID:         "resp-no-action",
			Model:      "test-model",
			StopReason: model.StopEndTurn,
			Content: []model.ContentPart{
				model.TextPart{Text: "I am done."},
			},
		},
		{
			ID:         "resp-repair",
			Model:      "test-model",
			StopReason: model.StopEndTurn,
			Content: []model.ContentPart{
				model.TextPart{Text: "```bash\necho COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT\n```"},
			},
		},
	}

	prov := &pragmaLoopTestProvider{responses: responses}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          conv.WorkDir,
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	engine := NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(16), EngineConfig{
		Model:     "test-model",
		MaxTokens: 4096,
		MaxTurns:  5,
	})

	checkCalls := 0
	events := collectPragmaLoopEvents(engine.RunPragmaLoopWithSystemCompletionCheck(
		t.Context(),
		model.SystemPrompt{},
		"write the report",
		func() (bool, string, error) {
			checkCalls++
			if checkCalls == 1 {
				return false, "worker_report was not freshly written", nil
			}
			return true, "", nil
		},
	))

	var sawComplete bool
	for _, ev := range events {
		switch ev.(type) {
		case ErrorEvent:
			t.Fatalf("unexpected ErrorEvent: %#v", ev)
		case TurnCompleteEvent:
			sawComplete = true
		}
	}
	if !sawComplete {
		t.Fatal("missing TurnCompleteEvent")
	}
	if checkCalls != 2 {
		t.Fatalf("completion check calls = %d, want 2", checkCalls)
	}
	if prov.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", prov.calls)
	}
	if len(prov.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(prov.requests))
	}
	second := fmt.Sprint(prov.requests[1].Messages)
	if !strings.Contains(second, "worker_report was not freshly written") {
		t.Fatalf("second request missing rejection guidance: %s", second)
	}
}

func TestPragmaLoopFinalTextOnlyDoesNotExecuteBash(t *testing.T) {
	response := model.Response{
		ID:         "resp-final-text",
		Model:      "test-model",
		StopReason: model.StopEndTurn,
		Content: []model.ContentPart{
			model.TextPart{Text: "```bash\necho should-not-run\n```\nBLOCK"},
		},
	}

	prov := &pragmaLoopTestProvider{responses: []model.Response{response}}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          conv.WorkDir,
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	engine := NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(16), EngineConfig{
		Model:     "test-model",
		MaxTokens: 4096,
		MaxTurns:  5,
	})

	completionCheckCalled := false
	events := collectPragmaLoopEvents(engine.RunPragmaLoopWithSystemCompletionCheckOptions(
		t.Context(),
		model.SystemPrompt{},
		"return a verdict",
		func() (bool, string, error) {
			completionCheckCalled = true
			return false, "should not be called", nil
		},
		PragmaLoopRunOptions{FinalTextOnly: true},
	))

	var sawText, sawComplete bool
	for _, ev := range events {
		switch e := ev.(type) {
		case ErrorEvent:
			t.Fatalf("unexpected ErrorEvent: %v", e.Err)
		case TextEvent:
			if strings.Contains(e.Text, "BLOCK") {
				sawText = true
			}
		case TurnCompleteEvent:
			sawComplete = true
		case ToolCallEvent, ToolResultEvent:
			t.Fatalf("unexpected tool event: %#v", ev)
		}
	}
	if !sawText {
		t.Fatal("missing final text")
	}
	if !sawComplete {
		t.Fatal("missing TurnCompleteEvent")
	}
	if completionCheckCalled {
		t.Fatal("completion check should not run for final-text-only mode")
	}
	if prov.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", prov.calls)
	}
}

func TestPragmaLoopFinalTextOnlyRetriesInvalidFinalText(t *testing.T) {
	prov := &pragmaLoopTestProvider{responses: []model.Response{
		{
			ID:         "resp-empty",
			Model:      "test-model",
			StopReason: model.StopMaxTokens,
			Content:    []model.ContentPart{model.ThinkingPart{Text: "reasoning without final answer"}},
		},
		{
			ID:         "resp-block",
			Model:      "test-model",
			StopReason: model.StopEndTurn,
			Content:    []model.ContentPart{model.TextPart{Text: "BLOCK"}},
		},
	}}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          conv.WorkDir,
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	engine := NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(16), EngineConfig{
		Model:     "test-model",
		MaxTokens: 4096,
		MaxTurns:  5,
	})

	checkCalls := 0
	events := collectPragmaLoopEvents(engine.RunPragmaLoopWithSystemCompletionCheckOptions(
		t.Context(),
		model.SystemPrompt{},
		"return a verdict",
		nil,
		PragmaLoopRunOptions{
			FinalTextOnly: true,
			FinalTextCheck: func(text string) (bool, string, error) {
				checkCalls++
				if strings.TrimSpace(text) == "BLOCK" {
					return true, "", nil
				}
				return false, "Return exactly BLOCK.", nil
			},
		},
	))

	var sawText, sawComplete bool
	for _, ev := range events {
		switch e := ev.(type) {
		case ErrorEvent:
			t.Fatalf("unexpected ErrorEvent: %v", e.Err)
		case TextEvent:
			if e.Text == "BLOCK" {
				sawText = true
			}
			if strings.Contains(e.Text, "reasoning without final answer") {
				t.Fatalf("invalid hidden reasoning leaked as text event: %q", e.Text)
			}
		case TurnCompleteEvent:
			sawComplete = true
		}
	}
	if !sawText || !sawComplete {
		t.Fatalf("sawText=%v sawComplete=%v, want both true", sawText, sawComplete)
	}
	if checkCalls != 2 {
		t.Fatalf("final text check calls = %d, want 2", checkCalls)
	}
	if prov.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", prov.calls)
	}
	second := fmt.Sprint(prov.requests[1].Messages)
	if !strings.Contains(second, "Return exactly BLOCK.") {
		t.Fatalf("second request missing correction: %s", second)
	}
}

func TestPragmaLoopCommandPolicyRequiresPattern(t *testing.T) {
	ctx := WithPragmaLoopCommandPolicy(t.Context(), PragmaLoopCommandPolicyConfig{
		RequirePatterns: []string{`/tmp/pragma/survey\.md`},
		DenyMessage:     "write the survey artifact",
	})
	message, rejected := rejectPragmaLoopCommand(ctx, "pwd && ls")
	if !rejected {
		t.Fatal("expected command to be rejected")
	}
	if message != "write the survey artifact" {
		t.Fatalf("message = %q", message)
	}

	_, rejected = rejectPragmaLoopCommand(ctx, "pwd > /tmp/pragma/survey.md")
	if rejected {
		t.Fatal("expected command with required artifact path to pass")
	}
}

func TestPragmaLoopCommandPolicyDeniesForbiddenWorkerCommands(t *testing.T) {
	ctx := WithPragmaLoopCommandPolicy(t.Context(), PragmaLoopCommandPolicyConfig{
		DenyPatterns: []string{
			`(^|[;&|[:space:]])go[[:space:]]+(build|test|generate)\b`,
			`(^|[;&|[:space:]])git[[:space:]]+(checkout|restore|reset|clean)\b`,
			`(^|[;&|[:space:]])git[[:space:]]+show[[:space:]]+[^;&|[:space:]]+:`,
			`(^|[;&|[:space:]])git[[:space:]]+cat-file\b`,
			`(^|[;&|[:space:]])sed[[:space:]]+-i\b`,
			`(^|[;&|[:space:]])perl[[:space:]]+-pi\b`,
			`(^|[;&|[:space:]])patch([[:space:]]|$)`,
		},
		DenyMessage: "worker command denied",
	})

	for _, command := range []string{
		"cd /app && go build ./internal/config/ 2>&1 | head -30",
		"cd /app && git checkout internal/config/authentication.go",
		"git checkout -- internal/config/authentication.go rpc/flipt/auth/auth.proto",
		"cd /app && git show HEAD:internal/config/authentication.go > /tmp/original_auth.go && cp /tmp/original_auth.go internal/config/authentication.go",
		"cd /app && git cat-file blob HEAD:internal/config/authentication.go > internal/config/authentication.go",
		"sed -i 's/old/new/' /app/internal/config/authentication.go",
		"sed -i 's/METHOD_OIDC = 2;/METHOD_OIDC = 2;\\n  METHOD_KUBERNETES = 3;/' /app/rpc/flipt/auth/auth.proto && grep -A 6 \"enum Method\" /app/rpc/flipt/auth/auth.proto",
		"perl -pi -e 's/old/new/' /app/internal/config/authentication.go",
		"cd /app && perl -pi -e 's/old/new/' /app/internal/config/authentication.go && grep new /app/internal/config/authentication.go",
		"cd /app && patch -p1 < /tmp/k8s_auth_patch.txt",
	} {
		message, rejected := rejectPragmaLoopCommand(ctx, command)
		if !rejected {
			t.Fatalf("expected command to be rejected: %s", command)
		}
		if message != "worker command denied" {
			t.Fatalf("message = %q", message)
		}
	}
}

func TestCommandLooksLikeRepoMutation(t *testing.T) {
	reportPath := "/tmp/pragma/swe/worker-report.md"
	cases := []struct {
		name    string
		command string
		want    bool
	}{
		{
			name:    "temp move into app source",
			command: "awk '{ print }' /app/internal/config/authentication.go > /tmp/authentication.go.tmp && mv /tmp/authentication.go.tmp /app/internal/config/authentication.go",
			want:    true,
		},
		{
			name:    "python source write",
			command: "python3 - <<'PY'\nfrom pathlib import Path\nPath('/app/rpc/flipt/auth/auth.proto').write_text('x')\nPY",
			want:    true,
		},
		{
			name:    "coordination artifact report",
			command: "cat > /tmp/pragma/swe/worker-report.md <<'EOF'\n# Worker Report\nEOF",
			want:    false,
		},
		{
			name:    "source mutation plus report write",
			command: "python3 - <<'PY'\nfrom pathlib import Path\nPath('/app/internal/config/authentication.go').write_text('x')\nPath('/tmp/pragma/swe/worker-report.md').write_text('report')\nPY",
			want:    true,
		},
		{
			name:    "read only source",
			command: "sed -n '1,80p' /app/internal/config/authentication.go",
			want:    false,
		},
		{
			name:    "read only source with stderr redirection",
			command: "ls /app/internal/config/testdata/ && grep -l \"oidc\\|token\" /app/internal/config/testdata/*.yaml 2>/dev/null | head -5",
			want:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := commandLooksLikeRepoMutation(tc.command, []string{reportPath}); got != tc.want {
				t.Fatalf("commandLooksLikeRepoMutation() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPragmaLoopCustomSystemPromptPrependsRuntimePrompt(t *testing.T) {
	response := model.Response{
		ID:         "resp-text-only",
		Model:      "test-model",
		StopReason: model.StopEndTurn,
		Content: []model.ContentPart{
			model.TextPart{Text: "done"},
		},
	}

	prov := &pragmaLoopTestProvider{responses: []model.Response{response}}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          conv.WorkDir,
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	engine := NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(16), EngineConfig{
		Model:              "test-model",
		MaxTokens:          4096,
		MaxTurns:           5,
		CustomSystemPrompt: "Custom rules",
	})

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "hello"))
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			t.Fatalf("unexpected ErrorEvent: %v", e.Err)
		}
	}
	if len(prov.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(prov.requests))
	}
	blocks := prov.requests[0].System.Blocks
	if len(blocks) != 1 {
		t.Fatalf("system blocks = %d, want 1", len(blocks))
	}
	text := blocks[0].Text
	if !strings.HasPrefix(text, "Custom rules\n\nPragma loop mode is a shell-action transport.") {
		t.Fatalf("system prompt did not prepend custom prompt:\n%s", text)
	}
	if !strings.Contains(text, "```bash\nyour_command_here\n```") {
		t.Fatalf("system prompt lost shell-action format instructions:\n%s", text)
	}
}

func TestPragmaLoopDefaultSystemPromptUnchanged(t *testing.T) {
	engine := &Engine{}
	system := engine.pragmaLoopSystemPrompt()
	if len(system.Blocks) != 1 {
		t.Fatalf("system blocks = %d, want 1", len(system.Blocks))
	}
	if system.Blocks[0].Text != pragmaLoopSystemPrompt {
		t.Fatalf("default system prompt changed:\n%s", system.Blocks[0].Text)
	}
	if system.Blocks[0].Cacheable {
		t.Fatal("default system prompt cacheable = true, want false")
	}
}

func TestWithCustomSystemPromptPrependsExplicitSystemPrompt(t *testing.T) {
	engine := &Engine{config: EngineConfig{CustomSystemPrompt: "Custom orchestration rules"}}
	system := engine.WithCustomSystemPrompt(model.SystemPrompt{Blocks: []model.SystemBlock{
		{Text: "Persona rules", Cacheable: true},
	}})

	if len(system.Blocks) != 2 {
		t.Fatalf("system blocks = %d, want 2", len(system.Blocks))
	}
	if system.Blocks[0].Text != "Custom orchestration rules" {
		t.Fatalf("first block = %q, want custom rules", system.Blocks[0].Text)
	}
	if system.Blocks[0].Cacheable {
		t.Fatal("custom system prompt block cacheable = true, want false")
	}
	if system.Blocks[1].Text != "Persona rules" || !system.Blocks[1].Cacheable {
		t.Fatalf("persona block changed: %+v", system.Blocks[1])
	}
}

type pragmaLoopTestProvider struct {
	responses []model.Response
	calls     int
	requests  []provider.RequestParams
}

func (p *pragmaLoopTestProvider) Name() string { return "test" }

func (p *pragmaLoopTestProvider) Stream(context.Context, provider.RequestParams) (<-chan provider.StreamChunk, error) {
	return nil, fmt.Errorf("Stream is not used by this test provider")
}

func (p *pragmaLoopTestProvider) Complete(_ context.Context, params provider.RequestParams) (model.Response, error) {
	if p.calls >= len(p.responses) {
		return model.Response{}, fmt.Errorf("no response configured for call %d", p.calls+1)
	}
	p.requests = append(p.requests, params)
	response := p.responses[p.calls]
	p.calls++
	return response, nil
}

func (p *pragmaLoopTestProvider) SupportsFeature(provider.Feature) bool { return true }

func (p *pragmaLoopTestProvider) Pricing(string) (model.Pricing, bool) { return model.Pricing{}, false }

func (p *pragmaLoopTestProvider) ContextWindow(string) (int, bool) { return 200_000, true }

func TestProviderToolsLoopExecutesBashToolCall(t *testing.T) {
	callInput, err := json.Marshal(map[string]string{"cmd": "printf provider-tools-ok"})
	if err != nil {
		t.Fatal(err)
	}
	prov := &pragmaLoopTestProvider{responses: []model.Response{
		{
			Content: []model.ContentPart{
				model.ToolCallPart{ID: "call-1", Name: "Bash", Input: callInput},
			},
			StopReason: model.StopToolUse,
		},
		{
			Content:    []model.ContentPart{model.TextPart{Text: "done"}},
			StopReason: model.StopEndTurn,
		},
	}}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          conv.WorkDir,
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	costTracker := model.NewCostTracker(0)
	engine := NewEngine(prov, store, costTracker, observe.NewEventBus(16), EngineConfig{
		Model:     "test-model",
		LoopMode:  LoopModeProviderTools,
		MaxTokens: 4096,
		MaxTurns:  5,
	})

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "hello"))

	var sawToolCall, sawToolResult, sawFinal bool
	for _, ev := range events {
		switch e := ev.(type) {
		case ErrorEvent:
			t.Fatalf("unexpected ErrorEvent: %v", e.Err)
		case ToolCallEvent:
			if e.Call.Name == "Bash" {
				sawToolCall = true
			}
		case ToolResultEvent:
			if strings.Contains(e.Result.Content, "provider-tools-ok") {
				sawToolResult = true
			}
		case TurnCompleteEvent:
			sawFinal = true
		}
	}
	if !sawToolCall {
		t.Fatal("missing Bash ToolCallEvent")
	}
	if !sawToolResult {
		t.Fatal("missing Bash ToolResultEvent output")
	}
	if !sawFinal {
		t.Fatal("missing final TurnCompleteEvent")
	}
	if prov.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", prov.calls)
	}
	if len(prov.requests) != 2 || len(prov.requests[0].Tools) == 0 {
		t.Fatalf("provider tools were not sent: %#v", prov.requests)
	}
	if entries := costTracker.Snapshot(); len(entries) != 2 {
		t.Fatalf("cost entries = %d, want one per provider call", len(entries))
	}
}

func TestProviderToolsLoopContinuesReasoningOnlyMaxTokensResponse(t *testing.T) {
	callInput, err := json.Marshal(map[string]string{"cmd": "printf resumed-after-truncation"})
	if err != nil {
		t.Fatal(err)
	}
	prov := &pragmaLoopTestProvider{responses: []model.Response{
		{
			Content:    []model.ContentPart{model.ThinkingPart{Text: "partial reasoning"}},
			StopReason: model.StopMaxTokens,
		},
		{
			Content: []model.ContentPart{
				model.ToolCallPart{ID: "call-after-truncation", Name: "Bash", Input: callInput},
			},
			StopReason: model.StopToolUse,
		},
		{
			Content:    []model.ContentPart{model.TextPart{Text: "done"}},
			StopReason: model.StopEndTurn,
		},
	}}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          conv.WorkDir,
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	engine := NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(16), EngineConfig{
		Model:     "test-model",
		LoopMode:  LoopModeProviderTools,
		MaxTokens: 4096,
		MaxTurns:  5,
	})

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "hello"))

	var sawToolResult bool
	var completed int
	for _, ev := range events {
		switch e := ev.(type) {
		case ErrorEvent:
			t.Fatalf("unexpected ErrorEvent: %v", e.Err)
		case ToolResultEvent:
			if strings.Contains(e.Result.Content, "resumed-after-truncation") {
				sawToolResult = true
			}
		case TurnCompleteEvent:
			completed++
		}
	}
	if !sawToolResult {
		t.Fatal("missing tool result after max-token continuation")
	}
	if completed != 1 {
		t.Fatalf("TurnCompleteEvent count = %d, want only the final response", completed)
	}
	if prov.calls != 3 {
		t.Fatalf("provider calls = %d, want 3", prov.calls)
	}
	if len(prov.requests) < 2 {
		t.Fatalf("provider requests = %d, want at least 2", len(prov.requests))
	}
	messages := prov.requests[1].Messages
	if len(messages) != 3 {
		t.Fatalf("second request messages = %d, want original user, partial assistant, continuation user", len(messages))
	}
	if messages[1].Role != model.RoleAssistant {
		t.Fatalf("partial response role = %s, want assistant", messages[1].Role)
	}
	thinking, ok := messages[1].Content[0].(model.ThinkingPart)
	if !ok || thinking.Text != "partial reasoning" {
		t.Fatalf("partial response = %#v, want preserved reasoning", messages[1].Content)
	}
	if messages[2].Role != model.RoleUser {
		t.Fatalf("continuation role = %s, want user", messages[2].Role)
	}
	continuation, ok := messages[2].Content[0].(model.TextPart)
	if !ok || continuation.Text != providerToolsMaxTokensContinuation {
		t.Fatalf("continuation = %#v, want %q", messages[2].Content, providerToolsMaxTokensContinuation)
	}
}

func TestProviderToolsLoopHonorsCustomSystemPrompt(t *testing.T) {
	prov := &pragmaLoopTestProvider{responses: []model.Response{{
		Content:    []model.ContentPart{model.TextPart{Text: "done"}},
		StopReason: model.StopEndTurn,
	}}}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          conv.WorkDir,
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	engine := NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(16), EngineConfig{
		Model:              "test-model",
		LoopMode:           LoopModeProviderTools,
		MaxTokens:          4096,
		MaxTurns:           5,
		CustomSystemPrompt: "Custom provider-tools rules",
	})

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "hello"))
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			t.Fatalf("unexpected ErrorEvent: %v", e.Err)
		}
	}
	if len(prov.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(prov.requests))
	}
	blocks := prov.requests[0].System.Blocks
	if len(blocks) == 0 || blocks[0].Text != "Custom provider-tools rules" {
		t.Fatalf("system blocks = %#v, want custom prompt first", blocks)
	}
}

func collectPragmaLoopEvents(ch <-chan LoopEvent) []LoopEvent {
	var events []LoopEvent
	for ev := range ch {
		events = append(events, ev)
	}
	return events
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
			name: "prose before bash block rejected",
			text: strings.Join([]string{
				"I will run this now.",
				"```bash",
				"echo ok",
				"```",
			}, "\n"),
			wantCount: 2,
		},
		{
			name: "prose after bash block rejected",
			text: strings.Join([]string{
				"```bash",
				"echo ok",
				"```",
				"Done.",
			}, "\n"),
			wantCount: 2,
		},
		{
			name: "explicit think block before bash block ignored",
			text: strings.Join([]string{
				"<think>I need to inspect the file.</think>",
				"",
				"```bash",
				"echo ok",
				"```",
			}, "\n"),
			wantCount: 1,
			want:      "echo ok",
		},
		{
			name: "explicit think block after bash block ignored",
			text: strings.Join([]string{
				"```bash",
				"echo ok",
				"```",
				"",
				"<think>done</think>",
			}, "\n"),
			wantCount: 1,
			want:      "echo ok",
		},
		{
			name: "fused second bash fence inside command rejected",
			text: strings.Join([]string{
				"```bash",
				"echo one",
				"``````bash",
				"echo two",
				"```",
			}, "\n"),
			wantCount: 2,
		},
		{
			name: "plain markdown fence inside command rejected",
			text: strings.Join([]string{
				"```bash",
				"echo one",
				"```json",
				"{}",
				"```",
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

func TestPragmaLoopReplayContentStripsExplicitThinkBlocks(t *testing.T) {
	content := []model.ContentPart{
		model.TextPart{Text: strings.Join([]string{
			"<think>hidden scratchpad</think>",
			"",
			"```bash",
			"echo ok",
			"```",
		}, "\n")},
	}

	replay := pragmaLoopReplayContent(content)
	if len(replay) != 1 {
		t.Fatalf("len(replay) = %d, want 1", len(replay))
	}
	text, ok := replay[0].(model.TextPart)
	if !ok {
		t.Fatalf("replay[0] = %T, want model.TextPart", replay[0])
	}
	if strings.Contains(text.Text, "<think>") || strings.Contains(text.Text, "</think>") {
		t.Fatalf("replay text still contains explicit think block: %q", text.Text)
	}
	if strings.TrimSpace(text.Text) != "```bash\necho ok\n```" {
		t.Fatalf("replay text = %q", text.Text)
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
