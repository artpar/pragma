package main

import (
	"path/filepath"
	"testing"
	"time"
)

// TestMetaObserverConfigDefaults locks the operator-specified meta-observe
// parameters: ~3-minute sampling cadence, mechanical budget bounds, and
// the findings location under the observations dir (case
// meta-observer-value-waste-2026-09-24).
func TestMetaObserverConfigDefaults(t *testing.T) {
	cfg := &config{
		home:           "/tmp/pragma-test-home",
		criticProvider: "morphllm",
		criticMaxCalls: 42,
		// criticEvery and activeWithin left zero: defaults must fill in.
	}
	got := metaObserverConfig(cfg, "/queue/path.jsonl")

	if got.Interval != 3*time.Minute {
		t.Errorf("Interval = %v, want 3m (operator: sample every ~3 minutes)", got.Interval)
	}
	if got.ActiveWithin != 15*time.Minute {
		t.Errorf("ActiveWithin = %v, want 15m", got.ActiveWithin)
	}
	if got.LogDir != filepath.Join(cfg.home, "logs") {
		t.Errorf("LogDir = %q, want %s/logs", got.LogDir, cfg.home)
	}
	if got.SessionsDir != filepath.Join(cfg.home, "sessions") {
		t.Errorf("SessionsDir = %q, want %s/sessions", got.SessionsDir, cfg.home)
	}
	if got.QueuePath != "/queue/path.jsonl" {
		t.Errorf("QueuePath = %q, want passthrough", got.QueuePath)
	}
	if want := filepath.Join(observationsDir(cfg), "meta-findings.jsonl"); got.FindingsPath != want {
		t.Errorf("FindingsPath = %q, want %q", got.FindingsPath, want)
	}
	if got.MaxCallsPerHour != 42 {
		t.Errorf("MaxCallsPerHour = %d, want passthrough of --critic-max-calls", got.MaxCallsPerHour)
	}
	if got.CriticTimeout != 90*time.Second {
		t.Errorf("CriticTimeout = %v, want 90s", got.CriticTimeout)
	}
	if got.CriticProvider != "morphllm" {
		t.Errorf("CriticProvider = %q, want passthrough", got.CriticProvider)
	}
}
