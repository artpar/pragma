package watcher

import (
	"encoding/json"
	"fmt"
	"github.com/artpar/pragma/internal/observe"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// PostMortem is the durable end-of-session record the observer writes
// when a session's pragma process disappears. It captures the final
// observable state so a future session can learn what happened without
// having been there: growth trajectory, failure modes, loop evidence.
type PostMortem struct {
	Session    string `json:"session"` // event log file name
	Model      string `json:"model,omitempty"`
	FirstEvent string `json:"first_event_at,omitempty"`
	LastEvent  string `json:"last_event_at,omitempty"`
	ObservedAt string `json:"observed_at"`

	Turns        int `json:"turns"`
	APICalls     int `json:"api_calls"`
	ToolCalls    int `json:"tool_calls"`
	ToolErrors   int `json:"tool_errors"`
	Retries      int `json:"retries"`
	Failures     int `json:"failures"`
	OutputTokens int `json:"output_tokens"`
	MaxTokenHits int `json:"max_token_hits"`

	// Context trajectory: how big the conversation grew.
	ContextFill int `json:"context_fill"`
	ContextPeak int `json:"context_peak"`

	// End cause evidence.
	LastStopReason   string            `json:"last_stop_reason,omitempty"`
	StopReasons      map[string]int    `json:"stop_reasons,omitempty"`
	LastError        string            `json:"last_error,omitempty"`
	LoopSuspected    bool              `json:"loop_suspected"`
	StampishUserMsgs int               `json:"stampish_user_msgs"`
	TopTools         []ToolCount       `json:"top_tools,omitempty"`
	MCPServers       map[string]string `json:"mcp_servers,omitempty"`
	FinalStatus      string            `json:"final_status"`
	AlertKinds       map[string]int    `json:"alert_kinds,omitempty"`
}

// ToolCount is a tool-name usage count, used in post-mortems.
type ToolCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// BuildPostMortem derives the end-of-session record from a session state.
// finalStatus is the session's status at the moment its process
// disappeared (the observer computes it just before writing).
func BuildPostMortem(s *SessionState, observedAt time.Time, finalStatus string) PostMortem {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	pm := PostMortem{
		Session:          s.Name,
		Model:            s.Model,
		FirstEvent:       formatTime(s.FirstEventAt),
		LastEvent:        formatTime(s.LastEventAt),
		ObservedAt:       observedAt.Format("2006-01-02 15:04:05"),
		Turns:            s.Turns,
		APICalls:         s.APICalls,
		ToolCalls:        s.ToolCalls,
		ToolErrors:       s.ToolErrors,
		Retries:          s.Retries,
		Failures:         s.Failures,
		OutputTokens:     s.OutputTokens,
		MaxTokenHits:     s.MaxHits,
		ContextFill:      s.ContextFill,
		ContextPeak:      s.ContextPeak,
		LastStopReason:   s.LastStopReason,
		StopReasons:      s.StopReasonCounts,
		LastError:        s.LastError,
		LoopSuspected:    s.loopAlerted,
		StampishUserMsgs: s.StampishUserMsgs,
		MCPServers:       s.Servers,
		FinalStatus:      finalStatus,
		AlertKinds:       map[string]int{},
	}
	for _, a := range s.Alerts {
		observe.GlobalTrace("range s.Alerts")
		pm.AlertKinds[a.Kind]++
	}
	pm.TopTools = topTools(s.ToolCallCounts, 5)
	observe.GlobalTrace("return: pm")
	return pm
}

// WritePostMortem writes the post-mortem into dir as
// <session>.postmortem.json (atomic tmp+rename). The file name derives
// from the event log name so writes are idempotent per session.
func WritePostMortem(dir string, pm PostMortem) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"create postmortem dir: %w\", err)")
		return "", fmt.Errorf("create postmortem dir: %w", err)
	}
	name := strings.TrimSuffix(pm.Session, ".jsonl")
	path := filepath.Join(dir, name+".postmortem.json")
	b, err := json.MarshalIndent(pm, "", "  ")
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"marshal postmortem: %w\", err)")
		return "", fmt.Errorf("marshal postmortem: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"write postmortem: %w\", err)")
		return "", fmt.Errorf("write postmortem: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"rename postmortem: %w\", err)")
		return "", fmt.Errorf("rename postmortem: %w", err)
	}
	observe.GlobalTrace("return: path, nil")
	return path, nil
}

// AppendAlerts appends alerts as JSONL lines to the alerts log.
func AppendAlerts(path string, alerts []Alert) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(alerts) == 0 {
		observe.GlobalTrace("if: len(alerts) == 0")
		observe.GlobalTrace("return: nil")
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, a := range alerts {
		observe.GlobalTrace("range alerts")
		if err := enc.Encode(a); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: err")
			return err
		}
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func topTools(counts map[string]int, n int) []ToolCount {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(counts) == 0 {
		observe.GlobalTrace("if: len(counts) == 0")
		observe.GlobalTrace("return: nil")
		return nil
	}
	all := make([]ToolCount, 0, len(counts))
	for name, count := range counts {
		observe.GlobalTrace("range counts")
		all = append(all, ToolCount{Name: name, Count: count})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Count != all[j].Count {
			return all[i].Count > all[j].Count
		}
		return all[i].Name < all[j].Name
	})
	if len(all) > n {
		observe.GlobalTrace("if: len(all) > n")
		all = all[:n]
	}
	observe.GlobalTrace("return: all")
	return all
}

func formatTime(t time.Time) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if t.IsZero() {
		observe.GlobalTrace("if: t.IsZero()")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	observe.GlobalTrace("return: t.Format(\"2006-01-02 15:04:05\")")
	return t.Format("2006-01-02 15:04:05")
}
