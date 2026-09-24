// pragma-watch is a live observer for running pragma sessions.
//
// It tails the event logs that pragma itself writes (~/.pragma/logs/*.jsonl),
// reconstructs per-session state (turns, context fill, tool calls, retries,
// MCP servers), detects problems (tool loops, retry storms, context
// pressure, stalled requests), and renders either a terminal dashboard or
// NDJSON snapshots. It is strictly read-only: it never touches pragma
// sessions, only the logs they produce.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/watcher"
)

// ANSI color codes (disabled with --no-color or non-tty stdout).
const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiCyan   = "\x1b[36m"
)

type config struct {
	home         string
	interval     time.Duration
	activeWithin time.Duration
	stallAfter   time.Duration
	ctxWindow    int
	showTools    int
	once         bool
	jsonOut      bool
	noColor      bool
	recordDir    string // when set, durably record alerts + post-mortems
	daemon       bool   // spawn a detached recording daemon
	daemonChild  bool   // internal: this process IS the daemon
	report       bool   // print the durable record digest and exit
}

// tailer follows one event log file incrementally, keeping any partial
// trailing line until its newline arrives.
type tailer struct {
	path     string
	offset   int64
	pending  []byte
	state    *watcher.SessionState
	badLines int
	pmDone   bool // post-mortem already written for this session
}

func newTailer(path string, cfg *config) *tailer {
	state := watcher.NewSessionState(path)
	state.StallAfter = cfg.stallAfter
	state.ContextWindow = cfg.ctxWindow
	return &tailer{path: path, state: state}
}

// pump reads newly appended bytes and folds them into the session state.
func (t *tailer) pump() ([]watcher.Alert, error) {
	f, err := os.Open(t.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if fi.Size() < t.offset { // truncated/rotated: restart from zero
		t.offset = 0
		t.pending = nil
	}
	if fi.Size() == t.offset {
		return nil, nil
	}
	data := make([]byte, fi.Size()-t.offset)
	if _, err := f.ReadAt(data, t.offset); err != nil {
		return nil, err
	}

	combined := append(t.pending, data...)
	consumed := 0
	if idx := lastIndexByte(combined, '\n'); idx >= 0 {
		consumed = idx + 1
		t.pending = append([]byte(nil), combined[consumed:]...)
	} else {
		t.pending = combined
	}
	t.offset += int64(consumed)

	var alerts []watcher.Alert
	for _, line := range strings.Split(string(combined[:consumed]), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		ev, err := observe.UnmarshalEvent([]byte(line))
		if err != nil {
			t.badLines++
			continue
		}
		alerts = append(alerts, t.state.Apply(ev)...)
	}
	return alerts, nil
}

func lastIndexByte(b []byte, c byte) int {
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] == c {
			return i
		}
	}
	return -1
}

// monitor polls processes and logs, maintaining one tailer per active log.
type monitor struct {
	cfg      *config
	sessions map[string]*tailer
}

func newMonitor(cfg *config) *monitor {
	return &monitor{cfg: cfg, sessions: map[string]*tailer{}}
}

// sessionView is the per-session snapshot used for both dashboard
// rendering and --json output.
type sessionView struct {
	Name             string             `json:"name"`
	PID              int                `json:"pid,omitempty"`
	Model            string             `json:"model,omitempty"`
	Status           string             `json:"status"`
	StatusLevel      string             `json:"status_level"`
	Turns            int                `json:"turns"`
	APICalls         int                `json:"api_calls"`
	ToolCalls        int                `json:"tool_calls"`
	ToolErrors       int                `json:"tool_errors"`
	Retries          int                `json:"retries"`
	Failures         int                `json:"failures"`
	LastError        string             `json:"last_error,omitempty"`
	ContextFill      int                `json:"context_fill"`
	ContextWindow    int                `json:"context_window,omitempty"`
	ContextPct       float64            `json:"context_pct,omitempty"`
	OutputTokens     int                `json:"output_tokens"`
	InFlight         bool               `json:"in_flight"`
	InFlightSecs     float64            `json:"in_flight_secs,omitempty"`
	LastEventAgeSecs float64            `json:"last_event_age_secs"`
	RecentTools      []watcher.ToolCall `json:"recent_tools"`
	Servers          map[string]string  `json:"mcp_servers,omitempty"`
	NewAlerts        []watcher.Alert    `json:"new_alerts,omitempty"`
	RecentAlerts     []watcher.Alert    `json:"recent_alerts,omitempty"`
	Ended            bool               `json:"ended"`
}

