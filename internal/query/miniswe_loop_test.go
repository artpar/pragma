package query

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/tool"
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

func TestPragmaLoopInstancePromptMatchesPragmaLoopYamlSedBoundary(t *testing.T) {
	prompt := pragmaLoopInstancePrompt("do the task", "/work")
	if !strings.Contains(prompt, "Current working directory: /work") {
		t.Fatalf("instance prompt does not include current working directory: %q", prompt)
	}
	if strings.Contains(prompt, "MY_ENV_VAR=MY_VALUE cd /path/to/working/dir") {
		t.Fatalf("instance prompt contains invalid env/cd example: %q", prompt)
	}
	if strings.Contains(prompt, "THOUGHT") || strings.Contains(prompt, "Here are some thoughts") {
		t.Fatalf("instance prompt still contains reasoning prose examples: %q", prompt)
	}
	if !strings.Contains(prompt, "<system_information>") {
		t.Fatalf("instance prompt does not include system information: %q", prompt)
	}

	msg := fmt.Sprintf(pragmaLoopFormatErrorTemplate, 2)
	if strings.Contains(msg, "THOUGHT") || strings.Contains(msg, "thoughts") {
		t.Fatalf("format error still asks for reasoning prose: %q", msg)
	}
	if !strings.Contains(msg, "final answer with no fenced bash block") {
		t.Fatalf("format error does not explain no-bash final answers: %q", msg)
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

func TestPragmaLoopStopsOnNoBashFinalAnswer(t *testing.T) {
	prov := &pragmaLoopTestProvider{
		responses: []model.Response{{
			Content:    []model.ContentPart{model.TextPart{Text: "Hello. How can I help?"}},
			StopReason: model.StopEndTurn,
		}},
	}
	engine := newPragmaLoopTestEngine(t, prov)

	events := drainPragmaLoopEvents(engine.Run(t.Context(), "hi"))

	var gotText string
	var gotComplete bool
	for _, ev := range events {
		switch e := ev.(type) {
		case TextEvent:
			gotText += e.Text
		case TurnCompleteEvent:
			gotComplete = true
			if responseText(e.Response) != "Hello. How can I help?" {
				t.Fatalf("turn response = %q", responseText(e.Response))
			}
		case ErrorEvent:
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}
	if gotText != "Hello. How can I help?" {
		t.Fatalf("text = %q", gotText)
	}
	if !gotComplete {
		t.Fatal("expected turn complete")
	}
	if prov.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", prov.calls)
	}
}

func TestPragmaLoopExecutesOneBashBlockThenContinues(t *testing.T) {
	prov := &pragmaLoopTestProvider{
		responses: []model.Response{
			{
				Content:    []model.ContentPart{model.TextPart{Text: "```bash\necho observed\n```"}},
				StopReason: model.StopEndTurn,
			},
			{
				Content:    []model.ContentPart{model.TextPart{Text: "I saw observed."}},
				StopReason: model.StopEndTurn,
			},
		},
	}
	engine := newPragmaLoopTestEngine(t, prov)

	events := drainPragmaLoopEvents(engine.Run(t.Context(), "inspect"))

	var gotComplete bool
	for _, ev := range events {
		switch e := ev.(type) {
		case TurnCompleteEvent:
			gotComplete = true
			if responseText(e.Response) != "I saw observed." {
				t.Fatalf("turn response = %q", responseText(e.Response))
			}
		case ErrorEvent:
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}
	if !gotComplete {
		t.Fatal("expected turn complete")
	}
	if prov.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", prov.calls)
	}

	messages := engine.store.Snapshot().Conversation.Messages
	var sawObservation bool
	for _, msg := range messages {
		if msg.Role != model.RoleUser {
			continue
		}
		for _, part := range msg.Content {
			text, ok := part.(model.TextPart)
			if ok && strings.Contains(text.Text, "<returncode>0</returncode>") && strings.Contains(text.Text, "observed") {
				sawObservation = true
			}
		}
	}
	if !sawObservation {
		t.Fatal("expected command observation in conversation")
	}
}

func TestPragmaLoopRepairsMultipleBashBlocks(t *testing.T) {
	prov := &pragmaLoopTestProvider{
		responses: []model.Response{
			{
				Content:    []model.ContentPart{model.TextPart{Text: "```bash\necho one\n```\n```bash\necho two\n```"}},
				StopReason: model.StopEndTurn,
			},
			{
				Content:    []model.ContentPart{model.TextPart{Text: "done"}},
				StopReason: model.StopEndTurn,
			},
		},
	}
	engine := newPragmaLoopTestEngine(t, prov)

	events := drainPragmaLoopEvents(engine.Run(t.Context(), "inspect"))
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}
	if prov.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", prov.calls)
	}

	messages := engine.store.Snapshot().Conversation.Messages
	var sawRepair bool
	for _, msg := range messages {
		if msg.Role != model.RoleUser {
			continue
		}
		for _, part := range msg.Content {
			text, ok := part.(model.TextPart)
			if ok && strings.Contains(text.Text, "at most one bash action") && strings.Contains(text.Text, "Found 2 actions") {
				sawRepair = true
			}
		}
	}
	if !sawRepair {
		t.Fatal("expected multiple-action repair prompt")
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

type pragmaLoopTestProvider struct {
	responses []model.Response
	calls     int
}

func (p *pragmaLoopTestProvider) Name() string { return "pragma-loop-test" }

func (p *pragmaLoopTestProvider) Complete(_ context.Context, _ provider.RequestParams) (model.Response, error) {
	if p.calls >= len(p.responses) {
		return model.Response{}, fmt.Errorf("no response configured for call %d", p.calls+1)
	}
	resp := p.responses[p.calls]
	p.calls++
	return resp, nil
}

func (p *pragmaLoopTestProvider) Stream(_ context.Context, params provider.RequestParams) (<-chan provider.StreamChunk, error) {
	resp, err := p.Complete(context.Background(), params)
	if err != nil {
		return nil, err
	}
	ch := make(chan provider.StreamChunk, len(resp.Content)+1)
	for _, part := range resp.Content {
		if text, ok := part.(model.TextPart); ok {
			ch <- provider.StreamChunk{TextDelta: text.Text}
		}
	}
	ch <- provider.StreamChunk{Done: &provider.StreamDone{StopReason: resp.StopReason}}
	close(ch)
	return ch, nil
}

func (p *pragmaLoopTestProvider) SupportsFeature(_ provider.Feature) bool { return true }
func (p *pragmaLoopTestProvider) Pricing(_ string) (model.Pricing, bool) {
	return model.Pricing{}, false
}
func (p *pragmaLoopTestProvider) ContextWindow(_ string) (int, bool) { return 200_000, true }

type pragmaLoopAllowAllChecker struct{}

func (pragmaLoopAllowAllChecker) Check(_ context.Context, _ string, _ string) permission.CheckResult {
	return permission.CheckResult{Decision: permission.DecisionAllow}
}

func (pragmaLoopAllowAllChecker) AddSessionRule(_ permission.Rule) {}

func (pragmaLoopAllowAllChecker) AddPersistentRule(_ permission.Rule) error { return nil }

func newPragmaLoopTestEngine(t *testing.T, prov provider.Provider) *Engine {
	t.Helper()
	bus := observe.NewEventBus(256)
	registry := tool.NewRegistry(bus)
	orch := tool.NewOrchestrator(registry, pragmaLoopAllowAllChecker{}, &permission.NonInteractivePrompter{}, bus)
	workDir := t.TempDir()
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", workDir)
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          workDir,
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	return NewEngine(prov, registry, orch, store, model.NewCostTracker(0), bus, EngineConfig{
		Model:     "test-model",
		MaxTokens: 4096,
		MaxTurns:  10,
	})
}

func drainPragmaLoopEvents(ch <-chan LoopEvent) []LoopEvent {
	var events []LoopEvent
	for ev := range ch {
		events = append(events, ev)
	}
	return events
}
