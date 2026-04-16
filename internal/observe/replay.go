package observe

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/artpar/pragma/internal/model"
)

// ReplayEngine loads and provides access to recorded session data.
type ReplayEngine struct {
	events       []Event
	toolOutputs  map[string]RecordedToolOutput
	apiResponses map[int]model.Response
}

// LoadReplay loads a replay from a directory containing events.jsonl,
// tool-outputs/, and api-responses/.
func LoadReplay(dir string) (*ReplayEngine, error) {
	re := &ReplayEngine{
		toolOutputs:  make(map[string]RecordedToolOutput),
		apiResponses: make(map[int]model.Response),
	}

	// Load events
	eventsPath := filepath.Join(dir, "events.jsonl")
	if err := re.loadEvents(eventsPath); err != nil {
		return nil, fmt.Errorf("load events: %w", err)
	}

	// Load tool outputs (optional)
	toolDir := filepath.Join(dir, "tool-outputs")
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
	apiDir := filepath.Join(dir, "api-responses")
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
// it looks for events.jsonl inside it.
func LoadEvents(path string) ([]Event, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		path = filepath.Join(path, "events.jsonl")
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var events []Event
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB line buffer
	for scanner.Scan() {
		event, err := UnmarshalEvent(scanner.Bytes())
		if err != nil {
			return nil, fmt.Errorf("unmarshal event: %w", err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
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