// snapshot is one full observation of the pragma fleet.
type snapshot struct {
	Time      time.Time             `json:"time"`
	Processes []watcher.ProcessInfo `json:"processes"`
	Sessions  []*sessionView        `json:"sessions"`
}

func (m *monitor) poll(now time.Time) *snapshot {
	cfg := m.cfg
	procs, _ := watcher.FindPragmaProcesses(now)
	logs, err := watcher.ActiveLogFiles(filepath.Join(cfg.home, "logs"), cfg.activeWithin, now)
	if err != nil && len(m.sessions) == 0 {
		fmt.Fprintf(os.Stderr, "pragma-watch: %v\n", err)
	}

	active := map[string]bool{}
	for _, path := range logs {
		active[path] = true
	}
	for path := range m.sessions {
		if !active[path] {
			delete(m.sessions, path)
		}
	}
	var alerts []watcher.Alert
	for _, path := range logs {
		t, ok := m.sessions[path]
		if !ok {
			t = newTailer(path, cfg)
			m.sessions[path] = t
		}
		newAlerts, err := t.pump()
		if err != nil {
			m.recordLog(fmt.Sprintf("tail %s: %v", path, err))
			continue
		}
		alerts = append(alerts, newAlerts...)
	}
	if m.cfg.recordDir != "" && len(alerts) > 0 {
		if err := watcher.AppendAlerts(filepath.Join(m.cfg.recordDir, "alerts.jsonl"), alerts); err != nil {
			m.recordLog(fmt.Sprintf("append alerts: %v", err))
		}
	}
	newBySession := map[string][]watcher.Alert{}
	for _, a := range alerts {
		newBySession[a.Session] = append(newBySession[a.Session], a)
	}

	snap := &snapshot{Time: now, Processes: procs}
	for path, t := range m.sessions {
		view := viewOf(t.state, now)
		view.NewAlerts = newBySession[t.state.Name]
		matched := false
		if logStart, ok := watcher.ParseLogName(view.Name); ok {
			if p := watcher.MatchProcessToLog(procs, logStart, 3*time.Minute); p != nil {
				view.PID = p.PID
				matched = true
			}
		}
		if matched {
			snap.Sessions = append(snap.Sessions, view)
			continue
		}
		// No matching live process: the session's pragma run has ended.
		// Keep the state visible (recent activity) but mark it clearly,
		// and durably record the post-mortem exactly once.
		view.Ended = true
		view.Status = "ended · " + view.Status
		snap.Sessions = append(snap.Sessions, view)
		if m.cfg.recordDir != "" && !t.pmDone {
			t.pmDone = true
			pm := watcher.BuildPostMortem(t.state, now, view.Status)
			pmPath, err := watcher.WritePostMortem(filepath.Join(m.cfg.recordDir, "postmortems"), pm)
			if err != nil {
				m.recordLog(fmt.Sprintf("postmortem %s: %v", path, err))
			} else {
				m.recordLog(fmt.Sprintf("postmortem written: %s", pmPath))
			}
		}
	}
	sort.Slice(snap.Sessions, func(i, j int) bool {
		return snap.Sessions[i].LastEventAgeSecs < snap.Sessions[j].LastEventAgeSecs
	})
	return snap
}

