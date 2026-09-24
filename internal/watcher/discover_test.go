package watcher

import (
	"testing"
	"time"
)

func TestIsPragmaCommand(t *testing.T) {
	cases := []struct {
		cmd  string
		want bool
	}{
		{"./bin/pragma --provider morphllm --model m --loop provider-tools", true},
		{"/Users/x/workspace/code/pragma/bin/pragma --provider morphllm", true},
		{"/opt/homebrew/bin/pragma -p hi", true},
		{"/opt/homebrew/bin/pragma.exe -p hi", true},
		{"/opt/homebrew/bin/pragma-watch --once", false},
		{"/tmp/go-build123/exe/pragma-instrument ./internal/...", false},
		{"/opt/homebrew/bin/pragma-watch", false},
		{"grep pragma /var/log/system.log", false},
		{"go run ./cmd/pragma -p test", false}, // argv0 is `go`, not pragma
		{"", false},
	}
	for _, tc := range cases {
		if got := IsPragmaCommand(tc.cmd); got != tc.want {
			t.Errorf("IsPragmaCommand(%q) = %v, want %v", tc.cmd, got, tc.want)
		}
	}
}

func TestParsePSOutput(t *testing.T) {
	now := time.Date(2026, 9, 24, 11, 51, 0, 0, time.Local)
	out := "" +
		"    1 22-00:12:51   0.0 /sbin/launchd\n" +
		"68208       03:53   0.5 ./bin/pragma --provider morphllm --model morph-glm53-744b --loop provider-tools\n" +
		"20661 02-16:15:21   0.6 /Users/artpar/workspace/code/pragma/bin/pragma --provider morphllm --model morph-glm53-744b --loop provider-tools\n" +
		"99999       00:10   0.1 /opt/homebrew/bin/pragma-watch --json\n" +
		"garbage line without numbers\n"

	procs := ParsePSOutput(out, now)
	if len(procs) != 2 {
		t.Fatalf("procs = %+v, want 2 pragma processes", procs)
	}
	byPID := map[int]ProcessInfo{}
	for _, p := range procs {
		byPID[p.PID] = p
	}
	p := byPID[68208]
	if p.Command != "./bin/pragma --provider morphllm --model morph-glm53-744b --loop provider-tools" {
		t.Errorf("68208 command = %q", p.Command)
	}
	if p.CPU != 0.5 {
		t.Errorf("68208 cpu = %v", p.CPU)
	}
	// 03:53 elapsed → started 3m53s before now.
	wantStart := now.Add(-(3*time.Minute + 53*time.Second))
	if !p.StartedAt.Equal(wantStart) {
		t.Errorf("68208 started = %v, want %v", p.StartedAt, wantStart)
	}
	p2 := byPID[20661]
	wantStart2 := now.Add(-(2*24*time.Hour + 16*time.Hour + 15*time.Minute + 21*time.Second))
	if !p2.StartedAt.Equal(wantStart2) {
		t.Errorf("20661 started = %v, want %v", p2.StartedAt, wantStart2)
	}
}

func TestParseEtime(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.Local)
	cases := []struct {
		in     string
		offset time.Duration
		ok     bool
	}{
		{"45", 0, false},
		{"03:53", 3*time.Minute + 53*time.Second, true},
		{"1:03:07", time.Hour + 3*time.Minute + 7*time.Second, true},
		{"02-16:15:21", 2*24*time.Hour + 16*time.Hour + 15*time.Minute + 21*time.Second, true},
		{"bogus", 0, false},
	}
	for _, tc := range cases {
		got, ok := ParseEtime(tc.in, now)
		if ok != tc.ok {
			t.Errorf("ParseEtime(%q) ok = %v, want %v", tc.in, ok, tc.ok)
			continue
		}
		if ok && !got.Equal(now.Add(-tc.offset)) {
			t.Errorf("ParseEtime(%q) = %v, want %v", tc.in, got, now.Add(-tc.offset))
		}
	}
}

func TestParseLogName(t *testing.T) {
	got, ok := ParseLogName("2026-09-24T11-51-08.jsonl")
	if !ok {
		t.Fatal("ParseLogName ok = false")
	}
	want := time.Date(2026, 9, 24, 11, 51, 8, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("ParseLogName = %v, want %v", got, want)
	}
	if _, ok := ParseLogName("notes.jsonl"); ok {
		t.Error("ParseLogName(notes.jsonl) ok = true, want false")
	}
}

func TestMatchProcessToLog(t *testing.T) {
	now := time.Date(2026, 9, 24, 11, 51, 0, 0, time.Local)
	procs := []ProcessInfo{
		{PID: 20661, StartedAt: now.Add(-2 * 24 * time.Hour)},
		{PID: 68208, StartedAt: now.Add(-3 * time.Minute)},
	}
	logStart := now.Add(-3*time.Minute - 8*time.Second)
	p := MatchProcessToLog(procs, logStart, 3*time.Minute)
	if p == nil || p.PID != 68208 {
		t.Fatalf("match = %+v, want PID 68208", p)
	}
	if MatchProcessToLog(procs, logStart.Add(-time.Hour), 3*time.Minute) != nil {
		t.Error("expected no match for distant log")
	}
}
