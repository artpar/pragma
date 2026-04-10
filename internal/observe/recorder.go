package observe

import (
	"encoding/json"
	"errors"
	"log/slog"
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
	file      *os.File
	encoder   *json.Encoder
	mu        sync.Mutex
	encodeErr error
	closed    bool
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
	if r.encodeErr != nil {
		return
	}
	if err := r.encoder.Encode(event); err != nil {
		r.encodeErr = err
		slog.Error("recorder encode failed", "error", err)
	}
}

func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return r.encodeErr
	}
	r.closed = true
	return errors.Join(r.encodeErr, r.file.Close())
}