// recordLog appends a line to the record dir's daemon.log when recording
// is enabled; otherwise it writes to stderr.
func (m *monitor) recordLog(msg string) {
	line := time.Now().Format("2006-01-02 15:04:05") + " " + msg + "\n"
	if m.cfg.recordDir == "" {
		fmt.Fprint(os.Stderr, line)
		return
	}
	f, err := os.OpenFile(filepath.Join(m.cfg.recordDir, "daemon.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(line)
}

func viewOf(s *watcher.SessionState, now time.Time) *sessionView {
	status, level := s.Status(now)
	tools := s.RecentTools
	var pct float64
	if s.ContextWindow > 0 && s.ContextFill > 0 {
		pct = 100 * float64(s.ContextFill) / float64(s.ContextWindow)
	}
	recent := s.Alerts
	if len(recent) > 5 {
		recent = recent[len(recent)-5:]
	}
	v := &sessionView{
		Name:             s.Name,
		Model:            s.Model,
		Status:           status,
		StatusLevel:      level,
		Turns:            s.Turns,
		APICalls:         s.APICalls,
		ToolCalls:        s.ToolCalls,
		ToolErrors:       s.ToolErrors,
		Retries:          s.Retries,
		Failures:         s.Failures,
		LastError:        s.LastError,
		ContextFill:      s.ContextFill,
		ContextWindow:    s.ContextWindow,
		ContextPct:       pct,
		OutputTokens:     s.OutputTokens,
		InFlight:         s.InFlight,
		RecentTools:      tools,
		Servers:          s.Servers,
		RecentAlerts:     recent,
		LastEventAgeSecs: now.Sub(s.LastEventAt).Seconds(),
	}
	if s.InFlight {
		v.InFlightSecs = now.Sub(s.InFlightSince).Seconds()
	}
	return v
}

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "pragma-watch: cannot determine home: %v\n", err)
		os.Exit(1)
	}
	cfg := &config{}
	flag.StringVar(&cfg.home, "home", filepath.Join(home, ".pragma"), "pragma home directory")
	flag.StringVar(&cfg.recordDir, "record", "", "durably record alerts + post-mortems to this directory (default: off; daemon uses <home>/observations)")
	flag.DurationVar(&cfg.interval, "interval", time.Second, "poll interval")
	flag.DurationVar(&cfg.activeWithin, "active", 15*time.Minute, "treat logs modified within this window as active")
	flag.DurationVar(&cfg.stallAfter, "stall", 10*time.Minute, "in-flight request duration before a stall alert")
	flag.IntVar(&cfg.ctxWindow, "ctx", 0, "model context window in tokens (enables context % alerts; 0 = unknown)")
	flag.IntVar(&cfg.showTools, "tools", 5, "recent tool calls to show per session")
	flag.BoolVar(&cfg.once, "once", false, "print one snapshot and exit")
	flag.BoolVar(&cfg.jsonOut, "json", false, "emit NDJSON snapshots instead of a dashboard")
	flag.BoolVar(&cfg.noColor, "no-color", false, "disable ANSI colors and screen clearing")
	flag.BoolVar(&cfg.daemon, "daemon", false, "spawn a detached recording daemon (survives this terminal and every pragma session)")
	flag.BoolVar(&cfg.daemonChild, "daemon-child", false, "internal: run as the daemon child")
	flag.BoolVar(&cfg.report, "report", false, "print the durable record digest (post-mortems + alert history) and exit")
	flag.Parse()

	switch {
	case cfg.report:
		dir := cfg.recordDir
		if dir == "" {
			dir = filepath.Join(cfg.home, "observations")
		}
		if err := runReport(dir); err != nil {
			fmt.Fprintf(os.Stderr, "pragma-watch: %v\n", err)
			os.Exit(1)
		}
		return
	case cfg.daemon:
		if err := startDaemon(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "pragma-watch: %v\n", err)
			os.Exit(1)
		}
		return
	case cfg.daemonChild:
		// Recording daemon: no dashboard, no stdout — durable records only.
		if cfg.recordDir == "" {
			cfg.recordDir = filepath.Join(cfg.home, "observations")
		}
		os.MkdirAll(cfg.recordDir, 0o755)
		if cfg.activeWithin < time.Hour {
			cfg.activeWithin = time.Hour // wider window so deaths are caught
		}
		m := newMonitor(cfg)
		m.recordLog("daemon child started")
		for {
			m.poll(time.Now())
			time.Sleep(cfg.interval)
		}
	}

	isTTY := false
	if fi, err := os.Stdout.Stat(); err == nil {
		isTTY = fi.Mode()&os.ModeCharDevice != 0
	}
	useColor := isTTY && !cfg.noColor && !cfg.jsonOut
	clearOK := isTTY && !cfg.jsonOut && !cfg.noColor

	m := newMonitor(cfg)
	if !cfg.once {
		fmt.Fprintf(os.Stderr, "pragma-watch: observing %s every %s (Ctrl-C to stop)\n",
			filepath.Join(cfg.home, "logs"), cfg.interval)
	}
	for {
		snap := m.poll(time.Now())
		if cfg.jsonOut {
			emitJSON(snap)
		} else {
			if clearOK {
				fmt.Print("\x1b[2J\x1b[H")
			}
			fmt.Print(renderDashboard(snap, cfg, useColor))
		}
		if cfg.once {
			return
		}
		time.Sleep(cfg.interval)
	}
}

func emitJSON(snap *snapshot) {
	b, err := json.Marshal(snap)
	if err != nil {
		return
	}
	fmt.Println(string(b))
}

func color(level string, s string, on bool) string {
	if !on {
		return s
	}
	switch level {
	case watcher.LevelError:
		return ansiRed + s + ansiReset
	case watcher.LevelWarning:
		return ansiYellow + s + ansiReset
	case "ok":
		return ansiGreen + s + ansiReset
	case "head":
		return ansiBold + s + ansiReset
	case "dim":
		return ansiDim + s + ansiReset
	default:
		return ansiCyan + s + ansiReset
	}
}

