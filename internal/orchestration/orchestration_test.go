package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/llmconfig"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/persona"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/query"

	"gopkg.in/yaml.v3"
)

func TestNewRuntimeUsesLooplabFSM(t *testing.T) {
	runtime, err := NewRuntime(Definition{
		Name:    "planner-executor",
		Initial: "planner",
		States: []State{
			{ID: "planner"},
			{ID: "implementer"},
			{ID: "done", Terminal: true},
		},
		Transitions: []Transition{
			{Event: EventComplete, From: []string{"planner"}, To: "implementer"},
			{Event: EventComplete, From: []string{"implementer"}, To: "done"},
		},
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	if got := runtime.FSM.Current(); got != "planner" {
		t.Fatalf("initial state = %q, want planner", got)
	}
	if !runtime.FSM.Can(EventComplete) {
		t.Fatalf("expected looplab FSM to allow %q", EventComplete)
	}
	if err := runtime.FSM.Event(context.Background(), EventComplete); err != nil {
		t.Fatalf("first transition: %v", err)
	}
	if got := runtime.FSM.Current(); got != "implementer" {
		t.Fatalf("state after first transition = %q, want implementer", got)
	}
	if err := runtime.FSM.Event(context.Background(), EventComplete); err != nil {
		t.Fatalf("second transition: %v", err)
	}
	if got := runtime.FSM.Current(); got != "done" {
		t.Fatalf("state after second transition = %q, want done", got)
	}
}

func TestNewRuntimeRejectsTerminalTransitionSource(t *testing.T) {
	_, err := NewRuntime(Definition{
		Name:    "bad",
		Initial: "done",
		States: []State{
			{ID: "done", Terminal: true},
			{ID: "solver"},
		},
		Transitions: []Transition{
			{Event: EventComplete, From: []string{"done"}, To: "solver"},
		},
	})
	if err == nil {
		t.Fatal("expected terminal transition source to be rejected")
	}
}

func TestLoadArchitectImplementerProsecutorYAML(t *testing.T) {
	def, err := LoadDefinitionFile(filepath.Join("..", "..", "orchestrations", "architect-implementer-prosecutor.yaml"))
	if err != nil {
		t.Fatalf("LoadDefinitionFile: %v", err)
	}
	runtime, err := NewRuntime(def)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	for _, event := range []string{
		EventComplete,
		EventComplete,
		EventComplete,
		"block",
		EventComplete,
		EventComplete,
		"approve",
	} {
		if err := runtime.FSM.Event(context.Background(), event); err != nil {
			t.Fatalf("event %q from %q: %v", event, runtime.FSM.Current(), err)
		}
	}
	if got := runtime.FSM.Current(); got != "done" {
		t.Fatalf("final state = %q, want done", got)
	}
}

func TestLoadStateWithLLMConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "llm.yaml")
	if err := os.WriteFile(path, []byte(`
name: llm-test
initial: worker
states:
  - id: worker
    persona: coder
    llm:
      provider: anthropic
      model: sonnet
      max_tokens: 2048
      temperature: 0
      thinking: true
      thinking_budget: 512
      system_prompt: Use strict output.
  - id: done
    terminal: true
transitions:
  - event: complete
    from: [worker]
    to: done
`), 0o644); err != nil {
		t.Fatalf("write orchestration: %v", err)
	}

	def, err := LoadDefinitionFile(path)
	if err != nil {
		t.Fatalf("LoadDefinitionFile: %v", err)
	}
	if len(def.States) == 0 {
		t.Fatal("expected states")
	}
	state := def.States[0]
	if state.LLM.Provider != "anthropic" {
		t.Fatalf("provider = %q, want anthropic", state.LLM.Provider)
	}
	if state.LLM.Model != "sonnet" {
		t.Fatalf("model = %q, want sonnet", state.LLM.Model)
	}
	if state.LLM.MaxTokens != 2048 {
		t.Fatalf("max tokens = %d, want 2048", state.LLM.MaxTokens)
	}
	if state.LLM.Temperature == nil || *state.LLM.Temperature != 0 {
		t.Fatalf("temperature = %v, want 0", state.LLM.Temperature)
	}
	if state.LLM.Thinking == nil || !*state.LLM.Thinking {
		t.Fatalf("thinking = %v, want true", state.LLM.Thinking)
	}
	if state.LLM.ThinkingBudget != 512 {
		t.Fatalf("thinking budget = %d, want 512", state.LLM.ThinkingBudget)
	}
	if state.LLM.SystemPrompt != "Use strict output." {
		t.Fatalf("system prompt = %q", state.LLM.SystemPrompt)
	}
}

func TestLoadStateWithFinalTextRuntimeCapture(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "final-text.yaml")
	if err := os.WriteFile(path, []byte(`
name: final-text-test
initial: gate
states:
  - id: gate
    persona: gate
    artifacts:
      outputs:
        - id: verdict
          path: verdict.txt
          required: true
          runtime_capture:
            type: final_text
  - id: done
    terminal: true
transitions:
  - event: complete
    from: [gate]
    to: done
`), 0o644); err != nil {
		t.Fatalf("write orchestration: %v", err)
	}

	def, err := LoadDefinitionFile(path)
	if err != nil {
		t.Fatalf("LoadDefinitionFile: %v", err)
	}
	if got := def.States[0].Artifacts.Outputs[0].RuntimeCapture.Type; got != "final_text" {
		t.Fatalf("runtime capture = %q, want final_text", got)
	}
}

func TestLoadStateWithShellPolicyRequirePatterns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "require-pattern.yaml")
	if err := os.WriteFile(path, []byte(`
name: require-pattern-test
initial: survey
states:
  - id: survey
    persona: survey
    shell_policy:
      require_patterns:
        - /tmp/pragma/survey\.md
      deny_message: write the survey artifact
  - id: done
    terminal: true
transitions:
  - event: complete
    from: [survey]
    to: done
`), 0o644); err != nil {
		t.Fatalf("write orchestration: %v", err)
	}

	def, err := LoadDefinitionFile(path)
	if err != nil {
		t.Fatalf("LoadDefinitionFile: %v", err)
	}
	if got := def.States[0].ShellPolicy.RequirePatterns; len(got) != 1 || got[0] != `/tmp/pragma/survey\.md` {
		t.Fatalf("require patterns = %#v", got)
	}
}

func TestLoadArtifactMaxBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "max-bytes.yaml")
	if err := os.WriteFile(path, []byte(`
name: max-bytes-test
initial: audit
states:
  - id: audit
    persona: audit
    artifacts:
      inputs:
        - id: large_context
          path: context.md
          max_bytes: 128
  - id: done
    terminal: true
transitions:
  - event: complete
    from: [audit]
    to: done
`), 0o644); err != nil {
		t.Fatalf("write orchestration: %v", err)
	}

	def, err := LoadDefinitionFile(path)
	if err != nil {
		t.Fatalf("LoadDefinitionFile: %v", err)
	}
	if got := def.States[0].Artifacts.Inputs[0].MaxBytes; got != 128 {
		t.Fatalf("max bytes = %d, want 128", got)
	}
}

