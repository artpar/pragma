package metaobserve

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/watcher"
)

// Config carries the meta-observer's knobs. Zero values disable optional
// parts (QueuePath empty = durable findings only).
type Config struct {
	LogDir       string
	SessionsDir  string
	QueuePath    string // self-improvement queue (append meta-finding entries)
	FindingsPath string // durable findings record (always written)

	Interval     time.Duration // sampling cadence (operator: ~3 minutes)
	FirstWindow  time.Duration // look-back for a session's first sample
	ActiveWithin time.Duration // log freshness window

	MaxLogLines    int
	MaxSessionMsgs int
	MaxDigestChars int

	MaxCallsPerHour int // mechanical budget bound on critic calls
	MaxQueueItems   int // bound on queue appends per daemon run
	CriticTimeout   time.Duration

	CriticProvider string // provenance only
	CriticModel    string
}

// Provenance records where and when a finding was observed.
type Provenance struct {
	SessionLog     string    `json:"session_log"`
	SessionID      string    `json:"session_id,omitempty"`
	WindowFrom     time.Time `json:"window_from"`
	WindowTo       time.Time `json:"window_to"`
	SampledAt      time.Time `json:"sampled_at"`
	CriticProvider string    `json:"critic_provider"`
	CriticModel    string    `json:"critic_model"`
	DigestSHA256   string    `json:"digest_sha256"`
}

// FindingPayload is the queue-entry payload for one finding.
type FindingPayload struct {
	Finding    Finding    `json:"finding"`
	Provenance Provenance `json:"provenance"`
}

// FindingRecord is the JSONL entry appended to the findings file and the
// self-improvement queue.
type FindingRecord struct {
	ID      string         `json:"id"`
	Title   string         `json:"title"`
	Kind    string         `json:"kind"`   // meta-finding
	Status  string         `json:"status"` // open
	AddedAt time.Time      `json:"added_at"`
	Payload FindingPayload `json:"payload"`
}

// logTrack is the per-session sampling state.
type logTrack struct {
	size      int64
	lastAt    time.Time
	sessionID string
	seen      map[string]bool
}

// Observer periodically samples live sessions and runs the critic.
type Observer struct {
	cfg    Config
	critic Critic
	logw   io.Writer

	mu         sync.Mutex
	track      map[string]*logTrack
	calls      []time.Time
	queueItems int

	// ProcessLister is the live-process lookup; overridable for tests.
	// The default lists real pragma processes via ps.
	ProcessLister func(now time.Time) ([]watcher.ProcessInfo, error)
}

// NewObserver builds an observer; logw receives its operational log lines.
func NewObserver(cfg Config, critic Critic, logw io.Writer) *Observer {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if cfg.MaxCallsPerHour <= 0 {
		observe.GlobalTrace("if: cfg.MaxCallsPerHour <= 0")
		cfg.MaxCallsPerHour = 60
	}
	if cfg.MaxQueueItems <= 0 {
		observe.GlobalTrace("if: cfg.MaxQueueItems <= 0")
		cfg.MaxQueueItems = 200
	}
	if cfg.CriticTimeout <= 0 {
		observe.GlobalTrace("if: cfg.CriticTimeout <= 0")
		cfg.CriticTimeout = 90 * time.Second
	}
	observe.GlobalTrace("return: &Observer{ cfg: cfg, critic: critic, logw: logw, track: map[string]*logTrack{...")
	return &Observer{
		cfg:           cfg,
		critic:        critic,
		logw:          logw,
		track:         map[string]*logTrack{},
		ProcessLister: watcher.FindPragmaProcesses,
	}
}

func (o *Observer) logf(format string, args ...any) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if o.logw == nil {
		observe.GlobalTrace("if: o.logw == nil")
		return
	}
	fmt.Fprintf(o.logw, "%s meta-observe: %s\n",
		time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
}

// pruneCallsLocked drops critic-call timestamps older than one hour.
func (o *Observer) pruneCallsLocked(now time.Time) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	kept := o.calls[:0]
	for _, t := range o.calls {
		observe.GlobalTrace("range o.calls")
		if now.Sub(t) < time.Hour {
			observe.GlobalTrace("if: now.Sub(t) < time.Hour")
			kept = append(kept, t)
		}
	}
	o.calls = kept
}

