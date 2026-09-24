package metaobserve

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/watcher"
)

const fixtureLogName = "2026-09-24T16-58-58.jsonl"

var fixtureLogStart = mustParseTime("2026-09-24T16:58:58+05:30")

func mustParseTime(s string) time.Time {
	ts, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		panic(err)
	}
	return ts
}

func fixtureLines(t *testing.T, name string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	var out []string
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

// fakeCritic records what the observer sends and returns canned findings.
type fakeCritic struct {
	calls    int
	digest   string
	titles   []string
	findings []Finding
	err      error
}

func (f *fakeCritic) Judge(_ context.Context, digest string, titles []string) ([]Finding, error) {
	f.calls++
	f.digest = digest
	f.titles = titles
	return f.findings, f.err
}

// fakeProvider stands in for a real provider.Provider.
type fakeProvider struct {
	params []provider.RequestParams
	resp   model.Response
	err    error
}

func (p *fakeProvider) Name() string { return "fake" }
func (p *fakeProvider) Stream(_ context.Context, _ provider.RequestParams) (<-chan provider.StreamChunk, error) {
	return nil, fmt.Errorf("not implemented")
}
func (p *fakeProvider) Complete(_ context.Context, params provider.RequestParams) (model.Response, error) {
	p.params = append(p.params, params)
	return p.resp, p.err
}
func (p *fakeProvider) SupportsFeature(_ provider.Feature) bool { return false }
func (p *fakeProvider) Pricing(_ string) (model.Pricing, bool)  { return model.Pricing{}, false }
func (p *fakeProvider) ContextWindow(_ string) (int, bool)      { return 0, false }

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatalf("parse time %q: %v", s, err)
	}
	return ts
}

func TestDigestCarriesAuthenticTailSignals(t *testing.T) {
	s := &SessionSample{
		LogName:     fixtureLogName,
		SessionID:   "de57a18f-f5a3-487c-9ff7-78e501c0ca2d",
		LogLines:    fixtureLines(t, "log-tail.jsonl"),
		SessionMsgs: fixtureLines(t, "session-tail.jsonl"),
		WindowFrom:  mustTime(t, "2026-09-24T16:58:58+05:30"),
		WindowTo:    mustTime(t, "2026-09-24T17:02:00+05:30"),
	}
	d := s.Digest(12000)
	for _, want := range []string{
		"de57a18f-f5a3-487c-9ff7-78e501c0ca2d", // session link
		"Bash",                                 // tool call name
		"cat /Users/artpar/workspace/code/pragma/agent.md",  // tool input
		"Let me start by reading agent.md",                  // assistant thinking
		"WORKER instance in pragma's self-improvement loop", // operator text
		"stop=tool_use", // request outcome
	} {
		if !strings.Contains(d, want) {
			t.Errorf("digest missing %q\ndigest:\n%s", want, d)
		}
	}
	// Usage and cost signals for "context growth per unit of progress".
	if !strings.Contains(d, "in=") || !strings.Contains(d, "out=") {
		t.Errorf("digest missing per-request usage: %s", d)
	}
	if !strings.Contains(d, "1739909") {
		t.Errorf("digest missing cumulative input tokens from session metadata")
	}
	if n := len(d); n > 13000 {
		t.Errorf("digest unbounded: %d chars", n)
	}
}