func TestCaptureFinalTextArtifacts(t *testing.T) {
	root := t.TempDir()
	state := State{
		ID: "gate",
		Artifacts: Artifacts{Outputs: []Artifact{{
			ID:             "verdict",
			Path:           "verdict.txt",
			AllowedValues:  []string{"PASS", "BLOCK"},
			RuntimeCapture: ArtifactRuntimeCapture{Type: "final_text"},
		}}},
	}

	if err := captureFinalTextArtifacts(state, root, "<think>hidden reasoning</think>\n\nBLOCK\n\n"); err != nil {
		t.Fatalf("captureFinalTextArtifacts: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "verdict.txt"))
	if err != nil {
		t.Fatalf("read verdict: %v", err)
	}
	if got := string(data); got != "BLOCK\n" {
		t.Fatalf("captured text = %q, want BLOCK newline", got)
	}
}

func TestFinalTextStatePromptDoesNotUseShellContract(t *testing.T) {
	state := State{
		ID:      "gate",
		Persona: "gate",
		Artifacts: Artifacts{Outputs: []Artifact{{
			ID:             "verdict",
			Path:           "verdict.txt",
			Required:       true,
			RuntimeCapture: ArtifactRuntimeCapture{Type: "final_text"},
		}}},
	}
	system, prompt, err := BuildPromptWithArtifactRootChecked(
		Definition{Name: "test"},
		state,
		persona.Definition{ID: "gate", Prompt: "Return PASS or BLOCK."},
		"",
		"handoff",
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf("BuildPromptWithArtifactRootChecked: %v", err)
	}
	text := system.Blocks[0].Text
	if strings.Contains(text, "shell-action transport") || strings.Contains(text, "fenced bash block") {
		t.Fatalf("final_text prompt contains shell contract: %s", text)
	}
	if strings.Contains(prompt, "Response Output Gate") || strings.Contains(prompt, "```bash") {
		t.Fatalf("final_text user prompt contains shell response gate: %s", prompt)
	}
	if !strings.Contains(text, "captures the assistant final text") {
		t.Fatalf("final_text prompt missing final text contract: %s", text)
	}
}

func TestRunEventsStopAfterStateStopsBeforeTransition(t *testing.T) {
	root := t.TempDir()
	verdictPath := filepath.Join(root, "verdict.txt")
	if err := os.WriteFile(verdictPath, []byte("PASS\n"), 0o600); err != nil {
		t.Fatalf("write verdict: %v", err)
	}
	def := Definition{
		Name:    "stop-after-test",
		Initial: "gate",
		States: []State{
			{
				ID: "gate",
				Control: Control{ArtifactVerdict: &ArtifactVerdictControl{
					Path:         verdictPath,
					Approve:      "PASS",
					Block:        "BLOCK",
					ApproveEvent: "pass",
					BlockEvent:   "block",
				}},
			},
			{ID: "worker", Persona: "worker"},
			{ID: "done", Terminal: true},
		},
		Transitions: []Transition{
			{Event: "pass", From: []string{"gate"}, To: "worker"},
			{Event: "block", From: []string{"gate"}, To: "done"},
			{Event: EventComplete, From: []string{"worker"}, To: "done"},
		},
	}
	store := app.NewStateStore(app.AppState{
		Conversation: model.NewConversation(model.SystemPrompt{}, "base-model", "base", "."),
		CWD:          ".",
		Model:        "base-model",
		Provider:     "base",
		MaxTokens:    1000,
	})
	engine := query.NewEngine(&orchestrationTestProvider{name: "base"}, store, model.NewCostTracker(0), nil, query.EngineConfig{
		Model:     "base-model",
		MaxTokens: 1000,
	})

	var started []string
	var transitions []query.OrchestrationTransitionEvent
	for ev := range RunEventsWithOptions(t.Context(), engine, def, RunOptions{
		ArtifactRoot:   root,
		StopAfterState: "gate",
	}) {
		switch e := ev.(type) {
		case query.ErrorEvent:
			t.Fatalf("unexpected error: %v", e.Err)
		case query.OrchestrationStateStartedEvent:
			started = append(started, e.StateID)
		case query.OrchestrationTransitionEvent:
			transitions = append(transitions, e)
		}
	}

	if strings.Join(started, ",") != "gate" {
		t.Fatalf("started states = %v, want only gate", started)
	}
	if len(transitions) != 0 {
		t.Fatalf("transitions = %#v, want none", transitions)
	}
}

func TestRunEventsStartAtState(t *testing.T) {
	root := t.TempDir()
	verdictPath := filepath.Join(root, "verdict.txt")
	if err := os.WriteFile(verdictPath, []byte("BLOCK\n"), 0o600); err != nil {
		t.Fatalf("write verdict: %v", err)
	}
	def := Definition{
		Name:    "start-at-test",
		Initial: "survey",
		States: []State{
			{ID: "survey", Persona: "survey"},
			{
				ID: "gate",
				Control: Control{ArtifactVerdict: &ArtifactVerdictControl{
					Path:         verdictPath,
					Approve:      "PASS",
					Block:        "BLOCK",
					ApproveEvent: "pass",
					BlockEvent:   "block",
				}},
			},
			{ID: "done", Terminal: true},
		},
		Transitions: []Transition{
			{Event: EventComplete, From: []string{"survey"}, To: "gate"},
			{Event: "pass", From: []string{"gate"}, To: "done"},
			{Event: "block", From: []string{"gate"}, To: "done"},
		},
	}
	store := app.NewStateStore(app.AppState{
		Conversation: model.NewConversation(model.SystemPrompt{}, "base-model", "base", "."),
		CWD:          ".",
		Model:        "base-model",
		Provider:     "base",
		MaxTokens:    1000,
	})
	engine := query.NewEngine(&orchestrationTestProvider{name: "base"}, store, model.NewCostTracker(0), nil, query.EngineConfig{
		Model:     "base-model",
		MaxTokens: 1000,
	})

	var started []string
	var transitions []query.OrchestrationTransitionEvent
	for ev := range RunEventsWithOptions(t.Context(), engine, def, RunOptions{
		ArtifactRoot: root,
		StartAtState: "gate",
	}) {
		switch e := ev.(type) {
		case query.ErrorEvent:
			t.Fatalf("unexpected error: %v", e.Err)
		case query.OrchestrationStateStartedEvent:
			started = append(started, e.StateID)
		case query.OrchestrationTransitionEvent:
			transitions = append(transitions, e)
		}
	}

	if strings.Join(started, ",") != "gate" {
		t.Fatalf("started states = %v, want only gate", started)
	}
	if len(transitions) != 1 || transitions[0].From != "gate" || transitions[0].Event != "block" || transitions[0].To != "done" {
		t.Fatalf("transitions = %#v, want gate --block--> done", transitions)
	}
}

func TestRunEventsStartAtStateRejectsUnknownState(t *testing.T) {
	def := Definition{
		Name:    "start-at-test",
		Initial: "survey",
		States: []State{
			{ID: "survey", Persona: "survey"},
			{ID: "done", Terminal: true},
		},
		Transitions: []Transition{
			{Event: EventComplete, From: []string{"survey"}, To: "done"},
		},
	}
	store := app.NewStateStore(app.AppState{
		Conversation: model.NewConversation(model.SystemPrompt{}, "base-model", "base", "."),
		CWD:          ".",
		Model:        "base-model",
		Provider:     "base",
		MaxTokens:    1000,
	})
	engine := query.NewEngine(&orchestrationTestProvider{name: "base"}, store, model.NewCostTracker(0), nil, query.EngineConfig{
		Model:     "base-model",
		MaxTokens: 1000,
	})

	var err error
	for ev := range RunEventsWithOptions(t.Context(), engine, def, RunOptions{
		ArtifactRoot: t.TempDir(),
		StartAtState: "missing",
	}) {
		if e, ok := ev.(query.ErrorEvent); ok {
			err = e.Err
		}
	}
	if err == nil {
		t.Fatal("expected missing start state error")
	}
	if !strings.Contains(err.Error(), `start state "missing" not found`) {
		t.Fatalf("error = %v, want missing start state", err)
	}
}

func TestRunEventsStopAfterStateRejectsUnknownState(t *testing.T) {
	def := Definition{
		Name:    "stop-after-test",
		Initial: "survey",
		States: []State{
			{ID: "survey", Persona: "survey"},
			{ID: "done", Terminal: true},
		},
		Transitions: []Transition{
			{Event: EventComplete, From: []string{"survey"}, To: "done"},
		},
	}
	store := app.NewStateStore(app.AppState{
		Conversation: model.NewConversation(model.SystemPrompt{}, "base-model", "base", "."),
		CWD:          ".",
		Model:        "base-model",
		Provider:     "base",
		MaxTokens:    1000,
	})
	engine := query.NewEngine(&orchestrationTestProvider{name: "base"}, store, model.NewCostTracker(0), nil, query.EngineConfig{
		Model:     "base-model",
		MaxTokens: 1000,
	})

	var err error
	for ev := range RunEventsWithOptions(t.Context(), engine, def, RunOptions{
		ArtifactRoot:   t.TempDir(),
		StopAfterState: "missing",
	}) {
		if e, ok := ev.(query.ErrorEvent); ok {
			err = e.Err
		}
	}
	if err == nil {
		t.Fatal("expected missing stop-after state error")
	}
	if !strings.Contains(err.Error(), `stop-after state "missing" not found`) {
		t.Fatalf("error = %v, want missing stop-after state", err)
	}
}

func TestRunEventsStartAtStatePreservesIncomingHandoff(t *testing.T) {
	root := t.TempDir()
	personaDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(personaDir, "gate.yaml"), []byte("id: gate\nprompt: Inspect handoff evidence and finish.\n"), 0o600); err != nil {
		t.Fatalf("write persona: %v", err)
	}
	def := Definition{
		Name:    "start-at-handoff-test",
		Initial: "survey",
		States: []State{
			{ID: "survey", Persona: "survey"},
			{ID: "gate", Persona: "gate"},
			{ID: "done", Terminal: true},
		},
		Transitions: []Transition{
			{Event: EventComplete, From: []string{"survey"}, To: "gate", Handoff: []Artifact{{
				ID:       "survey_report",
				Path:     "survey/report.md",
				Required: true,
			}}},
			{Event: EventComplete, From: []string{"gate"}, To: "done"},
		},
	}
	prov := &orchestrationTestProvider{
		name: "base",
		responses: []model.Response{{
			Model:      "base-model",
			Content:    []model.ContentPart{model.TextPart{Text: "done"}},
			StopReason: model.StopEndTurn,
		}},
	}
	store := app.NewStateStore(app.AppState{
		Conversation: model.NewConversation(model.SystemPrompt{}, "base-model", "base", "."),
		CWD:          ".",
		Model:        "base-model",
		Provider:     "base",
		MaxTokens:    1000,
	})
	engine := query.NewEngine(prov, store, model.NewCostTracker(0), nil, query.EngineConfig{
		Model:     "base-model",
		MaxTokens: 1000,
	})

	for ev := range RunEventsWithOptions(t.Context(), engine, def, RunOptions{
		ArtifactRoot: root,
		PersonaDir:   personaDir,
		StartAtState: "gate",
		SeedArtifacts: map[string]string{
			"survey_report": "seeded survey evidence\n",
		},
	}) {
		if e, ok := ev.(query.ErrorEvent); ok {
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}
	if len(prov.requests) == 0 {
		t.Fatal("provider was not called")
	}
	var requestText strings.Builder
	for _, msg := range prov.requests[0].Messages {
		for _, part := range msg.Content {
			if text, ok := part.(model.TextPart); ok {
				requestText.WriteString(text.Text)
			}
		}
	}
	if !strings.Contains(requestText.String(), "seeded survey evidence") {
		t.Fatalf("first request missing seeded transition handoff:\n%s", requestText.String())
	}
}

func TestMaterializeSeedArtifactsByArtifactID(t *testing.T) {
	root := t.TempDir()
	def := Definition{
		Name:    "seed-test",
		Initial: "worker",
		States: []State{
			{
				ID: "worker",
				Artifacts: Artifacts{Outputs: []Artifact{{
					ID:   "worker_report",
					Path: "swe/worker-report.md",
				}}},
			},
			{ID: "done", Terminal: true},
		},
		Transitions: []Transition{
			{
				Event: EventComplete,
				From:  []string{"worker"},
				To:    "done",
				Handoff: []Artifact{{
					ID:   "worker_report",
					Path: "swe/worker-report.md",
				}},
			},
		},
	}

	writes, err := materializeSeedArtifacts(def, root, RunOptions{
		SeedArtifacts: map[string]string{
			"worker_report": "seeded report\n",
		},
	})
	if err != nil {
		t.Fatalf("materializeSeedArtifacts: %v", err)
	}
	if len(writes) != 1 {
		t.Fatalf("writes = %#v, want one write", writes)
	}
	path := filepath.Join(root, "swe", "worker-report.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read seeded artifact: %v", err)
	}
	if string(data) != "seeded report\n" {
		t.Fatalf("seeded artifact = %q", data)
	}
}

func TestMaterializeSeedArtifactsRejectsUnknownTarget(t *testing.T) {
	_, err := materializeSeedArtifacts(Definition{
		Name:    "seed-test",
		Initial: "worker",
		States:  []State{{ID: "worker"}},
	}, t.TempDir(), RunOptions{
		SeedArtifacts: map[string]string{
			"missing_artifact": "content",
		},
	})
	if err == nil {
		t.Fatal("expected unknown seed target error")
	}
	if !strings.Contains(err.Error(), `seed artifact target "missing_artifact" does not match`) {
		t.Fatalf("error = %v, want unknown seed target", err)
	}
}

func TestRequiredOutputCompletionCheckRejectsStaleSeededOutput(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "swe", "engineering-context.md")
	writeText(t, root, "swe/engineering-context.md", "old context\n")
	before, err := snapshotArtifactFile(path)
	if err != nil {
		t.Fatalf("snapshotArtifactFile: %v", err)
	}
	state := State{
		ID: "theory",
		Artifacts: Artifacts{Outputs: []Artifact{{
			ID:       "engineering_context",
			Path:     "swe/engineering-context.md",
			Required: true,
		}}},
	}

	check := requiredOutputArtifactCompletionCheckWithSnapshots(
		state,
		root,
		"",
		nil,
		map[string]ArtifactFileSnapshot{path: before},
	)
	ok, guidance, err := check()
	if err != nil {
		t.Fatalf("completion check: %v", err)
	}
	if ok {
		t.Fatal("expected stale seeded output to be rejected")
	}
	if !strings.Contains(guidance, "not freshly written") {
		t.Fatalf("guidance = %q, want stale output guidance", guidance)
	}
}

func TestRenderTransitionHandoffMaxBytesPreservesSnapshot(t *testing.T) {
	root := t.TempDir()
	writeText(t, root, "large.md", "0123456789abcdef\n")
	handoff, reads, err := RenderTransitionHandoff(State{ID: "audit"}, []Artifact{{
		ID:          "large_context",
		Path:        "large.md",
		Description: "Large context.",
		MaxBytes:    8,
	}}, root, true)
	if err != nil {
		t.Fatalf("RenderTransitionHandoff: %v", err)
	}
	if !strings.Contains(handoff, "truncated to first 8 of 17 bytes") {
		t.Fatalf("handoff missing truncation marker:\n%s", handoff)
	}
	if !strings.Contains(handoff, "01234567") {
		t.Fatalf("handoff missing rendered prefix:\n%s", handoff)
	}
	if strings.Contains(handoff, "89abcdef") {
		t.Fatalf("handoff included content beyond max_bytes:\n%s", handoff)
	}
	if len(reads) != 1 || string(reads[0].Content) != "0123456789abcdef\n" {
		t.Fatalf("snapshot reads = %#v, want full content", reads)
	}
}

func TestValidateFreshRequiredModelOutputsRejectsStaleSeededOutput(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "swe", "worker-report.md")
	writeText(t, root, "swe/worker-report.md", "old report\n")
	before, err := snapshotArtifactFile(path)
	if err != nil {
		t.Fatalf("snapshotArtifactFile before: %v", err)
	}
	after, err := snapshotArtifactFile(path)
	if err != nil {
		t.Fatalf("snapshotArtifactFile after: %v", err)
	}
	state := State{
		ID: "worker",
		Artifacts: Artifacts{Outputs: []Artifact{{
			ID:       "worker_report",
			Path:     "swe/worker-report.md",
			Required: true,
		}}},
	}

	err = validateFreshRequiredModelOutputs(
		state,
		root,
		map[string]ArtifactFileSnapshot{path: before},
		map[string]ArtifactFileSnapshot{path: after},
	)
	if err == nil {
		t.Fatal("expected stale seeded output to be rejected after state")
	}
	if !strings.Contains(err.Error(), "not freshly written") {
		t.Fatalf("error = %v, want stale output guidance", err)
	}
}

func TestRunEventsFinalTextGateCapturesAndRoutes(t *testing.T) {
	root := t.TempDir()
	verdictPath := filepath.Join(root, "validation-gate.txt")
	personaDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(personaDir, "gate.yaml"), []byte("id: gate\nprompt: Return PASS or BLOCK.\n"), 0o600); err != nil {
		t.Fatalf("write persona: %v", err)
	}
	def := Definition{
		Name:    "final-text-gate-test",
		Initial: "gate",
		States: []State{
			{
				ID:      "gate",
				Persona: "gate",
				Artifacts: Artifacts{Outputs: []Artifact{{
					ID:            "validation_gate",
					Path:          verdictPath,
					Required:      true,
					AllowedValues: []string{"PASS", "BLOCK"},
					RuntimeCapture: ArtifactRuntimeCapture{
						Type: "final_text",
					},
				}}},
			},
			{
				ID: "route_gate",
				Control: Control{ArtifactVerdict: &ArtifactVerdictControl{
					Path:         verdictPath,
					Approve:      "PASS",
					Block:        "BLOCK",
					ApproveEvent: "pass",
					BlockEvent:   "block",
				}},
			},
			{ID: "approved", Terminal: true},
			{ID: "blocked", Terminal: true},
		},
		Transitions: []Transition{
			{Event: EventComplete, From: []string{"gate"}, To: "route_gate"},
			{Event: "pass", From: []string{"route_gate"}, To: "approved"},
			{Event: "block", From: []string{"route_gate"}, To: "blocked"},
		},
	}
	store := app.NewStateStore(app.AppState{
		Conversation: model.NewConversation(model.SystemPrompt{}, "gate-model", "test", "."),
		CWD:          ".",
		Model:        "gate-model",
		Provider:     "test",
		MaxTokens:    1000,
	})
	engine := query.NewEngine(&orchestrationTestProvider{
		name: "test",
		responses: []model.Response{{
			Model:      "gate-model",
			Content:    []model.ContentPart{model.TextPart{Text: "<think>hidden</think>\nBLOCK"}},
			StopReason: model.StopEndTurn,
		}},
	}, store, model.NewCostTracker(0), nil, query.EngineConfig{
		Model:     "gate-model",
		MaxTokens: 1000,
	})

	var transitions []query.OrchestrationTransitionEvent
	for ev := range RunEventsWithOptions(t.Context(), engine, def, RunOptions{
		ArtifactRoot: root,
		PersonaDir:   personaDir,
	}) {
		switch e := ev.(type) {
		case query.ErrorEvent:
			t.Fatalf("unexpected error: %v", e.Err)
		case query.OrchestrationTransitionEvent:
			transitions = append(transitions, e)
		}
	}

	data, err := os.ReadFile(verdictPath)
	if err != nil {
		t.Fatalf("read validation gate artifact: %v", err)
	}
	if got := string(data); got != "BLOCK\n" {
		t.Fatalf("validation gate artifact = %q, want BLOCK newline", got)
	}
	if len(transitions) < 2 {
		t.Fatalf("transitions = %#v, want gate and block route", transitions)
	}
	last := transitions[len(transitions)-1]
	if last.From != "route_gate" || last.Event != "block" || last.To != "blocked" {
		t.Fatalf("last transition = %#v, want route_gate --block--> blocked", last)
	}
}