// queueTitlesLocked reads the tail of the queue file for dedup context.
func (o *Observer) queueTitlesLocked() []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if o.cfg.QueuePath == "" {
		observe.GlobalTrace("if: o.cfg.QueuePath == \"\"")
		observe.GlobalTrace("return: nil")
		return nil
	}
	lines, err := readTail(o.cfg.QueuePath, 256*1024)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	if len(lines) > 200 {
		observe.GlobalTrace("if: len(lines) > 200")
		lines = lines[len(lines)-200:]
	}
	titles := make([]string, 0, len(lines))
	for _, l := range lines {
		observe.GlobalTrace("range lines")
		var rec struct {
			Title string `json:"title"`
		}
		if json.Unmarshal([]byte(l), &rec) == nil && rec.Title != "" {
			observe.GlobalTrace("if: json.Unmarshal([]byte(l), &rec) == nil && rec.Title != \"\"")
			titles = append(titles, rec.Title)
		}
	}
	if len(titles) > 40 {
		observe.GlobalTrace("if: len(titles) > 40")
		titles = titles[len(titles)-40:]
	}
	observe.GlobalTrace("return: titles")
	return titles
}

// fingerprint identifies a finding per session for dedup.
func fingerprint(logBase string, f Finding) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	norm := strings.Join(strings.Fields(strings.ToLower(f.Finding)), " ")
	if len(norm) > 160 {
		observe.GlobalTrace("if: len(norm) > 160")
		norm = norm[:160]
	}
	h := sha256.Sum256([]byte(logBase + "\x00" + f.Kind + "\x00" + norm))
	observe.GlobalTrace("return: hex.EncodeToString(h[:])")
	return hex.EncodeToString(h[:])
}

// appendJSONL appends one JSON record as a line to path.
func appendJSONL(path string, rec any) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	b, err := json.Marshal(rec)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	observe.GlobalTrace("return: err")
	return err
}

