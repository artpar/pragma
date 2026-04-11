package cron

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Store persists durable cron jobs to disk as JSON.
type Store struct {
	path string
	mu   sync.Mutex
}

type durableData struct {
	Tasks []Job `json:"tasks"`
}

// NewStore creates a Store that reads/writes to the given path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Load reads durable jobs from disk. Returns nil slice and nil error
// if the file does not exist yet.
func (s *Store) Load() ([]Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("cron store load: %w", err)
	}

	var d durableData
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("cron store unmarshal: %w", err)
	}
	return d.Tasks, nil
}

// Save writes jobs to disk atomically (write-tmp then rename).
func (s *Store) Save(jobs []Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	d := durableData{Tasks: jobs}
	if d.Tasks == nil {
		d.Tasks = []Job{}
	}

	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return fmt.Errorf("cron store marshal: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("cron store mkdir: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("cron store write tmp: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("cron store rename: %w", err)
	}
	return nil
}