func TestRunEventsFinalTextGateRetriesMissingAllowedValue(t *testing.T) {
	root := t.TempDir()
	verdictPath := filepath.Join(root, "validation-gate.txt")
	personaDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(personaDir, "gate.yaml"), []byte("id: gate\nprompt: Return PASS or BLOCK.\n"), 0o600); err != nil {
		t.Fatalf("write persona: %v", err)
	}
	def := Definition{
		Name:    "final-text-gate-retry-test",
		Initial: "gate",
		States: []State{
			{
				ID:       "gate",
				Persona:  "gate",
				MaxTurns: 3,
				Artifacts: Artifacts{Outputs: []Artifact{{
					ID:            "validation_gate",
					Path:          verdictPath,
					Required:      true,
					AllowedValues: []string{"PASS", "BLOCK"},
					RuntimeCapture: ArtifactRuntimeCapture{
						Type: "final_text",
					},
				}}},
			},
			{
				ID: "route_gate",
				Control: Control{ArtifactVerdict: &ArtifactVerdictControl{
					Path:         verdictPath,
					Approve:      "PASS",
					Block:        "BLOCK",
					ApproveEvent: "pass",
					BlockEvent:   "block",
				}},
			},
			{ID: "approved", Terminal: true},
			{ID: "blocked", Terminal: true},
		},
		Transitions: []Transition{
			{Event: EventComplete, From: []string{"gate"}, To: "route_gate"},
			{Event: "pass", From: []string{"route_gate"}, To: "approved"},
			{Event: "block", From: []string{"route_gate"}, To: "blocked"},
		},
	}
	store := app.NewStateStore(app.AppState{
		Conversation: model.NewConversation(model.SystemPrompt{}, "gate-model", "test", "."),
		CWD:          ".",
		Model:        "gate-model",
		Provider:     "test",
		MaxTokens:    1000,
	})
	prov := &orchestrationTestProvider{
		name: "test",
		responses: []model.Response{
			{
				Model:      "gate-model",
				Content:    []model.ContentPart{model.ThinkingPart{Text: "reasoning ended before final answer"}},
				StopReason: model.StopMaxTokens,
			},
			{
				Model:      "gate-model",
				Content:    []model.ContentPart{model.TextPart{Text: "BLOCK"}},
				StopReason: model.StopEndTurn,
			},
		},
	}
	engine := query.NewEngine(prov, store, model.NewCostTracker(0), nil, query.EngineConfig{
		Model:     "gate-model",
		MaxTokens: 1000,
	})

	var transitions []query.OrchestrationTransitionEvent
	for ev := range RunEventsWithOptions(t.Context(), engine, def, RunOptions{
		ArtifactRoot: root,
		PersonaDir:   personaDir,
	}) {
		switch e := ev.(type) {
		case query.ErrorEvent:
			t.Fatalf("unexpected error: %v", e.Err)
		case query.OrchestrationTransitionEvent:
			transitions = append(transitions, e)
		}
	}

	data, err := os.ReadFile(verdictPath)
	if err != nil {
		t.Fatalf("read validation gate artifact: %v", err)
	}
	if got := string(data); got != "BLOCK\n" {
		t.Fatalf("validation gate artifact = %q, want BLOCK newline", got)
	}
	if prov.calls != 2 {
		t.Fatalf("provider calls = %d, want retry then success", prov.calls)
	}
	second := fmt.Sprint(prov.requests[1].Messages)
	if !strings.Contains(second, "Final text for artifact `validation_gate` was empty") {
		t.Fatalf("second request missing allowed-value correction: %s", second)
	}
	last := transitions[len(transitions)-1]
	if last.From != "route_gate" || last.Event != "block" || last.To != "blocked" {
		t.Fatalf("last transition = %#v, want route_gate --block--> blocked", last)
	}
}

func TestApplyStateLLMRuntimeUsesStateOverPersona(t *testing.T) {
	baseProvider := &orchestrationTestProvider{name: "base"}
	store := app.NewStateStore(app.AppState{
		Conversation: model.NewConversation(model.SystemPrompt{}, "base-model", "base", "."),
		CWD:          ".",
		Model:        "base-model",
		Provider:     "base",
		MaxTokens:    1000,
	})
	engine := query.NewEngine(baseProvider, store, model.NewCostTracker(0), nil, query.EngineConfig{
		Model:     "base-model",
		MaxTokens: 1000,
	})
	personaTemp := 0.7
	stateTemp := 0.1
	thinking := false
	var resolved llmconfig.Config
	err := applyStateLLMRuntime(context.Background(), engine, func(_ context.Context, cfg llmconfig.Config) (LLMRuntime, error) {
		resolved = cfg
		return LLMRuntime{
			Provider:           &orchestrationTestProvider{name: "openai"},
			ProviderName:       "openai",
			Model:              cfg.Model,
			MaxTokens:          cfg.MaxTokens,
			Temperature:        cfg.Temperature,
			Thinking:           &provider.ThinkingConfig{Enabled: *cfg.Thinking, BudgetTokens: cfg.ThinkingBudget},
			CustomSystemPrompt: cfg.SystemPrompt,
		}, nil
	}, State{
		ID: "worker",
		LLM: llmconfig.Config{
			Model:       "state-model",
			Temperature: &stateTemp,
		},
	}, persona.Definition{
		ID: "worker",
		LLM: llmconfig.Config{
			Provider:       "openai",
			Model:          "persona-model",
			MaxTokens:      4096,
			Temperature:    &personaTemp,
			Thinking:       &thinking,
			ThinkingBudget: 512,
			SystemPrompt:   "persona prompt",
		},
		Prompt: "Persona prompt",
	})
	if err != nil {
		t.Fatalf("applyStateLLMRuntime: %v", err)
	}
	if resolved.Provider != "openai" {
		t.Fatalf("resolved provider = %q, want openai", resolved.Provider)
	}
	if resolved.Model != "state-model" {
		t.Fatalf("resolved model = %q, want state-model", resolved.Model)
	}
	if resolved.MaxTokens != 4096 {
		t.Fatalf("resolved max tokens = %d, want 4096", resolved.MaxTokens)
	}
	if resolved.Temperature == nil || *resolved.Temperature != 0.1 {
		t.Fatalf("resolved temperature = %v, want 0.1", resolved.Temperature)
	}
	snap := store.Snapshot()
	if snap.Provider != "openai" || snap.Model != "state-model" {
		t.Fatalf("store provider/model = %q/%q, want openai/state-model", snap.Provider, snap.Model)
	}
	if snap.MaxTokens != 4096 {
		t.Fatalf("store max tokens = %d, want 4096", snap.MaxTokens)
	}
	if snap.Temperature == nil || *snap.Temperature != 0.1 {
		t.Fatalf("store temperature = %v, want 0.1", snap.Temperature)
	}
	if snap.Thinking == nil || *snap.Thinking {
		t.Fatalf("store thinking = %v, want false", snap.Thinking)
	}
}

func TestApplyStateLLMRuntimeNoLLMIsNoopWithoutResolver(t *testing.T) {
	if err := applyStateLLMRuntime(context.Background(), nil, nil, State{ID: "worker"}, persona.Definition{ID: "worker", Prompt: "Prompt"}); err != nil {
		t.Fatalf("applyStateLLMRuntime: %v", err)
	}
}

func TestLoadChecklistLoopYAML(t *testing.T) {
	def, err := LoadDefinitionFile(filepath.Join("..", "..", "examples", "orchestrations", "architect-checklist-item-loop-final.yaml"))
	if err != nil {
		t.Fatalf("LoadDefinitionFile: %v", err)
	}
	runtime, err := NewRuntime(def)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	for _, event := range []string{
		EventComplete,
		EventComplete,
		EventComplete,
		"item_available",
		EventComplete,
		EventComplete,
		"item_block",
		EventComplete,
		EventComplete,
		"item_approve",
		EventComplete,
		"all_items_done",
		EventComplete,
		"final_block",
		EventComplete,
		"all_items_done",
		EventComplete,
		"final_approve",
	} {
		if err := runtime.FSM.Event(context.Background(), event); err != nil {
			t.Fatalf("event %q from %q: %v", event, runtime.FSM.Current(), err)
		}
	}
	if got := runtime.FSM.Current(); got != "done" {
		t.Fatalf("final state = %q, want done", got)
	}
}

func TestLoadTaskEvidenceItemLoopYAML(t *testing.T) {
	def, err := LoadDefinitionFile(filepath.Join("..", "..", "orchestrations", "task-evidence-item-loop.yaml"))
	if err != nil {
		t.Fatalf("LoadDefinitionFile: %v", err)
	}
	if _, err := NewRuntime(def); err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
}

func TestLoadEveryActiveOrchestrationYAML(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "orchestrations", "*.yaml"))
	if err != nil {
		t.Fatalf("glob orchestration definitions: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("expected active orchestration definitions")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			def, err := LoadDefinitionFile(path)
			if err != nil {
				t.Fatalf("LoadDefinitionFile: %v", err)
			}
			if _, err := NewRuntime(def); err != nil {
				t.Fatalf("NewRuntime: %v", err)
			}
		})
	}
}

func TestTransitionHandoffUnmarshalAndValidation(t *testing.T) {
	var def Definition
	raw := []byte(`
name: handoff-test
initial: first
states:
  - id: first
  - id: second
  - id: done
    terminal: true
transitions:
  - event: complete
    from: [first]
    to: second
    handoff:
      - id: first_report
        path: report.md
        required: true
        allowed_values: [APPROVE]
  - event: complete
    from: [second]
    to: done
`)
	if err := yaml.Unmarshal(raw, &def); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	runtime, err := NewRuntime(def)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	tr, ok := runtime.TransitionFor("first", EventComplete)
	if !ok {
		t.Fatal("expected transition lookup")
	}
	if len(tr.Handoff) != 1 || tr.Handoff[0].ID != "first_report" || !tr.Handoff[0].Required {
		t.Fatalf("handoff = %#v", tr.Handoff)
	}

	def.Transitions[0].Handoff[0].AllowedValues = []string{" "}
	if _, err := NewRuntime(def); err == nil {
		t.Fatal("expected empty allowed transition handoff value to be rejected")
	}
}

func TestNewRuntimeRejectsDuplicateTransitionKey(t *testing.T) {
	_, err := NewRuntime(Definition{
		Name:    "bad",
		Initial: "source",
		States: []State{
			{ID: "source"},
			{ID: "one"},
			{ID: "two"},
		},
		Transitions: []Transition{
			{Event: EventComplete, From: []string{"source"}, To: "one"},
			{Event: EventComplete, From: []string{"source"}, To: "two"},
		},
	})
	if err == nil {
		t.Fatal("expected duplicate transition key to be rejected")
	}
}

