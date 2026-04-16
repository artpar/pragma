package background

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/artpar/gogent/internal/config"
	"github.com/artpar/gogent/internal/observe"
)

// pidFilePattern guards against reading non-PID files from the registry directory.
// Strict regex prevents accidental data loss (TS bug #34210).
var pidFilePattern = regexp.MustCompile(`^\d+\.json$`)

// Registry manages PID files for active background sessions.
// Each running background session writes a {pid}.json file.
type Registry struct {
	dir string
}

// NewRegistry creates a Registry at ~/.gogent/active-sessions/.
func NewRegistry() (*Registry, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	gogentHome, err := config.GogentHome()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"resolve gogent home: %w\", err)")
		return nil, fmt.Errorf("resolve gogent home: %w", err)
	}
	dir := filepath.Join(gogentHome, "active-sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"create active-sessions directory: %w\", err)")
		return nil, fmt.Errorf("create active-sessions directory: %w", err)
	}
	observe.GlobalTrace("return: &Registry{dir: dir}, nil")
	return &Registry{dir: dir}, nil
}

// Register writes a PID file for the given process info.
// Uses atomic write (temp file + rename) to prevent partial reads.
func (r *Registry) Register(info ProcessInfo) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	info.UpdatedAt = time.Now()
	data, err := json.Marshal(info)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"marshal process info: %w\", err)")
		return fmt.Errorf("marshal process info: %w", err)
	}

	path := r.pidPath(info.PID)
	tmpPath := path + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"write temp PID file: %w\", err)")
		return fmt.Errorf("write temp PID file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		observe.GlobalTrace("if: err != nil")
		os.Remove(tmpPath)
		observe.GlobalTrace("return: fmt.Errorf(\"rename PID file: %w\", err)")
		return fmt.Errorf("rename PID file: %w", err)
	}
	observe.GlobalTrace("return: nil")
	return nil
}

// Unregister removes a PID file. Ignores ENOENT (already cleaned up).
func (r *Registry) Unregister(pid int) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	err := os.Remove(r.pidPath(pid))
	if err != nil && !os.IsNotExist(err) {
		observe.GlobalTrace("if: err != nil && !os.IsNotExist(err)")
		observe.GlobalTrace("return: fmt.Errorf(\"remove PID file for %d: %w\", pid, err)")
		return fmt.Errorf("remove PID file for %d: %w", pid, err)
	}
	observe.GlobalTrace("return: nil")
	return nil
}

// UpdateStatus updates the status and timestamp in a PID file.
// Fire-and-forget: errors are silently ignored (TS pattern).
func (r *Registry) UpdateStatus(pid int, status Status) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	info, err := r.Get(pid)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		return
	}
	info.Status = status
	info.UpdatedAt = time.Now()
	_ = r.Register(info)
}

// List returns all active background sessions.
// Validates each PID is alive, removes stale entries, sorts by StartedAt descending.
func (r *Registry) List() ([]ProcessInfo, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		if os.IsNotExist(err) {
			observe.GlobalTrace("if: os.IsNotExist(err)")
			observe.GlobalTrace("return: nil, nil")
			return nil, nil
		}
		observe.GlobalTrace("return: nil, fmt.Errorf(\"read active-sessions directory: %w\", err)")
		return nil, fmt.Errorf("read active-sessions directory: %w", err)
	}

	var active []ProcessInfo
	for _, entry := range entries {
		observe.GlobalTrace("range entries")
		if entry.IsDir() || !pidFilePattern.MatchString(entry.Name()) {
			observe.GlobalTrace("if: entry.IsDir() || !pidFilePattern.MatchString(entry.Name())")
			continue
		}

		data, err := os.ReadFile(filepath.Join(r.dir, entry.Name()))
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			continue
		}

		var info ProcessInfo
		if err := json.Unmarshal(data, &info); err != nil {
			observe.GlobalTrace("if: err != nil")
			continue
		}

		if !isProcessAlive(info.PID) {
			observe.GlobalTrace("if: !isProcessAlive(info.PID)")

			os.Remove(filepath.Join(r.dir, entry.Name()))
			continue
		}

		active = append(active, info)
	}

	sort.Slice(active, func(i, j int) bool {
		return active[i].StartedAt.After(active[j].StartedAt)
	})
	observe.GlobalTrace("return: active, nil")

	return active, nil
}

// Get reads a single PID file.
func (r *Registry) Get(pid int) (ProcessInfo, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	data, err := os.ReadFile(r.pidPath(pid))
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: ProcessInfo{}, fmt.Errorf(\"read PID file for %d: %w\", pid, err)")
		return ProcessInfo{}, fmt.Errorf("read PID file for %d: %w", pid, err)
	}
	var info ProcessInfo
	if err := json.Unmarshal(data, &info); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: ProcessInfo{}, fmt.Errorf(\"parse PID file for %d: %w\", pid, err)")
		return ProcessInfo{}, fmt.Errorf("parse PID file for %d: %w", pid, err)
	}
	observe.GlobalTrace("return: info, nil")
	return info, nil
}

func (r *Registry) pidPath(pid int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: filepath.Join(r.dir, strconv.Itoa(pid)+\".json\")")
	return filepath.Join(r.dir, strconv.Itoa(pid)+".json")
}
