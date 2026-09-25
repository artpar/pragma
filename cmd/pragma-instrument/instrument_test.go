package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// INST-001 gate A: a file instrumented in the committed HEAD style —
// full-form (untruncated, single-space) return-trace messages plus block
// rationale comments above the statements — is a fixed point of the
// instrumenter: re-running it must leave the file byte-identical.
// The pre-fix instrumenter both re-adds a truncated duplicate trace line
// (its printed form of a multi-line return carries raw newlines/tabs and
// is truncated, so exact-match dedup misses the full-form message) and
// strips every in-body comment (file.Comments = nil before printing).
const alreadyInstrumentedFixture = `package fixture

import "github.com/artpar/pragma/internal/observe"

const (
	compactMaxConsecutiveFailures = 3
	minTurnsCooldown              = 5
)

// eligible is instrumented in the committed style: full-form messages.
func eligible(disabled bool, failures, turnsSince int) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	// rationale-if: the disabled short-circuit (must survive re-runs).
	if disabled {
		observe.GlobalTrace("return: false")
		return false
	}
	// rationale-return: multi-line return with a full-form trace message;
	// the instrumenter's printed form of this expression carries embedded
	// newlines and tabs and is truncated at the 80-rune bound.
	observe.GlobalTrace("return: !disabled && failures < compactMaxConsecutiveFailures && !(turnsSince < minTurnsCooldown)")
	return !disabled &&
		failures < compactMaxConsecutiveFailures &&
		!(turnsSince < minTurnsCooldown)
}
`

// INST-001 gate B: a never-instrumented file gets traces on the first
// run while every block comment above an if/for/return statement
// survives (and stays above the statement it documents), and a second
// run over that output is byte-identical — the idempotency the Makefile
// comment promises.
const freshFixture = `package fixture

import "strings"

const (
	compactMaxConsecutiveFailures = 3
	minTurnsCooldown              = 5
)

// eligible reports whether the tracker may run.
func eligible(disabled bool, failures, turnsSince int) bool {
	// rationale-if: the disabled short-circuit.
	if disabled {
		return false
	}
	// rationale-for: the cooldown drain loop.
	for i := 0; i < turnsSince; i++ {
		if failures > 0 && i < failures {
			continue
		}
	}
	// rationale-return: the multi-line conjunction whose printed form
	// carries embedded newlines and exceeds the truncation bound.
	return !disabled &&
		failures < compactMaxConsecutiveFailures &&
		!(turnsSince < minTurnsCooldown)
}

// classify mirrors internal/metaobserve/digest.go's digest switch: one
// case body holds only a comment, which must not split the injected
// case-trace selector expression (observed pre-anchor-fix:
// "observe." <comment> "GlobalTrace(...)").
func classify(kind string, ts string) string {
	switch kind {
	case "keepalive":
		// periodic keepalive: no digest value.
	case "error":
		return ts + " error"
	default:
		return ts + " " + kind
	}
}

var _ = strings.TrimSpace
`

func writeFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFixture(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func runInstrumenter(dir string) {
	cfg := instrumentConfig{
		excludePkg:  map[string]bool{"observe": true},
		excludeFunc: parseSet("String,MarshalJSON,UnmarshalJSON,eventSealed,loopEventSealed,Error"),
	}
	_, _, _, _ = processDir(dir, cfg)
}

func TestAlreadyInstrumentedFileIsFixedPoint(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "head.go", alreadyInstrumentedFixture)
	runInstrumenter(dir)
	out := readFixture(t, path)
	if out != alreadyInstrumentedFixture {
		t.Errorf("instrumenter mutated an already-instrumented file (must be a byte-identical fixed point)")
		t.Errorf("got:\n%s", out)
	}
}

func TestInstrumenterIdempotentAndCommentPreserving(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "fresh.go", freshFixture)

	runInstrumenter(dir)
	first := readFixture(t, path)

	for _, comment := range []string{
		"// rationale-if: the disabled short-circuit.",
		"// rationale-for: the cooldown drain loop.",
		"// rationale-return: the multi-line conjunction",
		"// periodic keepalive: no digest value.",
	} {
		if !strings.Contains(first, comment) {
			t.Errorf("first run dropped the block comment %q", comment)
		}
	}
	// The rationale-return comment must stay above the multi-line return
	// it documents, not float below the injected trace line.
	if cIdx, rIdx := strings.Index(first, "// rationale-return:"), strings.Index(first, "return !disabled"); cIdx == -1 || rIdx == -1 || cIdx > rIdx {
		t.Errorf("rationale-return comment no longer sits above its return statement (comment@%d, return@%d)", cIdx, rIdx)
	}
	if !strings.Contains(first, `observe.GlobalTrace("enter")`) {
		t.Errorf("first run did not instrument the function entry")
	}
	if strings.Contains(first, "observe.\n") {
		t.Errorf("injected trace selector was split across lines:\n%s", first)
	}
	if i := strings.Index(first, "case \"keepalive\":"); i != -1 {
		kSlice := first[i:]
		if cIdx, tIdx := strings.Index(kSlice, "// periodic keepalive"), strings.Index(kSlice, "GlobalTrace(\"case: \\\"keepalive\\\""); cIdx > tIdx {
			t.Errorf("keepalive case comment no longer sits with its case clause")
		}
	}
	dupIdx := strings.Count(first, "observe.GlobalTrace(")
	if dupIdx == 0 {
		t.Fatalf("no traces emitted")
	}

	runInstrumenter(dir)
	second := readFixture(t, path)
	if second != first {
		t.Errorf("second run over the instrumenter's own output is not byte-identical (idempotency failure)")
		t.Errorf("first:\n%s\nsecond:\n%s", first, second)
	}
}