func TestTransitionLookupUsesSourceStateAndEvent(t *testing.T) {
	runtime, err := NewRuntime(Definition{
		Name:    "lookup",
		Initial: "a",
		States: []State{
			{ID: "a", Event: Event{Default: "go"}},
			{ID: "b", Event: Event{Default: "go"}},
			{ID: "shared"},
			{ID: "done", Terminal: true},
		},
		Transitions: []Transition{
			{Event: "go", From: []string{"a"}, To: "shared", Handoff: []Artifact{{ID: "from_a", Path: "a.md"}}},
			{Event: "go", From: []string{"b"}, To: "shared", Handoff: []Artifact{{ID: "from_b", Path: "b.md"}}},
			{Event: EventComplete, From: []string{"shared"}, To: "done"},
		},
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	tr, ok := runtime.TransitionFor("a", "go")
	if !ok || len(tr.Handoff) != 1 || tr.Handoff[0].ID != "from_a" {
		t.Fatalf("lookup a/go = %#v, %v", tr, ok)
	}
	tr, ok = runtime.TransitionFor("b", "go")
	if !ok || len(tr.Handoff) != 1 || tr.Handoff[0].ID != "from_b" {
		t.Fatalf("lookup b/go = %#v, %v", tr, ok)
	}
}

func TestNewRuntimeRejectsInvalidTaskPromptMode(t *testing.T) {
	_, err := NewRuntime(Definition{
		Name:    "bad",
		Initial: "worker",
		States: []State{
			{ID: "worker", TaskPrompt: "summary"},
			{ID: "done", Terminal: true},
		},
		Transitions: []Transition{
			{Event: EventComplete, From: []string{"worker"}, To: "done"},
		},
	})
	if err == nil {
		t.Fatal("expected invalid task_prompt mode to be rejected")
	}
}

func TestNewRuntimeRejectsTaskPromptOnControlState(t *testing.T) {
	_, err := NewRuntime(Definition{
		Name:    "bad",
		Initial: "next_item",
		States: []State{
			{
				ID:         "next_item",
				TaskPrompt: TaskPromptNone,
				Control: Control{
					ForEachNext: &ForEachNextControl{
						ListPath:   "/tmp/list.json",
						CursorPath: "/tmp/current.json",
						ItemEvent:  "item",
						DoneEvent:  "done",
					},
				},
			},
			{ID: "done", Terminal: true},
		},
		Transitions: []Transition{
			{Event: "item", From: []string{"next_item"}, To: "done"},
			{Event: "done", From: []string{"next_item"}, To: "done"},
		},
	})
	if err == nil {
		t.Fatal("expected task_prompt on control state to be rejected")
	}
}

func TestNewRuntimeRejectsInvalidArtifacts(t *testing.T) {
	_, err := NewRuntime(Definition{
		Name:    "bad",
		Initial: "worker",
		States: []State{
			{
				ID: "worker",
				Artifacts: Artifacts{
					Inputs: []Artifact{{ID: "current_item"}},
				},
			},
			{ID: "done", Terminal: true},
		},
		Transitions: []Transition{
			{Event: EventComplete, From: []string{"worker"}, To: "done"},
		},
	})
	if err == nil {
		t.Fatal("expected artifact without path to be rejected")
	}
}

func TestRequiredTransitionHandoffFailsBeforeModelCall(t *testing.T) {
	dir := t.TempDir()
	personaDir := filepath.Join(dir, "personas")
	if err := os.MkdirAll(personaDir, 0o755); err != nil {
		t.Fatalf("mkdir persona dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(personaDir, "worker.yaml"), []byte("id: worker\nprompt: Worker prompt\n"), 0o600); err != nil {
		t.Fatalf("write persona: %v", err)
	}
	ch := make(chan query.LoopEvent, 8)
	state := State{ID: "worker", Persona: "worker"}
	_, _, err := runNodeEvents(context.Background(), ch, nil, NewProjection(), personaDir, Definition{Name: "test"}, state, "", "", "", []Artifact{
		{ID: "missing", Path: "missing.md", Required: true},
	}, "source", EventComplete, dir)
	if err == nil {
		t.Fatal("expected missing required handoff to fail")
	}
	if !strings.Contains(err.Error(), "read transition handoff artifact") {
		t.Fatalf("error = %v", err)
	}
}

func TestOptionalTransitionHandoffMissingIsOmitted(t *testing.T) {
	handoff, reads, err := RenderTransitionHandoff(State{ID: "worker"}, []Artifact{
		{ID: "optional_context", Path: "does-not-exist.md"},
	}, t.TempDir(), true)
	if err != nil {
		t.Fatalf("RenderTransitionHandoff: %v", err)
	}
	if handoff != "" {
		t.Fatalf("handoff = %q, want empty", handoff)
	}
	if len(reads) != 0 {
		t.Fatalf("reads = %#v, want none", reads)
	}
}

func TestPromptDoesNotRenderDestinationStateInputs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stale.md"), []byte("STALE_DESTINATION_INPUT"), 0o600); err != nil {
		t.Fatalf("write stale artifact: %v", err)
	}
	_, prompt, err := BuildPromptWithArtifactRootChecked(
		Definition{Name: "test"},
		State{
			ID: "worker",
			Artifacts: Artifacts{Inputs: []Artifact{{
				ID:       "stale",
				Path:     "stale.md",
				Required: true,
			}}},
		},
		testPersona("worker"),
		"",
		"",
		dir,
	)
	if err != nil {
		t.Fatalf("BuildPromptWithArtifactRootChecked: %v", err)
	}
	if strings.Contains(prompt, "STALE_DESTINATION_INPUT") || strings.Contains(prompt, "## Handoff From Previous Phase") {
		t.Fatalf("prompt rendered destination input handoff:\n%s", prompt)
	}
}

func TestStateMaxTurnsValidationAndPromptContract(t *testing.T) {
	def := Definition{
		Name:    "budgeted",
		Initial: "worker",
		States: []State{
			{ID: "worker", Persona: "worker", MaxTurns: 7},
			{ID: "done", Terminal: true},
		},
		Transitions: []Transition{{Event: EventComplete, From: []string{"worker"}, To: "done"}},
	}
	if _, err := NewRuntime(def); err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	system, _, err := BuildPromptWithArtifactRootChecked(def, def.States[0], testPersona("worker"), "", "", t.TempDir())
	if err != nil {
		t.Fatalf("BuildPromptWithArtifactRootChecked: %v", err)
	}
	if len(system.Blocks) == 0 || !strings.Contains(system.Blocks[0].Text, "runtime budget of 7 shell actions") {
		t.Fatalf("system prompt missing max_turns contract:\n%+v", system.Blocks)
	}

	bad := def
	bad.States = []State{{ID: "worker", MaxTurns: -1}, {ID: "done", Terminal: true}}
	if _, err := NewRuntime(bad); err == nil || !strings.Contains(err.Error(), "invalid max_turns -1") {
		t.Fatalf("NewRuntime negative max_turns err = %v", err)
	}
}

func TestRequiredOutputCompletionCheckStillEnforcesOutputs(t *testing.T) {
	dir := t.TempDir()
	state := State{
		ID: "worker",
		Artifacts: Artifacts{Outputs: []Artifact{{
			ID:       "report",
			Path:     "report.md",
			Required: true,
		}}},
	}
	check := requiredOutputArtifactCompletionCheck(state, dir)
	if check == nil {
		t.Fatal("expected completion check")
	}
	ok, guidance, err := check()
	if err != nil {
		t.Fatalf("completion check: %v", err)
	}
	if ok || !strings.Contains(guidance, "report.md") {
		t.Fatalf("ok=%v guidance=%q, want missing report guidance", ok, guidance)
	}
	if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte("done"), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}
	ok, guidance, err = check()
	if err != nil {
		t.Fatalf("completion check after write: %v", err)
	}
	if !ok || guidance != "" {
		t.Fatalf("ok=%v guidance=%q, want success", ok, guidance)
	}
}