// Tick performs one sampling round over every live session and returns
// the number of findings appended to the queue.
func (o *Observer) Tick(ctx context.Context, now time.Time) (int, error) {
	observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "enter")
	defer observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "exit")
	o.mu.Lock()
	defer o.mu.Unlock()

	logs, err := watcher.ActiveLogFiles(o.cfg.LogDir, o.cfg.ActiveWithin, now)
	if err != nil {
		observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "if: err != nil")
		o.logf("list logs: %v", err)
		observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "return: 0, err")
		return 0, err
	}
	procs := []watcher.ProcessInfo{}
	if o.ProcessLister != nil {
		observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "if: o.ProcessLister != nil")
		if p, err := o.ProcessLister(now); err != nil {
			observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "if: err != nil")
			o.logf("process list: %v (live check disabled this tick)", err)
		} else {
			observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "else: err != nil")
			procs = p
		}
	}

	appended := 0
	for _, logPath := range logs {
		observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "range logs")
		base := filepath.Base(logPath)
		logStart, ok := watcher.ParseLogName(base)
		if !ok {
			observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "if: !ok")
			continue
		}
		if watcher.MatchProcessToLog(procs, logStart, 3*time.Minute) == nil {
			observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "if: watcher.MatchProcessToLog(procs, logStart, 3*time.Minute) == nil")
			o.logf("skip %s: no live pragma process", base)
			continue
		}
		tr := o.track[logPath]
		if tr == nil {
			observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "if: tr == nil")
			tr = &logTrack{seen: map[string]bool{}}
			o.track[logPath] = tr
		}
		fi, err := os.Stat(logPath)
		if err != nil {
			observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "if: err != nil")
			o.logf("stat %s: %v", base, err)
			continue
		}
		if fi.Size() == tr.size {
			observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "if: fi.Size() == tr.size")
			continue // no new bytes since the last sample: no call, no spend
		}
		from := tr.lastAt
		if from.IsZero() {
			observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "if: from.IsZero()")
			from = now.Add(-o.cfg.FirstWindow)
		}
		sample, err := CollectSample(logPath, o.cfg.SessionsDir, from, now, o.cfg.MaxLogLines, o.cfg.MaxSessionMsgs)
		if err != nil {
			observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "if: err != nil")
			o.logf("sample %s: %v", base, err)
			continue
		}
		tr.size = fi.Size()
		tr.lastAt = now
		if sample.SessionID != "" {
			observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "if: sample.SessionID != \"\"")
			tr.sessionID = sample.SessionID
		} else {
			observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "else: sample.SessionID != \"\"")
			sample.SessionID = tr.sessionID
		}
		if len(sample.LogLines) == 0 {
			observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "if: len(sample.LogLines) == 0")
			continue
		}
		o.pruneCallsLocked(now)
		if len(o.calls) >= o.cfg.MaxCallsPerHour {
			observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "if: len(o.calls) >= o.cfg.MaxCallsPerHour")
			o.logf("skip %s: budget cap reached (%d critic calls in the last hour)", base, len(o.calls))
			continue
		}
		digest := sample.Digest(o.cfg.MaxDigestChars)
		titles := o.queueTitlesLocked()
		cctx, cancel := context.WithTimeout(ctx, o.cfg.CriticTimeout)
		o.calls = append(o.calls, now) // the attempt counts against the budget
		findings, err := o.critic.Judge(cctx, digest, titles)
		cancel()
		if err != nil {
			observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "if: err != nil")
			o.logf("critic %s: %v", base, err)
			continue
		}
		dh := sha256.Sum256([]byte(digest))
		for _, f := range findings {
			observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "range findings")
			fp := fingerprint(base, f)
			if tr.seen[fp] {
				observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "if: tr.seen[fp]")
				continue // already reported for this session
			}
			tr.seen[fp] = true
			rec := FindingRecord{
				ID:      "META-OBS-" + now.Format("20060102-150405") + "-" + fp[:8],
				Title:   "critic finding (" + f.Kind + "): " + truncateRunes(f.Finding, 90),
				Kind:    "meta-finding",
				Status:  "open",
				AddedAt: now,
				Payload: FindingPayload{
					Finding: f,
					Provenance: Provenance{
						SessionLog:     base,
						SessionID:      sample.SessionID,
						WindowFrom:     sample.WindowFrom,
						WindowTo:       sample.WindowTo,
						SampledAt:      now,
						CriticProvider: o.cfg.CriticProvider,
						CriticModel:    o.cfg.CriticModel,
						DigestSHA256:   hex.EncodeToString(dh[:]),
					},
				},
			}
			if err := appendJSONL(o.cfg.FindingsPath, rec); err != nil {
				observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "if: err != nil")
				o.logf("findings append: %v", err)
				continue
			}
			if o.cfg.QueuePath != "" && o.queueItems < o.cfg.MaxQueueItems {
				observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "if: o.cfg.QueuePath != \"\" && o.queueItems < o.cfg.MaxQueueItems")
				if err := appendJSONL(o.cfg.QueuePath, rec); err != nil {
					observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "if: err != nil")
					o.logf("queue append: %v", err)
					continue
				}
				o.queueItems++
			}
			appended++
		}
	}
	observe.TraceCtx(ctx, "metaobserve", "Observer.Tick", "return: appended, nil")
	return appended, nil
}

// Run samples forever on the configured cadence until ctx is cancelled.
func (o *Observer) Run(ctx context.Context) {
	observe.TraceCtx(ctx, "metaobserve", "Observer.Run", "enter")
	defer observe.TraceCtx(ctx, "metaobserve", "Observer.Run", "exit")
	interval := o.cfg.Interval
	if interval <= 0 {
		observe.TraceCtx(ctx, "metaobserve", "Observer.Run", "if: interval <= 0")
		interval = 3 * time.Minute
	}
	for {
		observe.TraceCtx(ctx, "metaobserve", "Observer.Run", "for: true")
		now := time.Now()
		if n, err := o.Tick(ctx, now); err == nil {
			observe.TraceCtx(ctx, "metaobserve", "Observer.Run", "if: err == nil")
			o.logf("tick complete: %d findings appended", n)
		}
		select {
		case <-ctx.Done():
			observe.TraceCtx(ctx, "metaobserve", "Observer.Run", "select: <-ctx.Done()")
			return
		case <-time.After(interval):
			observe.TraceCtx(ctx, "metaobserve", "Observer.Run", "select: <-time.After(interval)")
		}
	}
}
