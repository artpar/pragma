package observe

import (
	"encoding/json"
	"os"
	"sync"
)

// RecordedToolOutput captures a tool's output for replay.
type RecordedToolOutput struct {
	ToolCallID string `json:"tool_call_id"`
	Output     string `json:"output"`
	IsError    bool   `json:"is_error"`
}

// Recorder is a Subscriber that writes events as JSONL to a file.
type Recorder struct {
	file    *os.File
	encoder *json.Encoder
	mu      sync.Mutex
}

// NewRecorder creates a Recorder writing to the given path.
func NewRecorder(path string) (*Recorder, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &Recorder{
		file:    f,
		encoder: json.NewEncoder(f),
	}, nil
}

func (r *Recorder) HandleEvent(event Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_ = r.encoder.Encode(event)
}

func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.file.Close()
}
