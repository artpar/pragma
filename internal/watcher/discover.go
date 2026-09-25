package watcher

import (
	"fmt"
	"github.com/artpar/pragma/internal/observe"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ProcessInfo describes one running pragma process.
type ProcessInfo struct {
	PID       int       `json:"pid"`
	Command   string    `json:"command"`
	CPU       float64   `json:"cpu_percent"`
	StartedAt time.Time `json:"started_at"`
	Elapsed   string    `json:"elapsed"` // raw ps etime field
}

// Argv0 returns the first whitespace-delimited token of the command line.
func (p ProcessInfo) Argv0() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	fields := strings.Fields(p.Command)
	if len(fields) == 0 {
		observe.GlobalTrace("if: len(fields) == 0")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	observe.GlobalTrace("return: fields[0]")
	return fields[0]
}

// IsPragmaCommand reports whether a ps command line belongs to a pragma
// harness process (the pragma binary itself, not pragma-watch, not grep).
func IsPragmaCommand(command string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	fields := strings.Fields(command)
	if len(fields) == 0 {
		observe.GlobalTrace("if: len(fields) == 0")
		observe.GlobalTrace("return: false")
		return false
	}
	base := filepath.Base(fields[0])
	observe.GlobalTrace("return: base == \"pragma\" || base == \"pragma.exe\"")
	return base == "pragma" || base == "pragma.exe"
}

// FindPragmaProcesses lists running pragma processes via ps.
func FindPragmaProcesses(now time.Time) ([]ProcessInfo, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	out, err := exec.Command("ps", "-eo", "pid=,etime=,pcpu=,command=").Output()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"ps: %w\", err)")
		return nil, fmt.Errorf("ps: %w", err)
	}
	observe.GlobalTrace("return: ParsePSOutput(string(out), now), nil")
	return ParsePSOutput(string(out), now), nil
}

// ParsePSOutput parses `ps -eo pid=,etime=,pcpu=,command=` output into
// pragma process descriptors. Non-pragma lines are skipped.
func ParsePSOutput(out string, now time.Time) []ProcessInfo {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var procs []ProcessInfo
	for _, line := range strings.Split(out, "\n") {
		observe.GlobalTrace("range strings.Split(out, \"\\n\")")
		line = strings.TrimSpace(line)
		if line == "" {
			observe.GlobalTrace("if: line == \"\"")
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			observe.GlobalTrace("if: len(fields) < 4")
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			continue
		}
		started, ok := ParseEtime(fields[1], now)
		if !ok {
			observe.GlobalTrace("if: !ok")
			continue
		}
		cpu, err := strconv.ParseFloat(fields[2], 64)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			cpu = 0
		}
		command := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), fields[0]))
		command = strings.TrimSpace(strings.TrimPrefix(command, fields[1]))
		command = strings.TrimSpace(strings.TrimPrefix(command, fields[2]))
		if !IsPragmaCommand(command) {
			observe.GlobalTrace("if: !IsPragmaCommand(command)")
			continue
		}
		procs = append(procs, ProcessInfo{
			PID:       pid,
			Command:   command,
			CPU:       cpu,
			StartedAt: started,
			Elapsed:   fields[1],
		})
	}
	observe.GlobalTrace("return: procs")
	return procs
}

// ParseEtime parses a ps elapsed-time field ([[dd-]hh:]mm:ss) and returns
// the process start time relative to now.
func ParseEtime(s string, now time.Time) (time.Time, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var d, h, m, sec int
	if strings.Contains(s, "-") {
		observe.GlobalTrace("if: strings.Contains(s, \"-\")")
		parts := strings.SplitN(s, "-", 2)
		var err error
		d, err = strconv.Atoi(parts[0])
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: time.Time{}, false")
			return time.Time{}, false
		}
		s = parts[1]
	}
	switch parts := strings.Split(s, ":"); len(parts) {
	case 2:
		observe.GlobalTrace("case: 2")
		var err1, err2 error
		m, err1 = strconv.Atoi(parts[0])
		sec, err2 = strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			observe.GlobalTrace("return: time.Time{}, false")
			return time.Time{}, false
		}
	case 3:
		observe.GlobalTrace("case: 3")
		var err1, err2, err3 error
		h, err1 = strconv.Atoi(parts[0])
		m, err2 = strconv.Atoi(parts[1])
		sec, err3 = strconv.Atoi(parts[2])
		if err1 != nil || err2 != nil || err3 != nil {
			observe.GlobalTrace("return: time.Time{}, false")
			return time.Time{}, false
		}
	default:
		observe.GlobalTrace("default")
		return time.Time{}, false
	}
	dur := time.Duration(d)*24*time.Hour + time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(sec)*time.Second
	observe.GlobalTrace("return: now.Add(-dur), true")
	return now.Add(-dur), true
}

// ParseLogName extracts the session start time from a pragma event log
// file name (2006-01-02T15-04-05[.jsonl]).
func ParseLogName(name string) (time.Time, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	base := strings.TrimSuffix(name, ".jsonl")
	t, err := time.ParseInLocation("2006-01-02T15-04-05", base, time.Local)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: time.Time{}, false")
		return time.Time{}, false
	}
	observe.GlobalTrace("return: t, true")
	return t, true
}

// ActiveLogFiles lists pragma event logs in dir whose files were modified
// within the given window of now.
func ActiveLogFiles(dir string, within time.Duration, now time.Time) ([]string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	entries, err := os.ReadDir(dir)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"read log dir %s: %w\", dir, err)")
		return nil, fmt.Errorf("read log dir %s: %w", dir, err)
	}
	var paths []string
	for _, entry := range entries {
		observe.GlobalTrace("range entries")
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			observe.GlobalTrace("if: entry.IsDir() || !strings.HasSuffix(entry.Name(), \".jsonl\")")
			continue
		}
		info, err := entry.Info()
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			continue
		}
		if now.Sub(info.ModTime()) <= within {
			observe.GlobalTrace("if: now.Sub(info.ModTime()) <= within")
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	observe.GlobalTrace("return: paths, nil")
	return paths, nil
}

// MatchProcessToLog picks the process whose start time best matches the
// log file's embedded session start time, within tolerance. Returns nil
// when no process matches.
func MatchProcessToLog(procs []ProcessInfo, logStart time.Time, tolerance time.Duration) *ProcessInfo {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var best *ProcessInfo
	var bestGap time.Duration = tolerance + 1
	for i := range procs {
		observe.GlobalTrace("range procs")
		gap := procs[i].StartedAt.Sub(logStart)
		if gap < 0 {
			observe.GlobalTrace("if: gap < 0")
			gap = -gap
		}
		if gap <= tolerance && gap < bestGap {
			observe.GlobalTrace("if: gap <= tolerance && gap < bestGap")
			best = &procs[i]
			bestGap = gap
		}
	}
	observe.GlobalTrace("return: best")
	return best
}
