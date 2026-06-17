package query

import (
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/model"
)

func TestPragmaLoopRunOptionsOverrideMaxTurns(t *testing.T) {
	engine := &Engine{config: EngineConfig{MaxTurns: 300}}
	run := engine.newPragmaLoopRunConfig(model.SystemPrompt{}, "task", nil, PragmaLoopRunOptions{MaxTurns: 7})
	if run.MaxTurns != 7 {
		t.Fatalf("MaxTurns = %d, want option override 7", run.MaxTurns)
	}
	if !run.ExplicitMaxTurns {
		t.Fatal("ExplicitMaxTurns = false, want true for option override")
	}

	run = engine.newPragmaLoopRunConfig(model.SystemPrompt{}, "task", nil, PragmaLoopRunOptions{})
	if run.MaxTurns != 300 {
		t.Fatalf("MaxTurns = %d, want engine config 300", run.MaxTurns)
	}
	if run.ExplicitMaxTurns {
		t.Fatal("ExplicitMaxTurns = true, want false for engine config")
	}

	engine = &Engine{}
	run = engine.newPragmaLoopRunConfig(model.SystemPrompt{}, "task", nil, PragmaLoopRunOptions{})
	if run.MaxTurns != 300 {
		t.Fatalf("MaxTurns = %d, want shell-loop default 300", run.MaxTurns)
	}
	if run.ExplicitMaxTurns {
		t.Fatal("ExplicitMaxTurns = true, want false for default")
	}
}

func TestAppendPragmaLoopBudgetNoticeOnlyForExplicitMaxTurns(t *testing.T) {
	implicit := pragmaLoopRunConfig{MaxTurns: 80}
	if got := appendPragmaLoopBudgetNotice("observation", implicit, 0); got != "observation" {
		t.Fatalf("implicit notice = %q, want unchanged", got)
	}

	explicit := pragmaLoopRunConfig{MaxTurns: 80, ExplicitMaxTurns: true}
	got := appendPragmaLoopBudgetNotice("observation\n", explicit, 69)
	for _, want := range []string{
		"observation",
		"State turn budget: 70/80 turns used; 10 remaining.",
		"Budget warning:",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("budget notice missing %q:\n%s", want, got)
		}
	}

	got = appendPragmaLoopBudgetNotice("last", explicit, 79)
	if !strings.Contains(got, "State turn budget: 80/80 turns used; 0 remaining.") ||
		!strings.Contains(got, "Budget exhausted:") {
		t.Fatalf("exhausted notice missing expected text:\n%s", got)
	}
}
