package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/watcher"
)

// observationsDir is the default durable record location.
func observationsDir(cfg *config) string {
	if cfg.recordDir != "" {
		return cfg.recordDir
	}
	return filepath.Join(cfg.home, "observations")
}

// startDaemon spawns the detached recording daemon if none is running.
// The daemon survives this terminal, and every pragma session, and
// writes alerts + post-mortems durably under the record dir.
func startDaemon(cfg *config) error {
	record := observationsDir(cfg)
	if err := os.MkdirAll(record, 0o755); err != nil {
		return fmt.Errorf("create record dir: %w", err)
	}
	pidfile := filepath.Join(record, "daemon.pid")
	if pid, alive := daemonAlive(pidfile); alive {
		fmt.Printf("pragma-watch daemon already running (pid %d), recording to %s\n", pid, record)
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	args := []string{
		"--daemon-child",
		"--home", cfg.home,
		"--record", record,
		"--interval", cfg.interval.String(),
		"--active", cfg.activeWithin.String(),
		"--stall", cfg.stallAfter.String(),
	}
	if cfg.ctxWindow > 0 {
		args = append(args, "--ctx", strconv.Itoa(cfg.ctxWindow))
	}
	pid, err := detachSpawn(exe, args, filepath.Join(record, "daemon.log"))
	if err != nil {
		return fmt.Errorf("spawn daemon: %w", err)
	}
	if err := os.WriteFile(pidfile, []byte(strconv.Itoa(pid)), 0o644); err != nil {
		return fmt.Errorf("write pidfile: %w", err)
	}
	fmt.Printf("pragma-watch daemon started (pid %d), recording to %s\n", pid, record)
	fmt.Println("Stop it with: kill " + strconv.Itoa(pid))
	return nil
}

// runReport prints the digest of the durable record: recent post-mortems
// and the alert history. This is the consumption side of the loop — what
// a future session reads to learn from past sessions without replaying
// their full logs.
func runReport(record string) error {
	pmDir := filepath.Join(record, "postmortems")
	entries, err := os.ReadDir(pmDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read postmortems: %w", err)
	}
	type pmFile struct {
		path string
		mod  time.Time
	}
	var files []pmFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".postmortem.json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, pmFile{path: filepath.Join(pmDir, e.Name()), mod: info.ModTime()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	if len(files) > 15 {
		files = files[:15]
	}
	fmt.Printf("pragma-watch report — %s\n", record)
	fmt.Println(strings.Repeat("─", 88))
	if len(files) == 0 {
		fmt.Println("no post-mortems recorded yet")
	}
	for _, f := range files {
		b, err := os.ReadFile(f.path)
		if err != nil {
			continue
		}
		var pm watcher.PostMortem
		if err := json.Unmarshal(b, &pm); err != nil {
			continue
		}
		loop := ""
		if pm.LoopSuspected {
			loop = " · LOOP SUSPECTED"
		}
		fmt.Printf("%s  turns %d · api %d · tools %d (%d err) · ctx peak %s · retr %d · fail %d%s\n",
			pm.Session, pm.Turns, pm.APICalls, pm.ToolCalls, pm.ToolErrors,
			watcher.HumanTokens(pm.ContextPeak), pm.Retries, pm.Failures, loop)
		if pm.LastError != "" {
			fmt.Printf("    last error: %s\n", truncateRunes(pm.LastError, 78))
		}
		if pm.FinalStatus != "" {
			fmt.Printf("    final: %s\n", truncateRunes(pm.FinalStatus, 78))
		}
	}
	alertPath := filepath.Join(record, "alerts.jsonl")
	if b, err := os.ReadFile(alertPath); err == nil {
		lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
		if len(lines) > 200 {
			lines = lines[len(lines)-200:]
		}
		counts := map[string]int{}
		total := 0
		for _, line := range lines {
			if line == "" {
				continue
			}
			var a watcher.Alert
			if err := json.Unmarshal([]byte(line), &a); err == nil {
				counts[a.Kind+"/"+a.Level]++
				total++
			}
		}
		if total > 0 {
			fmt.Println(strings.Repeat("─", 88))
			fmt.Printf("alerts (last %d): ", total)
			var kinds []string
			for k, n := range counts {
				kinds = append(kinds, fmt.Sprintf("%s ×%d", k, n))
			}
			sort.Strings(kinds)
			fmt.Println(strings.Join(kinds, ", "))
		}
	}
	return nil
}
