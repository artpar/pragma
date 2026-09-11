package observe

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/artpar/pragma/internal/model"
)

const EventLogMaxLineBytes = 64 * 1024 * 1024

type EventLogLine struct {
	Number int
	Raw    []byte
}

// ReplayEngine loads and provides access to recorded session data.
type ReplayEngine struct {
	events       []Event
	toolOutputs  map[string]RecordedToolOutput
	apiResponses map[int]model.Response
	apiRequests  map[int]APIRequestStarted
}

// LoadReplay loads a replay from a directory containing events.jsonl,
// tool-outputs/, and api-responses/, or from a single recorded JSONL file.
func LoadReplay(path string) (*ReplayEngine, error) {
	re := &ReplayEngine{
		toolOutputs:  make(map[string]RecordedToolOutput),
		apiResponses: make(map[int]model.Response),
		apiRequests:  make(map[int]APIRequestStarted),
	}

	artifactDir := path
	// Let ScanEventLog resolve the events file: events.jsonl when present,
	// otherwise a lone *.jsonl recording file in the directory.
	eventsPath := path
	if info, err := os.Stat(path); err != nil {
		return nil, err
	} else if !info.IsDir() {
		eventsPath = path
		artifactDir = filepath.Dir(path)
	}

	// Load events
	if err := re.loadEvents(eventsPath); err != nil {
		return nil, fmt.Errorf("load events: %w", err)
	}
	re.indexAPIRequests()
	re.indexAPIResponses()

	// Load tool outputs (optional)
	toolDir := filepath.Join(artifactDir, "tool-outputs")
	if entries, err := os.ReadDir(toolDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(toolDir, entry.Name()))
			if err != nil {
				continue
			}
			var out RecordedToolOutput
			if err := json.Unmarshal(data, &out); err != nil {
				continue
			}
			re.toolOutputs[out.ToolCallID] = out
		}
	}

	// Load API responses (optional)
	apiDir := filepath.Join(artifactDir, "api-responses")
	if entries, err := os.ReadDir(apiDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(apiDir, entry.Name()))
			if err != nil {
				continue
			}
			var turn int
			var resp model.Response
			name := entry.Name()
			if _, err := fmt.Sscanf(name, "turn-%d.json", &turn); err != nil {
				continue
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				continue
			}
			re.apiResponses[turn] = resp
		}
	}

	return re, nil
}

// LoadEvents reads events from a JSONL file. If path is a directory,
// it looks for events.jsonl inside it, or for a single *.jsonl recording
// file when events.jsonl is absent. Events whose kinds are not registered
// with the current build are skipped with a stderr warning so recordings
// from older harness revisions remain loadable.
func LoadEvents(path string) ([]Event, error) {
	var events []Event
	skipped := 0
	var firstUnknown string
	err := ScanEventLog(path, func(line EventLogLine) error {
		event, err := UnmarshalEvent(line.Raw)
		if err != nil {
			var unknown UnknownEventKindError
			if errors.As(err, &unknown) {
				skipped++
				if firstUnknown == "" {
					firstUnknown = unknown.Kind
				}
				return nil
			}
			return fmt.Errorf("line %d: unmarshal event: %w", line.Number, err)
		}
		events = append(events, event)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if skipped > 0 {
		fmt.Fprintf(os.Stderr, "warning: skipped %d event(s) with unregistered kinds (first: %q)\n", skipped, firstUnknown)
	}
	return events, nil
}

func ScanEventLog(path string, handle func(EventLogLine) error) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		path, err = resolveEventLogPath(path)
		if err != nil {
			return err
		}
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), EventLogMaxLineBytes)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		if err := handle(EventLogLine{Number: lineNumber, Raw: scanner.Bytes()}); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan event log: %w", err)
	}
	return nil
}

// resolveEventLogPath resolves the event file for a directory. The canonical
// layout is events.jsonl; the recorder also writes <timestamp>.jsonl files
// under ~/.pragma/recordings/<session-id>/, so a directory holding exactly
// one such file resolves to it. Multiple candidates are reported rather
// than guessed.
func resolveEventLogPath(dir string) (string, error) {
	canonical := filepath.Join(dir, "events.jsonl")
	if _, err := os.Stat(canonical); err == nil {
		return canonical, nil
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return "", err
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no events.jsonl or *.jsonl event file in %s", dir)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("multiple event files in %s: %v", dir, matches)
	}
}

func (re *ReplayEngine) loadEvents(path string) error {
	events, err := LoadEvents(path)
	if err != nil {
		return err
	}
	re.events = events
	return nil
}

// Events returns all loaded events.
func (re *ReplayEngine) Events() []Event {
	return re.events
}

// ToolOutput returns the recorded output for a tool call.
func (re *ReplayEngine) ToolOutput(toolCallID string) (string, bool) {
	out, ok := re.toolOutputs[toolCallID]
	if !ok {
		return "", false
	}
	return out.Output, true
}

// APIResponse returns the recorded API response for a turn number.
func (re *ReplayEngine) APIResponse(turn int) (model.Response, bool) {
	resp, ok := re.apiResponses[turn]
	return resp, ok
}

// APIRequest returns the recorded provider request payload for a turn number.
func (re *ReplayEngine) APIRequest(turn int) (APIRequestStarted, bool) {
	req, ok := re.apiRequests[turn]
	return req, ok
}

func (re *ReplayEngine) indexAPIRequests() {
	turn := 0
	for _, ev := range re.events {
		if req, ok := ev.(APIRequestStarted); ok {
			turn++
			re.apiRequests[turn] = req
		}
	}
}

func (re *ReplayEngine) indexAPIResponses() {
	turn := 0
	for _, ev := range re.events {
		completed, ok := ev.(APIRequestCompleted)
		if !ok {
			continue
		}
		turn++
		if len(completed.Content) == 0 {
			continue
		}
		parts, err := model.UnmarshalContentParts(completed.Content)
		if err != nil {
			continue
		}
		re.apiResponses[turn] = model.Response{
			Model:      completed.Model,
			Content:    parts,
			StopReason: completed.StopReason,
			Usage:      completed.Usage,
		}
	}
}