func TestRequiredOutputCompletionCheckRejectsWeakAcceptanceMap(t *testing.T) {
	dir := t.TempDir()
	state := State{
		ID: "acceptance_mapper",
		Artifacts: Artifacts{Outputs: []Artifact{{
			ID:       "acceptance_map",
			Path:     "acceptance-map.json",
			Required: true,
			Checks:   acceptanceMapIntegrityChecks(),
		}}},
	}
	content := `{
  "version": 1,
  "source": "task_prompt",
  "acceptance_items": [
    {
      "id": "ACCEPT-THING",
      "task_text": "The feature works.",
      "source_quote": "The feature works",
      "behavior_surface": "config",
      "repo_surfaces_to_verify": ["/workspace/src/feature-config"],
      "required_validation": ["unknown", "command: behavior-check /workspace/src/feature-config"],
      "status": "pending",
      "validation_evidence": [],
      "blocking_if_missing": true,
      "notes": "bad validation"
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(dir, "acceptance-map.json"), []byte(content), 0o600); err != nil {
		t.Fatalf("write acceptance map: %v", err)
	}

	check := requiredOutputArtifactCompletionCheck(state, dir, "The feature works.")
	ok, guidance, err := check()
	if err != nil {
		t.Fatalf("completion check: %v", err)
	}
	if ok {
		t.Fatal("weak acceptance map passed")
	}
	for _, want := range []string{"non-repo-relative path", "placeholder validation", "absolute repository path"} {
		if !strings.Contains(guidance, want) {
			t.Fatalf("guidance missing %q:\n%s", want, guidance)
		}
	}
}

func TestRequiredOutputCompletionCheckAcceptsBehaviorAcceptanceMap(t *testing.T) {
	dir := t.TempDir()
	state := State{
		ID: "acceptance_mapper",
		Artifacts: Artifacts{Outputs: []Artifact{{
			ID:       "acceptance_map",
			Path:     "acceptance-map.json",
			Required: true,
			Checks:   acceptanceMapIntegrityChecks(),
		}}},
	}
	content := `{
  "version": 1,
  "source": "task_prompt",
  "acceptance_items": [
    {
      "id": "ACCEPT-THING",
      "task_text": "The feature works.",
      "source_quote": "The feature works",
      "behavior_surface": "config",
      "repo_surfaces_to_verify": ["src/feature-config"],
      "required_validation": ["run behavior-check feature-config"],
      "status": "pending",
      "validation_evidence": [],
      "blocking_if_missing": true,
      "notes": "behavior validation"
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(dir, "acceptance-map.json"), []byte(content), 0o600); err != nil {
		t.Fatalf("write acceptance map: %v", err)
	}

	check := requiredOutputArtifactCompletionCheck(state, dir, "The feature works.")
	ok, guidance, err := check()
	if err != nil {
		t.Fatalf("completion check: %v", err)
	}
	if !ok || guidance != "" {
		t.Fatalf("ok=%v guidance=%q, want success", ok, guidance)
	}
}

func TestRequiredOutputCompletionCheckRejectsCorruptedSourceQuote(t *testing.T) {
	dir := t.TempDir()
	state := State{
		ID: "acceptance_auditor",
		Artifacts: Artifacts{Outputs: []Artifact{{
			ID:       "acceptance_map",
			Path:     "acceptance-map.json",
			Required: true,
			Checks:   acceptanceMapIntegrityChecks(),
		}}},
	}
	content := `{
  "version": 1,
  "source": "task_prompt",
  "acceptance_items": [
    {
      "id": "ACCEPT-FRAMEWORK",
      "task_text": "The requested feature integrates with the existing framework.",
      "source_quote": "The requested feature integrates with the wrong framework",
      "behavior_surface": "runtime",
      "repo_surfaces_to_verify": ["src/feature-entrypoint"],
      "required_validation": ["unknown"],
      "status": "pending",
      "validation_evidence": [],
      "blocking_if_missing": true,
      "notes": "typo in source quote"
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(dir, "acceptance-map.json"), []byte(content), 0o600); err != nil {
		t.Fatalf("write acceptance map: %v", err)
	}

	check := requiredOutputArtifactCompletionCheck(state, dir, "The requested feature integrates with the existing framework")
	ok, guidance, err := check()
	if err != nil {
		t.Fatalf("completion check: %v", err)
	}
	if ok || !strings.Contains(guidance, "source_quote is not an exact substring") {
		t.Fatalf("ok=%v guidance=%q, want corrupted source quote rejection", ok, guidance)
	}
}

func TestTaskEvidenceItemLoopTransitionHandoffBranchPrompts(t *testing.T) {
	def, err := LoadDefinitionFile(filepath.Join("..", "..", "orchestrations", "task-evidence-item-loop.yaml"))
	if err != nil {
		t.Fatalf("LoadDefinitionFile: %v", err)
	}
	def = relativizeDefinitionArtifactPaths(def)
	runtime, err := NewRuntime(def)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	dir := t.TempDir()
	writeText(t, dir, "current-item.json", `{"id":"item-001","status":"pending"}`)
	writeText(t, dir, "handoff-prompts/next_item/current-item.md", "CURRENT_ITEM_HANDOFF")
	writeText(t, dir, "implementer-report.md", "IMPLEMENTER_REPORT")
	writeText(t, dir, "item-verdict.md", "ITEM_VERDICT")
	writeText(t, dir, "item-block-classification.json", `{"decision":"redo_item_worker","marker":"ITEM_BLOCK_CLASSIFICATION"}`)
	writeText(t, dir, "checklist.json", `{"items":[{"id":"item-001","status":"completed"}]}`)
	writeText(t, dir, "patch-plan.md", "PATCH_PLAN")

	firstWorker := branchPrompt(t, runtime, "next_item", "item_available", "item_worker", dir)
	requireContains(t, firstWorker, "current_item")
	requireContains(t, firstWorker, "CURRENT_ITEM_HANDOFF")
	requireNotContains(t, firstWorker, "ITEM_BLOCK_CLASSIFICATION")

	retryWorker := branchPrompt(t, runtime, "route_item_block_classification", "redo_item_worker", "item_worker", dir)
	requireContains(t, retryWorker, "ITEM_BLOCK_CLASSIFICATION")
	requireContains(t, retryWorker, "ITEM_VERDICT")
	requireContains(t, retryWorker, "IMPLEMENTER_REPORT")

	normalChecklist := branchPrompt(t, runtime, "mark_item_completed", EventComplete, "checklist_writer", dir)
	requireContains(t, normalChecklist, "PATCH_PLAN")
	requireNotContains(t, normalChecklist, "ITEM_BLOCK_CLASSIFICATION")

	repairChecklist := branchPrompt(t, runtime, "route_item_block_classification", "repair_checklist_scope", "checklist_writer", dir)
	requireContains(t, repairChecklist, "ITEM_BLOCK_CLASSIFICATION")
	requireContains(t, repairChecklist, "ITEM_VERDICT")
	requireContains(t, repairChecklist, "PATCH_PLAN")
}

func TestExecuteForEachNextWritesFirstPendingItem(t *testing.T) {
	dir := t.TempDir()
	listPath := filepath.Join(dir, "checklist.json")
	cursorPath := filepath.Join(dir, "current-item.json")
	writeTestChecklist(t, listPath, Checklist{
		Items: []ChecklistItem{
			{ID: "item-001", Status: "approved"},
			{ID: "item-002", Status: "pending"},
		},
	})

	event, err := ExecuteControl(State{
		ID: "next_item",
		Control: Control{
			ForEachNext: &ForEachNextControl{
				ListPath:   listPath,
				CursorPath: cursorPath,
				ItemEvent:  "item_available",
				DoneEvent:  "all_items_done",
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteControl: %v", err)
	}
	if event != "item_available" {
		t.Fatalf("event = %q, want item_available", event)
	}

	var current ChecklistItem
	readTestJSON(t, cursorPath, &current)
	if current.ID != "item-002" {
		t.Fatalf("current item id = %q, want item-002", current.ID)
	}
}

func TestExecuteForEachNextPreservesUnknownItemFields(t *testing.T) {
	dir := t.TempDir()
	listPath := filepath.Join(dir, "checklist.json")
	cursorPath := filepath.Join(dir, "current-item.json")
	raw := []byte(`{
  "items": [
    {
      "id": "item-001",
      "title": "pending",
      "status": "pending",
      "allowed_files": ["server/auth.go"],
      "forbidden_files": ["generated/**"],
      "validation_command": "go test ./server",
      "producer_evidence": {"kind": "patch-plan"}
    }
  ]
}`)
	if err := os.WriteFile(listPath, raw, 0o600); err != nil {
		t.Fatalf("write checklist: %v", err)
	}

	event, err := ExecuteControl(State{
		ID: "next_item",
		Control: Control{
			ForEachNext: &ForEachNextControl{
				ListPath:   listPath,
				CursorPath: cursorPath,
				ItemEvent:  "item_available",
				DoneEvent:  "all_items_done",
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteControl: %v", err)
	}
	if event != "item_available" {
		t.Fatalf("event = %q, want item_available", event)
	}

	var current map[string]any
	readTestJSON(t, cursorPath, &current)
	if _, ok := current["allowed_files"]; !ok {
		t.Fatalf("current item lost allowed_files: %#v", current)
	}
	if _, ok := current["forbidden_files"]; !ok {
		t.Fatalf("current item lost forbidden_files: %#v", current)
	}
	if _, ok := current["validation_command"]; !ok {
		t.Fatalf("current item lost validation_command: %#v", current)
	}
	if _, ok := current["producer_evidence"]; !ok {
		t.Fatalf("current item lost producer_evidence: %#v", current)
	}
}

func TestExecuteForEachNextEmitsDoneWhenNoPendingItems(t *testing.T) {
	dir := t.TempDir()
	listPath := filepath.Join(dir, "checklist.json")
	cursorPath := filepath.Join(dir, "current-item.json")
	writeTestChecklist(t, listPath, Checklist{
		Items: []ChecklistItem{
			{ID: "item-001", Status: "approved"},
		},
	})

	event, err := ExecuteControl(State{
		ID: "next_item",
		Control: Control{
			ForEachNext: &ForEachNextControl{
				ListPath:   listPath,
				CursorPath: cursorPath,
				ItemEvent:  "item_available",
				DoneEvent:  "all_items_done",
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteControl: %v", err)
	}
	if event != "all_items_done" {
		t.Fatalf("event = %q, want all_items_done", event)
	}
}

func TestExecuteMarkCurrentItemUpdatesChecklist(t *testing.T) {
	dir := t.TempDir()
	listPath := filepath.Join(dir, "checklist.json")
	cursorPath := filepath.Join(dir, "current-item.json")
	writeTestChecklist(t, listPath, Checklist{
		Items: []ChecklistItem{
			{ID: "item-001", Status: "pending"},
			{ID: "item-002", Status: "pending"},
		},
	})
	writeTestJSON(t, cursorPath, ChecklistItem{ID: "item-002", Status: "pending"})

	event, err := ExecuteControl(State{
		ID: "mark_item_approved",
		Control: Control{
			MarkCurrentItem: &MarkCurrentItemControl{
				ListPath:   listPath,
				CursorPath: cursorPath,
				Status:     "approved",
				Event:      EventComplete,
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteControl: %v", err)
	}
	if event != EventComplete {
		t.Fatalf("event = %q, want complete", event)
	}

	var checklist Checklist
	readTestJSON(t, listPath, &checklist)
	if checklist.Items[0].Status != "pending" || checklist.Items[1].Status != "approved" {
		t.Fatalf("statuses = %q, %q; want pending, approved", checklist.Items[0].Status, checklist.Items[1].Status)
	}
}

func TestExecuteMarkCurrentItemPreservesUnknownChecklistFields(t *testing.T) {
	dir := t.TempDir()
	listPath := filepath.Join(dir, "checklist.json")
	cursorPath := filepath.Join(dir, "current-item.json")
	raw := []byte(`{
  "items": [
    {
      "id": "item-001",
      "status": "pending",
      "allowed_files": ["server/auth.go"],
      "validation_command": "go test ./server"
    },
    {
      "id": "item-002",
      "status": "pending",
      "allowed_files": ["client/auth.go"],
      "producer_evidence": {"kind": "generated"}
    }
  ]
}`)
	if err := os.WriteFile(listPath, raw, 0o600); err != nil {
		t.Fatalf("write checklist: %v", err)
	}
	if err := os.WriteFile(cursorPath, []byte(`{"id":"item-002","status":"pending","allowed_files":["client/auth.go"],"producer_evidence":{"kind":"generated"}}`), 0o600); err != nil {
		t.Fatalf("write current item: %v", err)
	}

	_, err := ExecuteControl(State{
		ID: "mark_item_approved",
		Control: Control{
			MarkCurrentItem: &MarkCurrentItemControl{
				ListPath:   listPath,
				CursorPath: cursorPath,
				Status:     "approved",
				Event:      EventComplete,
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteControl: %v", err)
	}

	var checklist map[string][]map[string]any
	readTestJSON(t, listPath, &checklist)
	if checklist["items"][0]["allowed_files"] == nil || checklist["items"][0]["validation_command"] == nil {
		t.Fatalf("first item metadata was stripped: %#v", checklist["items"][0])
	}
	if checklist["items"][1]["allowed_files"] == nil || checklist["items"][1]["producer_evidence"] == nil {
		t.Fatalf("second item metadata was stripped: %#v", checklist["items"][1])
	}
	if checklist["items"][1]["status"] != "approved" {
		t.Fatalf("second item status = %v, want approved", checklist["items"][1]["status"])
	}
}

func TestExecuteArtifactVerdictEmitsDecisionEvent(t *testing.T) {
	dir := t.TempDir()
	verdictPath := filepath.Join(dir, "verdict.md")
	if err := os.WriteFile(verdictPath, []byte("Findings:\n- ok\n\nDecision:\nAPPROVE\n"), 0o600); err != nil {
		t.Fatalf("write verdict: %v", err)
	}

	event, err := ExecuteControl(State{
		ID: "route_verdict",
		Control: Control{
			ArtifactVerdict: &ArtifactVerdictControl{
				Path:         verdictPath,
				ApproveEvent: "approve",
				BlockEvent:   "block",
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteControl: %v", err)
	}
	if event != "approve" {
		t.Fatalf("event = %q, want approve", event)
	}
}

func TestExecuteArtifactVerdictAcceptsBareVerdict(t *testing.T) {
	dir := t.TempDir()
	verdictPath := filepath.Join(dir, "verdict.txt")
	if err := os.WriteFile(verdictPath, []byte("BLOCK\n"), 0o600); err != nil {
		t.Fatalf("write verdict: %v", err)
	}

	event, err := ExecuteControl(State{
		ID: "route_verdict",
		Control: Control{
			ArtifactVerdict: &ArtifactVerdictControl{
				Path:         verdictPath,
				Approve:      "PASS",
				Block:        "BLOCK",
				ApproveEvent: "pass",
				BlockEvent:   "block",
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteControl: %v", err)
	}
	if event != "block" {
		t.Fatalf("event = %q, want block", event)
	}
}

func TestSWEBenchProValidationGateBlockRoutesToTheoryKeeper(t *testing.T) {
	def, err := LoadDefinitionFile(filepath.Join("..", "..", "orchestrations", "swe-bench-pro-engineering-loop.yaml"))
	if err != nil {
		t.Fatalf("LoadDefinitionFile: %v", err)
	}
	runtime, err := NewRuntime(def)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	state, ok := runtime.States["route_validation_gate"]
	if !ok {
		t.Fatal("missing route_validation_gate state")
	}
	if state.Control.ArtifactVerdict == nil {
		t.Fatal("route_validation_gate missing artifact_verdict control")
	}
	control := state.Control.ArtifactVerdict
	if control.Approve != "PASS" || control.Block != "BLOCK" {
		t.Fatalf("validation gate values approve=%q block=%q, want PASS/BLOCK", control.Approve, control.Block)
	}
	if control.ApproveEvent != "pass" || control.BlockEvent != "block" {
		t.Fatalf("validation gate events approve=%q block=%q, want pass/block", control.ApproveEvent, control.BlockEvent)
	}

	verdictPath := filepath.Join(t.TempDir(), "validation-gate.txt")
	if err := os.WriteFile(verdictPath, []byte("BLOCK\n"), 0o600); err != nil {
		t.Fatalf("write verdict: %v", err)
	}
	state.Control.ArtifactVerdict.Path = verdictPath

	event, err := ExecuteControl(state)
	if err != nil {
		t.Fatalf("ExecuteControl: %v", err)
	}
	if event != "block" {
		t.Fatalf("event = %q, want block", event)
	}

	runtime.FSM.SetState("route_validation_gate")
	if err := runtime.FSM.Event(context.Background(), event); err != nil {
		t.Fatalf("FSM.Event(%q): %v", event, err)
	}
	if got := runtime.FSM.Current(); got != "swe_theory_keeper" {
		t.Fatalf("state after block = %q, want swe_theory_keeper", got)
	}
}

func TestArtifactDecisionFreshArtifactRejectsStalePreviousOutput(t *testing.T) {
	dir := t.TempDir()
	decisionPath := filepath.Join(dir, "evidence-adjudication.json")
	writeText(t, dir, "evidence-adjudication.json", `{"decision":"validated"}`)
	snapshot, err := snapshotArtifactFile(decisionPath)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	_, err = ExecuteControlWithContext(freshDecisionRouteState(decisionPath), ControlExecutionContext{
		ArtifactRoot: dir,
		PreviousStateOutputs: &StateOutputRun{
			StateID: "swe_single_evidence_adjudicator",
			Outputs: map[string]StateOutputArtifactRun{
				decisionPath: {
					ArtifactID: "evidence_adjudication",
					Path:       decisionPath,
					Before:     snapshot,
					After:      snapshot,
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected stale adjudication artifact to be rejected")
	}
	if !strings.Contains(err.Error(), "not freshly written") {
		t.Fatalf("error = %v, want freshness rejection", err)
	}
}

func TestArtifactDecisionFreshArtifactAcceptsNewPreviousOutput(t *testing.T) {
	dir := t.TempDir()
	decisionPath := filepath.Join(dir, "evidence-adjudication.json")
	writeText(t, dir, "evidence-adjudication.json", `{"decision":"validated"}`)
	after, err := snapshotArtifactFile(decisionPath)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	event, err := ExecuteControlWithContext(freshDecisionRouteState(decisionPath), ControlExecutionContext{
		ArtifactRoot: dir,
		PreviousStateOutputs: &StateOutputRun{
			StateID: "swe_single_evidence_adjudicator",
			Outputs: map[string]StateOutputArtifactRun{
				decisionPath: {
					ArtifactID: "evidence_adjudication",
					Path:       decisionPath,
					Before:     ArtifactFileSnapshot{Path: decisionPath},
					After:      after,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteControlWithContext: %v", err)
	}
	if event != EventComplete {
		t.Fatalf("event = %q, want complete", event)
	}
}

func freshDecisionRouteState(decisionPath string) State {
	return State{
		ID: "route_evidence_adjudication",
		Control: Control{
			ArtifactDecision: &ArtifactDecisionControl{
				Path:  decisionPath,
				Field: "decision",
				FreshArtifacts: []FreshArtifactRequirement{{
					Path: decisionPath,
				}},
				Events: map[string]string{
					"validated": EventComplete,
				},
			},
		},
	}
}

func writeTestChecklist(t *testing.T, path string, checklist Checklist) {
	t.Helper()
	writeTestJSON(t, path, checklist)
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(raw, value); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
}

func TestValidateMarkdownCandidateSurfacesConcrete(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	writeText(t, root, "internal/config/authentication.go", "package config\n")
	report := filepath.Join(root, "repo-survey.md")
	if err := os.WriteFile(report, []byte(`# Repository Survey

Candidate surfaces:
- internal/config/authentication.go - observed config surface
- internal/authn/ or internal/auth/ - guessed auth package
- ui/ - guessed frontend surface
- internal/config/config.go - new method should be added here
`), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}

	issues, err := validateArtifactIntegrityCheck(
		Artifact{ID: "repo_survey"},
		ArtifactIntegrityCheck{Type: "markdown_candidate_surfaces_concrete"},
		State{},
		report,
		root,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := strings.Join(issues, "\n")
	if !strings.Contains(got, "guessed instead of concrete") {
		t.Fatalf("issues missing guessed surface failure:\n%s", got)
	}
	if !strings.Contains(got, "not present in the current repository") {
		t.Fatalf("issues missing absent path failure:\n%s", got)
	}
	if !strings.Contains(got, "implementation instruction") {
		t.Fatalf("issues missing implementation instruction failure:\n%s", got)
	}
	if strings.Contains(got, "internal/config/authentication.go") {
		t.Fatalf("issues unexpectedly rejected existing path:\n%s", got)
	}
}

func TestValidateJSONStringArrayValuesInTaskOrHandoff(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "acceptance-map.json")
	if err := os.WriteFile(report, []byte(`{
  "acceptance_items": [
    {
      "id": "ACCEPT-1",
      "repo_surfaces_to_verify": [
        "internal/config/authentication.go",
        "internal/authn",
        "unknown"
      ]
    }
  ]
}`), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}

	issues, err := validateArtifactIntegrityCheck(
		Artifact{ID: "acceptance_map"},
		ArtifactIntegrityCheck{
			Type:       "json_each_string_array_values_in_task_or_handoff",
			Path:       "acceptance_items",
			Field:      "repo_surfaces_to_verify",
			ArtifactID: "repo_survey",
		},
		State{},
		report,
		root,
		"The task names internal/config/authentication.go explicitly.",
		map[string][]byte{
			"repo_survey": []byte("Candidate surfaces:\n- internal/config/authentication.go - observed config file\n"),
		},
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := strings.Join(issues, "\n")
	if !strings.Contains(got, "internal/authn") {
		t.Fatalf("issues missing ungrounded surface:\n%s", got)
	}
	if strings.Contains(got, "internal/config/authentication.go") {
		t.Fatalf("issues unexpectedly rejected grounded surface:\n%s", got)
	}
	if strings.Contains(got, "unknown") {
		t.Fatalf("issues unexpectedly rejected unknown:\n%s", got)
	}
}

func TestValidateJSONArrayMaxItems(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "slice-plan.json")
	if err := os.WriteFile(report, []byte(`{
  "acceptance_ids": ["ACCEPT-1", "ACCEPT-2", "ACCEPT-3", "ACCEPT-4"]
}`), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}

	issues, err := validateArtifactIntegrityCheck(
		Artifact{ID: "slice_plan"},
		ArtifactIntegrityCheck{Type: "json_array_max_items", Field: "acceptance_ids", Value: "3"},
		State{},
		report,
		root,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := strings.Join(issues, "\n")
	if !strings.Contains(got, "maximum is 3") {
		t.Fatalf("issues missing max-items failure:\n%s", got)
	}
}

func TestValidateJSONNoUnprovenGeneratedOutputsInApprovedEditPaths(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "slice-plan.json")
	if err := os.WriteFile(report, []byte(`{
  "approved_edit_paths": ["internal/config/authentication.go", "rpc/flipt/auth/auth.pb.go"],
  "suspected_coupled_paths": ["rpc/flipt/auth/auth.proto"],
  "generated_policy": "auth.pb.go is source of truth for enum values; protoc unavailable; manual enum addition is correct"
}`), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}

	issues, err := validateArtifactIntegrityCheck(
		Artifact{ID: "slice_plan"},
		ArtifactIntegrityCheck{Type: "json_no_unproven_generated_outputs_in_approved_edit_paths"},
		State{},
		report,
		root,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := strings.Join(issues, "\n")
	if !strings.Contains(got, "auth.pb.go") || !strings.Contains(got, "source-of-truth proof") {
		t.Fatalf("issues missing generated-output failure:\n%s", got)
	}
}

func TestValidateJSONNoUnprovenGeneratedOutputsAllowsProvenSourceOfTruth(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "slice-plan.json")
	if err := os.WriteFile(report, []byte(`{
  "approved_edit_paths": ["rpc/flipt/auth/auth.pb.go"],
  "generated_policy": "Original task and repository documentation make this generated file the source of truth for this change."
}`), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}

	issues, err := validateArtifactIntegrityCheck(
		Artifact{ID: "slice_plan"},
		ArtifactIntegrityCheck{Type: "json_no_unproven_generated_outputs_in_approved_edit_paths"},
		State{},
		report,
		root,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %v, want none", issues)
	}
}

func TestValidateJSONWorkerTrackTargetedValidationRejectsDiscoveryWithCommand(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "slice-plan.json")
	if err := os.WriteFile(report, []byte(`{
  "worker_track": "discovery",
  "mode": "discovery",
  "targeted_validation": "go build ./internal/config/..."
}`), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}

	issues, err := validateArtifactIntegrityCheck(
		Artifact{ID: "slice_plan"},
		ArtifactIntegrityCheck{Type: "json_worker_track_targeted_validation_consistent"},
		State{},
		report,
		root,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := strings.Join(issues, "\n")
	if !strings.Contains(got, "worker_track is discovery") || !strings.Contains(got, "go build ./internal/config/...") {
		t.Fatalf("issues missing worker-track/targeted-validation failure:\n%s", got)
	}
}

func TestValidateJSONWorkerTrackTargetedValidationAllowsImplementationValidation(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "slice-plan.json")
	if err := os.WriteFile(report, []byte(`{
  "worker_track": "implementation",
  "mode": "validation",
  "targeted_validation": "go build ./internal/config/..."
}`), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}

	issues, err := validateArtifactIntegrityCheck(
		Artifact{ID: "slice_plan"},
		ArtifactIntegrityCheck{Type: "json_worker_track_targeted_validation_consistent"},
		State{},
		report,
		root,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %v, want none", issues)
	}
}

func TestValidateJSONWorkerTrackTargetedValidationRejectsValidationWithoutCommand(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "slice-plan.json")
	if err := os.WriteFile(report, []byte(`{
  "worker_track": "discovery",
  "mode": "validation",
  "targeted_validation": "none"
}`), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}

	issues, err := validateArtifactIntegrityCheck(
		Artifact{ID: "slice_plan"},
		ArtifactIntegrityCheck{Type: "json_worker_track_targeted_validation_consistent"},
		State{},
		report,
		root,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := strings.Join(issues, "\n")
	if !strings.Contains(got, "mode validation") || !strings.Contains(got, "targeted_validation is none") {
		t.Fatalf("issues missing validation-without-command failure:\n%s", got)
	}
}

func TestValidateMarkdownNoGeneratedOutputEditRecommendations(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "worker-report.md")
	writeText(t, root, "worker-report.md", `# Worker Report

## Scope request
- paths: ["rpc/flipt/auth/auth.pb.go"]
- evidence: Request scope expansion for adding the missing enum to rpc/flipt/auth/auth.pb.go.
## Next validation suggestion
- Request scope expansion to add METHOD_KUBERNETES=3 to rpc/flipt/auth/auth.pb.go enum, or obtain clean-baseline protobuf regeneration evidence from external tooling.
`)

	issues, err := validateArtifactIntegrityCheck(
		Artifact{ID: "worker_report"},
		ArtifactIntegrityCheck{Type: "markdown_no_generated_output_edit_recommendations"},
		State{},
		report,
		root,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := strings.Join(issues, "\n")
	if !strings.Contains(got, "generated output") {
		t.Fatalf("issues missing generated-output recommendation failure:\n%s", got)
	}
}

func TestValidateMarkdownNoGeneratedOutputEditRecommendationsAllowsProhibition(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "worker-report.md")
	writeText(t, root, "worker-report.md", `# Worker Report

## Worker Contract
- Worker must not: edit rpc/flipt/auth/auth.pb.go for enum additions.
- Scope signal: source-of-truth or producer/toolchain repair required; generated output hand edits are not a valid validation recommendation.
`)

	issues, err := validateArtifactIntegrityCheck(
		Artifact{ID: "worker_report"},
		ArtifactIntegrityCheck{Type: "markdown_no_generated_output_edit_recommendations"},
		State{},
		report,
		root,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %v, want none", issues)
	}
}

func TestValidateMarkdownNoForbiddenWorkerCommandsRejectsBuildFromWorker(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "worker-report.md")
	writeText(t, root, "worker-report.md", `# Worker Report

## Actions taken
- Edited internal/config/authentication.go

## Validation run by worker
- Command: go build ./internal/config/...
- Status: 1
`)

	issues, err := validateArtifactIntegrityCheck(
		Artifact{ID: "worker_report"},
		ArtifactIntegrityCheck{Type: "markdown_no_forbidden_worker_commands", ArtifactID: "plan_audit"},
		State{},
		report,
		root,
		"",
		map[string][]byte{
			"plan_audit": []byte(`## Worker Contract
- Worker must run before reporting: no producer, build, lint, or test commands; targeted validation owns go build ./internal/config/... after the worker report.
`),
		},
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := strings.Join(issues, "\n")
	if !strings.Contains(got, "forbidden") || !strings.Contains(got, "go build") {
		t.Fatalf("issues missing forbidden worker command failure:\n%s", got)
	}
}

func TestValidateMarkdownNoForbiddenWorkerCommandsRejectsRollbackFromWorker(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "worker-report.md")
	writeText(t, root, "worker-report.md", `# Worker Report

## Actions taken
- Ran: git checkout -- internal/config/authentication.go rpc/flipt/auth/auth.proto

## Validation run by worker
- none
`)

	issues, err := validateArtifactIntegrityCheck(
		Artifact{ID: "worker_report"},
		ArtifactIntegrityCheck{Type: "markdown_no_forbidden_worker_commands", ArtifactID: "plan_audit"},
		State{},
		report,
		root,
		"",
		map[string][]byte{
			"plan_audit": []byte(`## Worker Contract
- Worker must run before reporting: no producer, build, lint, or test commands; targeted validation owns none after the worker report.
- Worker must not: rollback/history restore commands.
`),
		},
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := strings.Join(issues, "\n")
	if !strings.Contains(got, "forbidden") || !strings.Contains(got, "git checkout") {
		t.Fatalf("issues missing rollback worker command failure:\n%s", got)
	}
}

func TestValidateMarkdownNoForbiddenWorkerCommandsAllowsScopeOnlyReport(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "worker-report.md")
	writeText(t, root, "worker-report.md", `# Worker Report

## Actions taken
- Inspected internal/config/authentication.go

## Scope request
- status: requested
- paths: ["rpc/flipt/auth/auth.proto"]
- evidence: source-of-truth enum missing outside approved edit paths

## Validation run by worker
- none
`)

	issues, err := validateArtifactIntegrityCheck(
		Artifact{ID: "worker_report"},
		ArtifactIntegrityCheck{Type: "markdown_no_forbidden_worker_commands", ArtifactID: "plan_audit"},
		State{},
		report,
		root,
		"",
		map[string][]byte{
			"plan_audit": []byte(`## Worker Contract
- Worker must run before reporting: no producer, build, lint, or test commands; targeted validation owns none after the worker report.
`),
		},
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %v, want none", issues)
	}
}

func TestValidateCommandEvidenceRepoMutationLimitRejectsMutationSpiral(t *testing.T) {
	root := t.TempDir()
	evidence := filepath.Join(root, "worker-command-evidence.jsonl")
	writeText(t, root, "worker-report.md", `# Worker Report

## Changed files
- internal/config/authentication.go

## Blocker
- none
`)
	writeText(t, root, "worker-command-evidence.jsonl", strings.Join([]string{
		`{"command_preview":"edit proto","command_sha256":"a","output_sha256":"oa","repo_mutation":true}`,
		`{"command_preview":"edit config","command_sha256":"b","output_sha256":"ob","repo_mutation":true}`,
		`{"command_preview":"repair config","command_sha256":"c","output_sha256":"oc","repo_mutation":true}`,
		`{"command_preview":"write report","command_sha256":"d","output_sha256":"od","writes_report":true,"repo_mutation":false}`,
		"",
	}, "\n"))

	issues, err := validateCommandEvidenceRepoMutationLimit(
		Artifact{ID: "worker_report"},
		filepath.Join(root, "worker-report.md"),
		ArtifactIntegrityCheck{Type: "command_evidence_repo_mutation_limit", ArtifactID: "worker_command_evidence", Value: "2"},
		evidence,
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := strings.Join(issues, "\n")
	if !strings.Contains(got, "3 repository mutation commands") || !strings.Contains(got, "exceeding limit 2") {
		t.Fatalf("issues missing mutation limit failure:\n%s", got)
	}
}

func TestValidateCommandEvidenceRepoMutationLimitAllowsTwoMutations(t *testing.T) {
	root := t.TempDir()
	evidence := filepath.Join(root, "worker-command-evidence.jsonl")
	writeText(t, root, "worker-command-evidence.jsonl", strings.Join([]string{
		`{"command_preview":"edit proto","command_sha256":"a","output_sha256":"oa","repo_mutation":true}`,
		`{"command_preview":"edit config","command_sha256":"b","output_sha256":"ob","repo_mutation":true}`,
		`{"command_preview":"write report","command_sha256":"c","output_sha256":"oc","writes_report":true,"repo_mutation":false}`,
		"",
	}, "\n"))

	issues, err := validateCommandEvidenceRepoMutationLimit(
		Artifact{ID: "worker_report"},
		filepath.Join(root, "worker-report.md"),
		ArtifactIntegrityCheck{Type: "command_evidence_repo_mutation_limit", ArtifactID: "worker_command_evidence", Value: "2"},
		evidence,
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %v, want none", issues)
	}
}

func TestValidateCommandEvidenceRepoMutationLimitAllowsExplicitBlocker(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "worker-report.md")
	evidence := filepath.Join(root, "worker-command-evidence.jsonl")
	writeText(t, root, "worker-report.md", `# Worker Report

## Changed files
- internal/config/authentication.go

## Acceptance coverage
- acceptance_ids_addressed: none

## Blocker
- Mutation budget exceeded while applying the approved source edit; source repair needs a fresh plan.
`)
	writeText(t, root, "worker-command-evidence.jsonl", strings.Join([]string{
		`{"command_preview":"edit one","command_sha256":"a","output_sha256":"oa","repo_mutation":true}`,
		`{"command_preview":"edit two","command_sha256":"b","output_sha256":"ob","repo_mutation":true}`,
		`{"command_preview":"edit three","command_sha256":"c","output_sha256":"oc","repo_mutation":true}`,
		`{"command_preview":"write report","command_sha256":"d","output_sha256":"od","writes_report":true,"repo_mutation":false}`,
		"",
	}, "\n"))

	issues, err := validateCommandEvidenceRepoMutationLimit(
		Artifact{ID: "worker_report"},
		report,
		ArtifactIntegrityCheck{Type: "command_evidence_repo_mutation_limit", ArtifactID: "worker_command_evidence", Value: "2"},
		evidence,
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %v, want none", issues)
	}
}

func TestValidateMarkdownWorkerBlockerValidationNoCommandsRejectsBuild(t *testing.T) {
	worker := []byte(`# Worker Report

## Acceptance coverage
- acceptance_ids_addressed: none

## Blocker
- Source edit exceeded mutation budget.
`)
	validation := []byte(`# Targeted Validation

## Commands run
- go build ./...
`)

	issues := validateMarkdownWorkerBlockerValidationNoCommands(
		Artifact{ID: "targeted_validation"},
		validation,
		"worker_report",
		worker,
	)
	got := strings.Join(issues, "\n")
	if !strings.Contains(got, "explicit blocker") || !strings.Contains(got, "go build") {
		t.Fatalf("issues missing blocker validation failure:\n%s", got)
	}
}

func TestValidateMarkdownWorkerBlockerValidationNoCommandsAllowsNone(t *testing.T) {
	worker := []byte(`# Worker Report

## Acceptance coverage
- acceptance_ids_addressed: none

## Blocker
- Source edit exceeded mutation budget.
`)
	validation := []byte(`# Targeted Validation

## Commands run
- none
`)

	issues := validateMarkdownWorkerBlockerValidationNoCommands(
		Artifact{ID: "targeted_validation"},
		validation,
		"worker_report",
		worker,
	)
	if len(issues) != 0 {
		t.Fatalf("issues = %v, want none", issues)
	}
}

func TestValidateCommandEvidenceClaimedChangesRejectsNoRepoMutation(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "worker-report.md")
	evidence := filepath.Join(root, "worker-command-evidence.jsonl")
	writeText(t, root, "worker-report.md", `# Worker Report

## Actions taken
- Applied fixes to server.go and config.go.

## Changed files
- internal/server/auth/method/kubernetes/server.go
- internal/server/auth/method/kubernetes/config.go
`)
	writeText(t, root, "worker-command-evidence.jsonl", strings.Join([]string{
		`{"command_preview":"cat server.go","command_sha256":"a","output_sha256":"oa","repo_mutation":false}`,
		`{"command_preview":"write report","command_sha256":"b","output_sha256":"ob","writes_report":true,"repo_mutation":false}`,
		"",
	}, "\n"))

	issues, err := validateCommandEvidenceClaimedChanges(
		Artifact{ID: "worker_report"},
		report,
		ArtifactIntegrityCheck{Type: "command_evidence_claimed_changes", ArtifactID: "worker_command_evidence"},
		evidence,
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := strings.Join(issues, "\n")
	if !strings.Contains(got, "reports changed files") || !strings.Contains(got, "no non-report repository mutation command") {
		t.Fatalf("issues missing claimed-change failure:\n%s", got)
	}
}

func TestValidateCommandEvidenceClaimedChangesAllowsRepoMutation(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "worker-report.md")
	evidence := filepath.Join(root, "worker-command-evidence.jsonl")
	writeText(t, root, "worker-report.md", `# Worker Report

## Changed files
- internal/config/authentication.go
`)
	writeText(t, root, "worker-command-evidence.jsonl", strings.Join([]string{
		`{"command_preview":"python3 /tmp/patch.py","command_sha256":"a","output_sha256":"oa","repo_mutation":true}`,
		`{"command_preview":"write report","command_sha256":"b","output_sha256":"ob","writes_report":true,"repo_mutation":false}`,
		"",
	}, "\n"))

	issues, err := validateCommandEvidenceClaimedChanges(
		Artifact{ID: "worker_report"},
		report,
		ArtifactIntegrityCheck{Type: "command_evidence_claimed_changes", ArtifactID: "worker_command_evidence"},
		evidence,
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %v, want none", issues)
	}
}

func TestValidateCommandEvidenceNonReportLimitRejectsDiscoveryOverread(t *testing.T) {
	root := t.TempDir()
	evidence := filepath.Join(root, "worker-command-evidence.jsonl")
	writeText(t, root, "worker-command-evidence.jsonl", strings.Join([]string{
		`{"command_preview":"cat internal/config/authentication.go","command_sha256":"a","output_sha256":"oa","repo_mutation":false}`,
		`{"command_preview":"sed -n '1,100p' internal/config/authentication.go","command_sha256":"b","output_sha256":"ob","repo_mutation":false}`,
		`{"command_preview":"write report","command_sha256":"c","output_sha256":"oc","writes_report":true,"repo_mutation":false}`,
		"",
	}, "\n"))

	issues, err := validateCommandEvidenceNonReportLimit(
		Artifact{ID: "worker_report"},
		ArtifactIntegrityCheck{Type: "command_evidence_non_report_limit", ArtifactID: "worker_command_evidence", Value: "1"},
		evidence,
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := strings.Join(issues, "\n")
	if !strings.Contains(got, "2 non-report inspection commands") || !strings.Contains(got, "exceeding limit 1") {
		t.Fatalf("issues missing overread failure:\n%s", got)
	}
}

func TestValidateCommandEvidenceNonReportLimitAllowsSingleInspection(t *testing.T) {
	root := t.TempDir()
	evidence := filepath.Join(root, "worker-command-evidence.jsonl")
	writeText(t, root, "worker-command-evidence.jsonl", strings.Join([]string{
		`{"command_preview":"cat internal/config/authentication.go","command_sha256":"a","output_sha256":"oa","repo_mutation":false}`,
		`{"command_preview":"write report","command_sha256":"b","output_sha256":"ob","writes_report":true,"repo_mutation":false}`,
		"",
	}, "\n"))

	issues, err := validateCommandEvidenceNonReportLimit(
		Artifact{ID: "worker_report"},
		ArtifactIntegrityCheck{Type: "command_evidence_non_report_limit", ArtifactID: "worker_command_evidence", Value: "1"},
		evidence,
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %v, want none", issues)
	}
}

func TestValidateMarkdownScopeRequestRequiresNoChangedFilesRejectsPartialEdit(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "worker-report.md")
	writeText(t, root, "worker-report.md", `# Worker Report

## Changed files
- internal/config/authentication.go

## Scope request
- status: requested
- paths: ["rpc/flipt/auth/auth.proto"]
- evidence: source-of-truth enum missing outside approved edit paths
`)

	issues, err := validateArtifactIntegrityCheck(
		Artifact{ID: "worker_report"},
		ArtifactIntegrityCheck{Type: "markdown_scope_request_requires_no_changed_files"},
		State{},
		report,
		root,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := strings.Join(issues, "\n")
	if !strings.Contains(got, "requests scope expansion") || !strings.Contains(got, "internal/config/authentication.go") {
		t.Fatalf("issues missing partial-edit scope failure:\n%s", got)
	}
}

func TestValidateMarkdownScopeRequestRequiresNoChangedFilesAllowsCleanScopeRequest(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "worker-report.md")
	writeText(t, root, "worker-report.md", `# Worker Report

## Changed files
- none

## Scope request
- status: requested
- paths: ["rpc/flipt/auth/auth.proto"]
- evidence: source-of-truth enum missing outside approved edit paths
`)

	issues, err := validateArtifactIntegrityCheck(
		Artifact{ID: "worker_report"},
		ArtifactIntegrityCheck{Type: "markdown_scope_request_requires_no_changed_files"},
		State{},
		report,
		root,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %v, want none", issues)
	}
}

func TestValidateMarkdownScopeRequestRequiresCleanWorktreeRejectsTrackedDiff(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	runGit(t, root, "init")
	writeText(t, root, "tracked.go", "package tracked\n")
	runGit(t, root, "add", "tracked.go")
	writeText(t, root, "tracked.go", "package tracked\n\nconst Changed = true\n")
	report := filepath.Join(root, "worker-report.md")
	writeText(t, root, "worker-report.md", `# Worker Report

## Changed files
- none

## Scope request
- status: requested
- paths: ["rpc/flipt/auth/auth.proto"]
- evidence: source-of-truth enum missing outside approved edit paths
`)

	issues, err := validateArtifactIntegrityCheck(
		Artifact{ID: "worker_report"},
		ArtifactIntegrityCheck{Type: "markdown_scope_request_requires_clean_worktree"},
		State{},
		report,
		root,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := strings.Join(issues, "\n")
	if !strings.Contains(got, "current worktree has changes") || !strings.Contains(got, "tracked.go") {
		t.Fatalf("issues missing dirty worktree failure:\n%s", got)
	}
}

func TestValidateMarkdownScopeBlockerRequiresNoChangedFiles(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "worker-report.md")
	writeText(t, root, "worker-report.md", `# Worker Report

## Changed files
- internal/config/authentication.go

## Scope adherence
- approved_paths_touched: internal/config/authentication.go
- forbidden_scope_touched: rpc/flipt/auth/auth.pb.go

## Blocker
- Missing enum requires proto source discovery and producer discovery before this slice can proceed.
`)

	issues, err := validateArtifactIntegrityCheck(
		Artifact{ID: "worker_report"},
		ArtifactIntegrityCheck{Type: "markdown_scope_request_requires_no_changed_files"},
		State{},
		report,
		root,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := strings.Join(issues, "\n")
	if !strings.Contains(got, "requests scope expansion") || !strings.Contains(got, "internal/config/authentication.go") {
		t.Fatalf("issues missing changed-file scope blocker failure:\n%s", got)
	}
}

func TestValidateMarkdownScopeBlockerRequiresCleanWorktree(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	runGit(t, root, "init")
	writeText(t, root, "tracked.go", "package tracked\n")
	runGit(t, root, "add", "tracked.go")
	writeText(t, root, "tracked.go", "package tracked\n\nconst Changed = true\n")
	report := filepath.Join(root, "worker-report.md")
	writeText(t, root, "worker-report.md", `# Worker Report

## Changed files
- internal/config/authentication.go

## Scope adherence
- approved_paths_touched: internal/config/authentication.go
- forbidden_scope_touched: rpc/flipt/auth/auth.pb.go

## Blocker
- Incoherent edit references a generated enum outside approved paths; scope expansion is required before this slice can proceed.
`)

	issues, err := validateArtifactIntegrityCheck(
		Artifact{ID: "worker_report"},
		ArtifactIntegrityCheck{Type: "markdown_scope_request_requires_clean_worktree"},
		State{},
		report,
		root,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := strings.Join(issues, "\n")
	if !strings.Contains(got, "current worktree has changes") || !strings.Contains(got, "tracked.go") {
		t.Fatalf("issues missing dirty worktree scope blocker failure:\n%s", got)
	}
}

func TestValidateMarkdownScopeRequestRequiresCleanWorktreeRejectsUntrackedResidue(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	runGit(t, root, "init")
	writeText(t, root, "internal/config/authentication.go.orig", "left by failed patch\n")
	writeText(t, root, "internal/config/authentication.go.rej", "failed hunk\n")
	report := filepath.Join(root, "worker-report.md")
	writeText(t, root, "worker-report.md", `# Worker Report

## Changed files
- none

## Scope request
- status: requested
- paths: ["rpc/flipt/auth/auth.proto"]
- evidence: source-of-truth enum missing outside approved edit paths
`)

	issues, err := validateArtifactIntegrityCheck(
		Artifact{ID: "worker_report"},
		ArtifactIntegrityCheck{Type: "markdown_scope_request_requires_clean_worktree"},
		State{},
		report,
		root,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := strings.Join(issues, "\n")
	if !strings.Contains(got, "current worktree has changes") ||
		!strings.Contains(got, "internal/config/authentication.go.orig") ||
		!strings.Contains(got, "internal/config/authentication.go.rej") {
		t.Fatalf("issues missing untracked residue failure:\n%s", got)
	}
}

func TestValidateMarkdownScopeRequestRequiresCleanWorktreeAllowsCleanRepo(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(repo)
	runGit(t, repo, "init")
	report := filepath.Join(root, "worker-report.md")
	if err := os.WriteFile(report, []byte(`# Worker Report

## Changed files
- none

## Scope request
- status: requested
- paths: ["rpc/flipt/auth/auth.proto"]
- evidence: source-of-truth enum missing outside approved edit paths
`), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}

	issues, err := validateArtifactIntegrityCheck(
		Artifact{ID: "worker_report"},
		ArtifactIntegrityCheck{Type: "markdown_scope_request_requires_clean_worktree"},
		State{},
		report,
		root,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %v, want none", issues)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func TestSWEBenchEngineeringWorkerShellPolicyDeniesUnsafeCommands(t *testing.T) {
	def, err := LoadDefinitionFile(filepath.Join("..", "..", "orchestrations", "swe-bench-pro-engineering-loop.yaml"))
	if err != nil {
		t.Fatalf("LoadDefinitionFile: %v", err)
	}
	var worker State
	for _, state := range def.States {
		if state.ID == "swe_engineering_worker" {
			worker = state
			break
		}
	}
	if worker.ID == "" {
		t.Fatal("missing swe_engineering_worker")
	}
	cfg, ok := commandPolicyConfig(worker, nil, t.TempDir())
	if !ok {
		t.Fatal("missing command policy config")
	}
	for _, command := range []string{
		"cd /app && git checkout internal/config/authentication.go",
		"git checkout -- internal/config/authentication.go rpc/flipt/auth/auth.proto",
		"cd /app && git show HEAD:internal/config/authentication.go > /tmp/original_auth.go && cp /tmp/original_auth.go internal/config/authentication.go",
		"cd /app && git cat-file blob HEAD:internal/config/authentication.go > internal/config/authentication.go",
		"sed -i '304a\\\ntext' /app/internal/config/authentication.go",
		"sed -i 's/METHOD_OIDC = 2;/METHOD_OIDC = 2;\\n  METHOD_KUBERNETES = 3;/' /app/rpc/flipt/auth/auth.proto && grep -A 6 \"enum Method\" /app/rpc/flipt/auth/auth.proto",
		"perl -pi -e 's/old/new/' /app/internal/config/authentication.go",
		"cd /app && perl -pi -e 's/old/new/' /app/internal/config/authentication.go && grep new /app/internal/config/authentication.go",
		"cd /app && patch -p1 < /tmp/k8s_auth_patch.txt",
	} {
		matched := false
		for _, pattern := range cfg.DenyPatterns {
			re, err := regexp.Compile(pattern)
			if err != nil {
				t.Fatalf("compile %q: %v", pattern, err)
			}
			if re.MatchString(command) {
				matched = true
				break
			}
		}
		if !matched {
			t.Fatalf("expected command to match a deny pattern: %q; deny patterns=%v", command, cfg.DenyPatterns)
		}
	}
}

func TestSWEBenchAcceptanceAuditorShellPolicyDeniesStdinSources(t *testing.T) {
	def, err := LoadDefinitionFile(filepath.Join("..", "..", "orchestrations", "swe-bench-pro-engineering-loop.yaml"))
	if err != nil {
		t.Fatalf("LoadDefinitionFile: %v", err)
	}
	var auditor State
	for _, state := range def.States {
		if state.ID == "swe_acceptance_auditor" {
			auditor = state
			break
		}
	}
	if auditor.ID == "" {
		t.Fatal("missing swe_acceptance_auditor")
	}
	cfg, ok := commandPolicyConfig(auditor, nil, t.TempDir())
	if !ok {
		t.Fatal("missing command policy config")
	}
	for _, command := range []string{
		"cp /dev/stdin /tmp/pragma/swe/acceptance-map.json",
		"cat - > /tmp/pragma/swe/acceptance-map.json",
	} {
		matched := false
		for _, pattern := range cfg.DenyPatterns {
			re, err := regexp.Compile(pattern)
			if err != nil {
				t.Fatalf("compile %q: %v", pattern, err)
			}
			if re.MatchString(command) {
				matched = true
				break
			}
		}
		if !matched {
			t.Fatalf("expected command to match a deny pattern: %q; deny patterns=%v", command, cfg.DenyPatterns)
		}
	}
}

func TestValidateJSONRequiredFields(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "slice-plan.json")
	if err := os.WriteFile(report, []byte(`{
  "slice_id": "config-prereq",
  "acceptance_ids": [],
  "targeted_validation": "go test ./internal/config"
}`), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}

	issues, err := validateArtifactIntegrityCheck(
		Artifact{ID: "slice_plan"},
		ArtifactIntegrityCheck{
			Type:   "json_required_fields",
			Fields: []string{"slice_id", "worker_track", "acceptance_ids", "targeted_validation"},
		},
		State{},
		report,
		root,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := strings.Join(issues, "\n")
	if !strings.Contains(got, `missing required field "worker_track"`) {
		t.Fatalf("issues missing absent-field failure:\n%s", got)
	}
	if !strings.Contains(got, `required field "acceptance_ids" is empty`) {
		t.Fatalf("issues missing empty-array failure:\n%s", got)
	}
	if strings.Contains(got, "slice_id") || strings.Contains(got, "targeted_validation") {
		t.Fatalf("issues unexpectedly rejected present non-empty fields:\n%s", got)
	}
}

func TestValidateMarkdownValidationCoverageConsistent(t *testing.T) {
	issues := validateMarkdownValidationCoverageConsistent(
		Artifact{ID: "targeted_validation"},
		[]byte(`# Targeted Validation

## Acceptance coverage
- planned_acceptance_ids: ["ACCEPT-ONE", "ACCEPT-TWO"]
- validated_acceptance_ids: none
- insufficient_acceptance_ids: none - discovery only

## Result
- not_applicable
`),
	)
	got := strings.Join(issues, "\n")
	if !strings.Contains(got, "lists neither validated_acceptance_ids nor insufficient_acceptance_ids") {
		t.Fatalf("issues missing empty coverage failure:\n%s", got)
	}
	if !strings.Contains(got, "result is not_applicable but insufficient_acceptance_ids is empty") {
		t.Fatalf("issues missing non-pass insufficient failure:\n%s", got)
	}

	issues = validateMarkdownValidationCoverageConsistent(
		Artifact{ID: "targeted_validation"},
		[]byte(`# Targeted Validation

## Acceptance coverage
- planned_acceptance_ids: ["ACCEPT-ONE", "ACCEPT-TWO"]
- validated_acceptance_ids: none
- insufficient_acceptance_ids: ["ACCEPT-ONE", "ACCEPT-TWO"]

## Result
- insufficient
`),
	)
	if len(issues) != 0 {
		t.Fatalf("unexpected issues for insufficient coverage:\n%s", strings.Join(issues, "\n"))
	}

	issues = validateMarkdownValidationCoverageConsistent(
		Artifact{ID: "targeted_validation"},
		[]byte(`# Targeted Validation

## Commands run
- go build ./internal/config/...: EXIT_STATUS: 0

## Acceptance coverage
- planned_acceptance_ids: ACCEPT-K8S-AUTH-CONFIG-STRUCT, ACCEPT-K8S-AUTH-DEFAULTS, ACCEPT-K8S-AUTH-FRAMEWORK-INTEGRATION
- validated_acceptance_ids: ACCEPT-K8S-AUTH-CONFIG-STRUCT, ACCEPT-K8S-AUTH-DEFAULTS, ACCEPT-K8S-AUTH-FRAMEWORK-INTEGRATION
- insufficient_acceptance_ids: none

## Result
- pass
`),
	)
	got = strings.Join(issues, "\n")
	if !strings.Contains(got, "compile/static commands cannot prove behavior-sensitive acceptance IDs") ||
		!strings.Contains(got, "ACCEPT-K8S-AUTH-DEFAULTS") ||
		!strings.Contains(got, "ACCEPT-K8S-AUTH-FRAMEWORK-INTEGRATION") {
		t.Fatalf("issues missing weak validation failure:\n%s", got)
	}
	if strings.Contains(got, "ACCEPT-K8S-AUTH-CONFIG-STRUCT") {
		t.Fatalf("compile-only struct claim should remain allowed:\n%s", got)
	}
}

func testPersona(id string) persona.Definition {
	return persona.Definition{ID: id, Prompt: "Persona prompt"}
}

func acceptanceMapIntegrityChecks() []ArtifactIntegrityCheck {
	return []ArtifactIntegrityCheck{
		{Type: "json_field_equals", Field: "source", Value: "task_prompt"},
		{Type: "json_array_non_empty", Path: "acceptance_items"},
		{Type: "json_each_required_fields", Path: "acceptance_items", Fields: []string{"id", "source_quote"}},
		{Type: "json_each_string_substring_of_task_prompt", Path: "acceptance_items", Field: "source_quote"},
		{Type: "json_each_repo_relative_paths", Path: "acceptance_items", Field: "repo_surfaces_to_verify"},
		{Type: "json_each_behavior_validation_commands", Path: "acceptance_items", Field: "required_validation"},
	}
}

func writeText(t *testing.T, root string, rel string, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func relativizeDefinitionArtifactPaths(def Definition) Definition {
	rel := func(path string) string {
		return strings.TrimPrefix(path, "/tmp/pragma/")
	}
	for stateIdx := range def.States {
		for inputIdx := range def.States[stateIdx].Artifacts.Inputs {
			def.States[stateIdx].Artifacts.Inputs[inputIdx].Path = rel(def.States[stateIdx].Artifacts.Inputs[inputIdx].Path)
		}
		for outputIdx := range def.States[stateIdx].Artifacts.Outputs {
			def.States[stateIdx].Artifacts.Outputs[outputIdx].Path = rel(def.States[stateIdx].Artifacts.Outputs[outputIdx].Path)
		}
		if control := def.States[stateIdx].Control.ForEachNext; control != nil {
			control.ListPath = rel(control.ListPath)
			control.CursorPath = rel(control.CursorPath)
			control.HandoffPath = rel(control.HandoffPath)
		}
		if control := def.States[stateIdx].Control.MarkCurrentItem; control != nil {
			control.ListPath = rel(control.ListPath)
			control.CursorPath = rel(control.CursorPath)
		}
		if control := def.States[stateIdx].Control.ArtifactVerdict; control != nil {
			control.Path = rel(control.Path)
		}
		if control := def.States[stateIdx].Control.ArtifactDecision; control != nil {
			control.Path = rel(control.Path)
			for idx := range control.FreshArtifacts {
				control.FreshArtifacts[idx].Path = rel(control.FreshArtifacts[idx].Path)
			}
		}
	}
	for transitionIdx := range def.Transitions {
		for handoffIdx := range def.Transitions[transitionIdx].Handoff {
			def.Transitions[transitionIdx].Handoff[handoffIdx].Path = rel(def.Transitions[transitionIdx].Handoff[handoffIdx].Path)
		}
	}
	return def
}

func branchPrompt(t *testing.T, runtime *Runtime, from string, event string, to string, artifactRoot string) string {
	t.Helper()
	tr, ok := runtime.TransitionFor(from, event)
	if !ok {
		t.Fatalf("missing transition %s --%s", from, event)
	}
	if tr.To != to {
		t.Fatalf("transition %s --%s--> %s, want %s", from, event, tr.To, to)
	}
	state, ok := runtime.States[to]
	if !ok {
		t.Fatalf("missing state %s", to)
	}
	handoff, _, err := RenderTransitionHandoff(state, tr.Handoff, artifactRoot, true)
	if err != nil {
		t.Fatalf("RenderTransitionHandoff %s --%s--> %s: %v", from, event, to, err)
	}
	_, prompt, err := BuildPromptWithArtifactRootChecked(runtime.Definition, state, testPersona(firstNonEmpty(state.Persona, state.ID)), "", handoff, artifactRoot)
	if err != nil {
		t.Fatalf("BuildPromptWithArtifactRootChecked %s: %v", to, err)
	}
	return prompt
}

func requireContains(t *testing.T, text string, needle string) {
	t.Helper()
	if !strings.Contains(text, needle) {
		t.Fatalf("expected prompt to contain %q:\n%s", needle, text)
	}
}

func requireNotContains(t *testing.T, text string, needle string) {
	t.Helper()
	if strings.Contains(text, needle) {
		t.Fatalf("expected prompt not to contain %q:\n%s", needle, text)
	}
}

type orchestrationTestProvider struct {
	name      string
	responses []model.Response
	calls     int
	requests  []provider.RequestParams
}

func (p *orchestrationTestProvider) Name() string { return p.name }

func (p *orchestrationTestProvider) Stream(context.Context, provider.RequestParams) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk)
	close(ch)
	return ch, nil
}

func (p *orchestrationTestProvider) Complete(_ context.Context, params provider.RequestParams) (model.Response, error) {
	p.requests = append(p.requests, params)
	if len(p.responses) > 0 {
		if p.calls >= len(p.responses) {
			return model.Response{}, fmt.Errorf("no response configured for call %d", p.calls+1)
		}
		response := p.responses[p.calls]
		p.calls++
		return response, nil
	}
	return model.Response{}, nil
}

func (p *orchestrationTestProvider) SupportsFeature(provider.Feature) bool { return true }

func (p *orchestrationTestProvider) Pricing(string) (model.Pricing, bool) {
	return model.Pricing{}, false
}

func (p *orchestrationTestProvider) ContextWindow(string) (int, bool) { return 0, false }
