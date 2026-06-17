package query

import (
	"testing"

	"github.com/artpar/pragma/internal/model"
)

func TestPragmaLoopRunOptionsOverrideMaxTurns(t *testing.T) {
	engine := &Engine{config: EngineConfig{MaxTurns: 300}}
	run := engine.newPragmaLoopRunConfig(model.SystemPrompt{}, "task", nil, PragmaLoopRunOptions{MaxTurns: 7})
	if run.MaxTurns != 7 {
		t.Fatalf("MaxTurns = %d, want option override 7", run.MaxTurns)
	}

	run = engine.newPragmaLoopRunConfig(model.SystemPrompt{}, "task", nil, PragmaLoopRunOptions{})
	if run.MaxTurns != 300 {
		t.Fatalf("MaxTurns = %d, want engine config 300", run.MaxTurns)
	}

	engine = &Engine{}
	run = engine.newPragmaLoopRunConfig(model.SystemPrompt{}, "task", nil, PragmaLoopRunOptions{})
	if run.MaxTurns != 300 {
		t.Fatalf("MaxTurns = %d, want shell-loop default 300", run.MaxTurns)
	}
}