func TestCollectSampleWindowsAndLinks(t *testing.T) {
	dir := t.TempDir()
	logDir := filepath.Join(dir, "logs")
	sessDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(logDir, fixtureLogName)
	if err := os.WriteFile(logPath, []byte(strings.Join(fixtureLines(t, "log-tail.jsonl"), "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sessID := "de57a18f-f5a3-487c-9ff7-78e501c0ca2d"
	if err := os.WriteFile(filepath.Join(sessDir, sessID+".jsonl"),
		[]byte(strings.Join(fixtureLines(t, "session-tail.jsonl"), "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	from := mustTime(t, "2026-09-24T16:58:50+05:30")
	to := mustTime(t, "2026-09-24T17:02:00+05:30")
	s, err := CollectSample(logPath, sessDir, from, to, 120, 40)
	if err != nil {
		t.Fatalf("CollectSample: %v", err)
	}
	if s.SessionID != sessID {
		t.Errorf("SessionID = %q, want %q", s.SessionID, sessID)
	}
	if len(s.LogLines) < 10 {
		t.Errorf("LogLines = %d, want >= 10 (full fixture window)", len(s.LogLines))
	}
	if len(s.SessionMsgs) != 4 {
		t.Errorf("SessionMsgs = %d, want 4 (3 messages + metadata)", len(s.SessionMsgs))
	}

	// A window that starts after every fixture event samples nothing.
	lateFrom := mustTime(t, "2026-09-24T17:30:00+05:30")
	s2, err := CollectSample(logPath, sessDir, lateFrom, lateFrom.Add(time.Minute), 120, 40)
	if err != nil {
		t.Fatalf("CollectSample (late): %v", err)
	}
	if len(s2.LogLines) != 0 {
		t.Errorf("late-window LogLines = %d, want 0", len(s2.LogLines))
	}
}

func TestProviderCriticPromptAndParse(t *testing.T) {
	fp := &fakeProvider{}
	fp.resp = model.Response{
		Content: []model.ContentPart{
			model.TextPart{Text: "Here are findings:\n" + `[
{"kind":"waste","finding":"re-read agent.md twice within the window","evidence":"two identical Bash cat calls","confidence":"high"},
{"kind":"spec_correction","finding":"operator asked to stop stamp-only messages; not in queue","evidence":"operator text line 3","confidence":"medium"},
{"kind":"nonsense","finding":"bad kind dropped","evidence":"x","confidence":"high"}
]`},
		},
	}
	c := &ProviderCritic{Prov: fp, Model: "morph-glm53-744b", MaxTokens: 900}
	findings, err := c.Judge(context.Background(), "SESSION TAIL DIGEST lorem ipsum", []string{"META-observe: Periodic model-based meta-observer", "CLK-002: Wall-clock stamps"})
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if len(fp.params) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(fp.params))
	}
	p := fp.params[0]
	if p.Model != "morph-glm53-744b" {
		t.Errorf("request model = %q", p.Model)
	}
	sysText := systemText(p.System)
	for _, marker := range []string{"wasted motion", "NOT checking", "spec corrections"} {
		if !strings.Contains(sysText, marker) {
			t.Errorf("critic system prompt missing question %q:\n%s", marker, sysText)
		}
	}
	userText := messageText(p.Messages)
	if !strings.Contains(userText, "SESSION TAIL DIGEST") {
		t.Errorf("critic user prompt missing digest")
	}
	if !strings.Contains(userText, "META-observe: Periodic model-based meta-observer") {
		t.Errorf("critic user prompt missing queue titles")
	}
	if got, want := len(findings), 2; got != want {
		t.Fatalf("findings = %d, want %d (bad kind dropped), got %+v", got, want, findings)
	}
	if findings[0].Kind != "waste" || findings[1].Kind != "spec_correction" {
		t.Errorf("finding kinds = %q,%q", findings[0].Kind, findings[1].Kind)
	}

	// Extraction robustness: JSON inside prose, oversized arrays bounded.
	long := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		long = append(long, fmt.Sprintf(`{"kind":"waste","finding":"f%d","evidence":"e","confidence":"low"}`, i))
	}
	got := ParseFindings("prefix text [" + strings.Join(long, ",") + "] suffix")
	if len(got) != 5 {
		t.Errorf("ParseFindings bounded array: got %d, want 5", len(got))
	}
	if ParseFindings("no json here") != nil {
		t.Errorf("ParseFindings on plain text should return nil")
	}
}

func newTestEnv(t *testing.T) (cfg Config, critic *fakeCritic) {
	t.Helper()
	dir := t.TempDir()
	logDir := filepath.Join(dir, "logs")
	sessDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(logDir, fixtureLogName)
	if err := os.WriteFile(logPath, []byte(strings.Join(fixtureLines(t, "log-tail.jsonl"), "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sessID := "de57a18f-f5a3-487c-9ff7-78e501c0ca2d"
	if err := os.WriteFile(filepath.Join(sessDir, sessID+".jsonl"),
		[]byte(strings.Join(fixtureLines(t, "session-tail.jsonl"), "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	queuePath := filepath.Join(dir, "queue.jsonl")
	seedQueue := []string{
		`{"id":"X-1","title":"Seed queue item alpha","kind":"case-revision","status":"open","added_at":"2026-09-24T10:00:00+05:30"}`,
		`{"id":"X-2","title":"META-observe: Periodic model-based meta-observer","kind":"operator-directive","status":"in_progress","added_at":"2026-09-24T16:18:00+05:30"}`,
	}
	if err := os.WriteFile(queuePath, []byte(strings.Join(seedQueue, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg = Config{
		LogDir:          logDir,
		SessionsDir:     sessDir,
		QueuePath:       queuePath,
		FindingsPath:    filepath.Join(dir, "findings.jsonl"),
		Interval:        3 * time.Minute,
		FirstWindow:     10 * time.Minute,
		ActiveWithin:    time.Hour,
		MaxLogLines:     120,
		MaxSessionMsgs:  40,
		MaxDigestChars:  12000,
		MaxCallsPerHour: 2,
		MaxQueueItems:   100,
		CriticTimeout:   30 * time.Second,
		CriticProvider:  "morphllm",
		CriticModel:     "morph-glm53-744b",
	}
	critic = &fakeCritic{findings: []Finding{
		{Kind: "waste", Finding: "re-reads agent.md repeatedly", Evidence: "two identical cat calls", Confidence: "high"},
		{Kind: "spec_correction", Finding: "operator correction not in queue", Evidence: "operator text", Confidence: "medium"},
	}}
	return cfg, critic
}

func matchingProcessLister(now time.Time) ([]watcher.ProcessInfo, error) {
	return []watcher.ProcessInfo{{PID: 1, Command: "/usr/local/bin/pragma --provider morphllm", StartedAt: fixtureLogStart}}, nil
}

func readJSONL(t *testing.T, path string) []map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var out []map[string]any
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("bad jsonl line: %v\n%s", err, l)
		}
		out = append(out, m)
	}
	return out
}

func TestObserverTickLifecycle(t *testing.T) {
	cfg, critic := newTestEnv(t)
	o := NewObserver(cfg, critic, os.Stderr)
	o.ProcessLister = matchingProcessLister
	now := mustTime(t, "2026-09-24T17:04:00+05:30")

	// Tick 1: first sample of a live session — one critic call, two
	// findings appended to both the queue and the durable findings file,
	// each with session+time provenance and the critic model.
	n, err := o.Tick(context.Background(), now)
	if err != nil {
		t.Fatalf("tick 1: %v", err)
	}
	if n != 2 {
		t.Fatalf("tick 1 appended = %d, want 2", n)
	}
	if critic.calls != 1 {
		t.Fatalf("critic calls after tick 1 = %d, want 1", critic.calls)
	}
	for _, want := range []string{"Seed queue item alpha", "META-observe: Periodic model-based meta-observer"} {
		if !contains(critic.titles, want) {
			t.Errorf("critic queue titles missing %q, got %v", want, critic.titles)
		}
	}
	queue := readJSONL(t, cfg.QueuePath)
	if len(queue) != 4 { // 2 seed + 2 findings
		t.Fatalf("queue lines after tick 1 = %d, want 4", len(queue))
	}
	var kinds []string
	for _, rec := range queue[2:] {
		if rec["kind"] != "meta-finding" {
			t.Errorf("queue entry kind = %v, want meta-finding", rec["kind"])
		}
		if rec["status"] != "open" {
			t.Errorf("queue entry status = %v, want open", rec["status"])
		}
		if !strings.HasPrefix(fmt.Sprint(rec["id"]), "META-OBS-") {
			t.Errorf("queue entry id = %v, want META-OBS- prefix", rec["id"])
		}
		payload := rec["payload"].(map[string]any)
		prov := payload["provenance"].(map[string]any)
		if prov["session_log"] != fixtureLogName {
			t.Errorf("provenance session_log = %v, want %q", prov["session_log"], fixtureLogName)
		}
		if prov["sampled_at"] != now.Format(time.RFC3339Nano) && prov["sampled_at"] != now.Format(time.RFC3339) {
			t.Errorf("provenance sampled_at = %v, want %v", prov["sampled_at"], now)
		}
		if prov["critic_model"] != "morph-glm53-744b" {
			t.Errorf("provenance critic_model = %v", prov["critic_model"])
		}
		f := payload["finding"].(map[string]any)
		kinds = append(kinds, fmt.Sprint(f["kind"]))
	}
	if !contains(kinds, "waste") || !contains(kinds, "spec_correction") {
		t.Errorf("appended finding kinds = %v", kinds)
	}
	if len(readJSONL(t, cfg.FindingsPath)) != 2 {
		t.Errorf("durable findings file should mirror the queue appends")
	}

	// Tick 2: no new bytes — no critic call, no appends.
	if n, err := o.Tick(context.Background(), now.Add(3*time.Minute)); err != nil {
		t.Fatalf("tick 2: %v", err)
	} else if n != 0 {
		t.Fatalf("tick 2 appended = %d, want 0 (no new bytes)", n)
	}
	if critic.calls != 1 {
		t.Fatalf("critic calls after tick 2 = %d, want 1 (skipped, no new bytes)", critic.calls)
	}

	// Tick 3: new log bytes; session link via the remembered id; the
	// findings are duplicates — nothing appended.
	logPath := filepath.Join(cfg.LogDir, fixtureLogName)
	newEvent := `{"kind":"MessageAppended","time":"2026-09-24T17:07:30+05:30","trace_id":"","span_id":"s","message_id":"m","role":"user","content_types":["tool_result"],"token_estimate":400}` + "\n"
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(newEvent); err != nil {
		t.Fatal(err)
	}
	f.Close()
	now3 := mustTime(t, "2026-09-24T17:07:31+05:30")
	if n, err := o.Tick(context.Background(), now3); err != nil {
		t.Fatalf("tick 3: %v", err)
	} else if n != 0 {
		t.Fatalf("tick 3 appended = %d, want 0 (dedup)", n)
	}
	if critic.calls != 2 {
		t.Fatalf("critic calls after tick 3 = %d, want 2", critic.calls)
	}
	if !strings.Contains(critic.digest, "de57a18f-f5a3-487c-9ff7-78e501c0ca2d") {
		t.Errorf("tick 3 digest lost the remembered session id:\n%s", critic.digest)
	}
	if len(readJSONL(t, cfg.QueuePath)) != 4 {
		t.Errorf("queue lines after dedup = %d, want 4", len(readJSONL(t, cfg.QueuePath)))
	}

	// Tick 4: budget cap (2 calls/hour) — new bytes but no call.
	f, err = os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(strings.Replace(newEvent, "17:07:30", "17:10:30", 1)); err != nil {
		t.Fatal(err)
	}
	f.Close()
	now4 := mustTime(t, "2026-09-24T17:10:31+05:30")
	if n, err := o.Tick(context.Background(), now4); err != nil {
		t.Fatalf("tick 4: %v", err)
	} else if n != 0 {
		t.Fatalf("tick 4 appended = %d, want 0 (budget cap)", n)
	}
	if critic.calls != 2 {
		t.Fatalf("critic calls after tick 4 = %d, want 2 (budget cap held)", critic.calls)
	}
}

func TestEndedSessionNotSampled(t *testing.T) {
	cfg, critic := newTestEnv(t)
	o := NewObserver(cfg, critic, os.Stderr)
	o.ProcessLister = func(time.Time) ([]watcher.ProcessInfo, error) { return nil, nil }
	now := mustTime(t, "2026-09-24T17:04:00+05:30")
	n, err := o.Tick(context.Background(), now)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if n != 0 || critic.calls != 0 {
		t.Fatalf("ended session: appended=%d critic calls=%d, want 0/0", n, critic.calls)
	}
	if len(readJSONL(t, cfg.QueuePath)) != 2 {
		t.Errorf("queue mutated for ended session")
	}
}

func TestBudgetWindowRolls(t *testing.T) {
	cfg, critic := newTestEnv(t)
	cfg.MaxCallsPerHour = 1
	o := NewObserver(cfg, critic, os.Stderr)
	o.ProcessLister = matchingProcessLister
	now := mustTime(t, "2026-09-24T17:04:00+05:30")
	if _, err := o.Tick(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	// Grow the log past the first sample, then two more ticks: the second
	// is beyond the 1/hour cap within the hour window, but after the hour
	// rolls over a call is allowed again.
	logPath := filepath.Join(cfg.LogDir, fixtureLogName)
	appendEvent := func(ts string) {
		line := fmt.Sprintf(`{"kind":"MessageAppended","time":%q,"trace_id":"","span_id":"s","message_id":"m","role":"user","content_types":["tool_result"],"token_estimate":400}`+"\n", ts)
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString(line); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	appendEvent("2026-09-24T17:07:30+05:30")
	if n, err := o.Tick(context.Background(), mustTime(t, "2026-09-24T17:07:31+05:30")); err != nil || n != 0 {
		t.Fatalf("within-hour tick: n=%d err=%v, want 0 (capped)", n, err)
	}
	if critic.calls != 1 {
		t.Fatalf("critic calls = %d, want 1 (cap held)", critic.calls)
	}
	appendEvent("2026-09-24T18:05:00+05:30")
	if n, err := o.Tick(context.Background(), mustTime(t, "2026-09-24T18:05:01+05:30")); err != nil || n != 0 {
		t.Fatalf("after-hour tick: n=%d err=%v, want 0 (dedup still holds)", n, err)
	}
	if critic.calls != 2 {
		t.Fatalf("critic calls after hour rollover = %d, want 2", critic.calls)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func systemText(sp model.SystemPrompt) string {
	var b strings.Builder
	for _, blk := range sp.Blocks {
		b.WriteString(blk.Text)
	}
	return b.String()
}

func messageText(msgs []model.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		for _, part := range m.Content {
			if tp, ok := part.(model.TextPart); ok {
				b.WriteString(tp.Text)
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}
