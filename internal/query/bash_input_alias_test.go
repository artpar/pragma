package query

import (
	"github.com/artpar/pragma/internal/model"
	"strings"
	"testing"
)

// INT-002 gates: a Bash tool call carrying the command under the observed
// synonymous key (`command`) executes through the production executor
// instead of failing with "Bash input requires non-empty cmd" (the
// 2026-09-12 live recurrence plus the 2026-08-30 Terminal-Bench instance);
// when no recognized key is present, the error lists the received keys so
// the model self-corrects in one turn. The schema key (`cmd`) keeps
// executing on both baseline and candidate.

func runBashToolCall(t *testing.T, engine *Engine, input string) model.ToolResultPart {
	t.Helper()
	call := model.ToolCallPart{ID: "call-bash-alias", Name: "Bash", Input: []byte(input)}
	return engine.executeProviderToolCall(t.Context(), call, nil)
}

func TestProviderBashToolAcceptsCommandAlias(t *testing.T) {
	engine := newProviderToolsTestEngine(t, &pragmaLoopTestProvider{})

	result := runBashToolCall(t, engine, `{"command": "echo ALIAS_OK"}`)

	if result.IsError {
		t.Fatalf("Bash input carrying the observed `command` alias failed: %q", result.Content)
	}
	if !strings.Contains(result.Content, "ALIAS_OK") {
		t.Fatalf("alias-keyed command did not execute; result: %q", result.Content)
	}
}

func TestProviderBashToolEmptyInputErrorListsReceivedKeys(t *testing.T) {
	engine := newProviderToolsTestEngine(t, &pragmaLoopTestProvider{})

	result := runBashToolCall(t, engine, `{"foo": "bar"}`)

	if !result.IsError {
		t.Fatalf("input with no recognized command key must still be an error; result: %q", result.Content)
	}
	if !strings.Contains(result.Content, "foo") {
		t.Fatalf("error must list the received keys so the model can self-correct; got: %q", result.Content)
	}
}

func TestProviderBashToolSchemaKeyStillExecutes(t *testing.T) {
	engine := newProviderToolsTestEngine(t, &pragmaLoopTestProvider{})

	result := runBashToolCall(t, engine, `{"cmd": "echo CMD_OK"}`)

	if result.IsError {
		t.Fatalf("schema-key Bash input must keep executing (adjacent behavior); got: %q", result.Content)
	}
	if !strings.Contains(result.Content, "CMD_OK") {
		t.Fatalf("schema-keyed command did not execute; result: %q", result.Content)
	}
}