func renderDashboard(snap *snapshot, cfg *config, c bool) string {
	var b strings.Builder
	head := fmt.Sprintf("pragma-watch · %s · %d pragma process(es) · %d active session(s)",
		snap.Time.Format("2006-01-02 15:04:05"), len(snap.Processes), len(snap.Sessions))
	b.WriteString(color("head", head, c) + "\n")
	b.WriteString(strings.Repeat("─", 90) + "\n")

	if len(snap.Processes) > 0 {
		b.WriteString(color("dim", "PROCESSES", c) + "\n")
		for _, p := range snap.Processes {
			uptime := watcher.Dur(time.Since(p.StartedAt))
			line := fmt.Sprintf("  pid %-7d up %-7s cpu %4.1f%%  %s",
				p.PID, uptime, p.CPU, truncateRunes(p.Command, 64))
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}

	if len(snap.Sessions) == 0 {
		b.WriteString("No active pragma sessions in the last " + watcher.Dur(cfg.activeWithin) + ".\n")
		b.WriteString("Start pragma (or wait for a session to emit events) — pragma-watch polls every " +
			watcher.Dur(cfg.interval) + ".\n")
		return b.String()
	}

	for _, s := range snap.Sessions {
		b.WriteString(color("head", "SESSION "+s.Name, c))
		meta := []string{}
		if s.PID > 0 {
			meta = append(meta, fmt.Sprintf("pid %d", s.PID))
		}
		if s.Model != "" {
			meta = append(meta, s.Model)
		}
		if len(meta) > 0 {
			b.WriteString("  " + color("dim", strings.Join(meta, " · "), c))
		}
		b.WriteString("\n")

		b.WriteString(color(s.StatusLevel, "  status   "+s.Status, c) + "\n")

		ctxLine := fmt.Sprintf("  context  %s tok", watcher.HumanTokens(s.ContextFill))
		if s.ContextWindow > 0 {
			ctxLine += fmt.Sprintf(" (%.0f%% of %s window)", s.ContextPct, watcher.HumanTokens(s.ContextWindow))
		} else {
			ctxLine += color("dim", " — pass --ctx <tokens> for %", c)
		}
		b.WriteString(ctxLine + "\n")
		b.WriteString(fmt.Sprintf("  turns %d · api %d · tools %d (%d err) · retries %d · failures %d · out %s tok\n",
			s.Turns, s.APICalls, s.ToolCalls, s.ToolErrors, s.Retries, s.Failures, watcher.HumanTokens(s.OutputTokens)))
		if s.LastError != "" {
			b.WriteString(color(watcher.LevelError, "  last err "+truncateRunes(s.LastError, 80), c) + "\n")
		}
		if servers := serverLine(s.Servers); servers != "" {
			b.WriteString(color("dim", "  mcp      "+servers, c) + "\n")
		}
		if len(s.RecentTools) > 0 {
			shown := s.RecentTools
			if len(shown) > cfg.showTools {
				shown = shown[len(shown)-cfg.showTools:]
			}
			b.WriteString(color("dim", "  recent tools", c) + "\n")
			for _, tc := range shown {
				b.WriteString(fmt.Sprintf("  %s  %-40s %s\n",
					color("dim", tc.At.Format("15:04:05"), c), tc.Name, truncateRunes(tc.Summary, 52)))
			}
		}
		b.WriteString("\n")
	}

	// Global alerts pane: newest last, across sessions.
	var allAlerts []watcher.Alert
	for _, s := range snap.Sessions {
		allAlerts = append(allAlerts, s.RecentAlerts...)
	}
	if len(allAlerts) > 0 {
		sort.Slice(allAlerts, func(i, j int) bool { return allAlerts[i].At.Before(allAlerts[j].At) })
		if len(allAlerts) > 6 {
			allAlerts = allAlerts[len(allAlerts)-6:]
		}
		b.WriteString(color("dim", "ALERTS (recent)", c) + "\n")
		for _, a := range allAlerts {
			b.WriteString(fmt.Sprintf("  %s %s [%s] %s\n",
				a.At.Format("15:04:05"), marker(a.Level, c), a.Kind, truncateRunes(a.Message, 76)))
		}
	}
	return b.String()
}

func marker(level string, c bool) string {
	switch level {
	case watcher.LevelError:
		return color(watcher.LevelError, "✗", c)
	case watcher.LevelWarning:
		return color(watcher.LevelWarning, "⚠", c)
	default:
		return color("dim", "·", c)
	}
}

func serverLine(servers map[string]string) string {
	if len(servers) == 0 {
		return ""
	}
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, len(names))
	for i, name := range names {
		parts[i] = name + "=" + servers[name]
	}
	return strings.Join(parts, ", ")
}

func truncateRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
