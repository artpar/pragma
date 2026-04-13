package background

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"syscall"
	"time"

	"github.com/artpar/gogent/internal/config"
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
	gogentHome, err := config.GogentHome()
	if err != nil {
		return nil, fmt.Errorf("resolve gogent home: %w", err)
	}
	dir := filepath.Join(gogentHome, "active-sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create active-sessions directory: %w", err)
	}
	return &Registry{dir: dir}, nil
}

// Register writes a PID file for the given process info.
// Uses atomic write (temp file + rename) to prevent partial reads.
func (r *Registry) Register(info ProcessInfo) error {
	info.UpdatedAt = time.Now()
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshal process info: %w", err)
	}

	path := r.pidPath(info.PID)
	tmpPath := path + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write temp PID file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename PID file: %w", err)
	}
	return nil
}

// Unregister removes a PID file. Ignores ENOENT (already cleaned up).
func (r *Registry) Unregister(pid int) error {
	err := os.Remove(r.pidPath(pid))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove PID file for %d: %w", pid, err)
	}
	return nil
}

// UpdateStatus updates the status and timestamp in a PID file.
// Fire-and-forget: errors are silently ignored (TS pattern).
func (r *Registry) UpdateStatus(pid int, status Status) {
	info, err := r.Get(pid)
	if err != nil {
		return
	}
	info.Status = status
	info.UpdatedAt = time.Now()
	_ = r.Register(info)
}

// List returns all active background sessions.
// Validates each PID is alive, removes stale entries, sorts by StartedAt descending.
func (r *Registry) List() ([]ProcessInfo, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read active-sessions directory: %w", err)
	}

	var active []ProcessInfo
	for _, entry := range entries {
		if entry.IsDir() || !pidFilePattern.MatchString(entry.Name()) {
			continue
		}

		data, err := os.ReadFile(filepath.Join(r.dir, entry.Name()))
		if err != nil {
			continue
		}

		var info ProcessInfo
		if err := json.Unmarshal(data, &info); err != nil {
			continue
		}

		if !isProcessAlive(info.PID) {
			// Stale PID file — process is dead, clean up
			os.Remove(filepath.Join(r.dir, entry.Name()))
			continue
		}

		active = append(active, info)
	}

	sort.Slice(active, func(i, j int) bool {
		return active[i].StartedAt.After(active[j].StartedAt)
	})

	return active, nil
}

// Get reads a single PID file.
func (r *Registry) Get(pid int) (ProcessInfo, error) {
	data, err := os.ReadFile(r.pidPath(pid))
	if err != nil {
		return ProcessInfo{}, fmt.Errorf("read PID file for %d: %w", pid, err)
	}
	var info ProcessInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return ProcessInfo{}, fmt.Errorf("parse PID file for %d: %w", pid, err)
	}
	return info, nil
}

// Kill terminates a background session and its entire process group.
// Sends SIGTERM to the process group, waits up to 5 seconds, then SIGKILL.
// Addresses orphan process accumulation (GitHub #32964, #15945, #26658).
func (r *Registry) Kill(pid int) error {
	info, err := r.Get(pid)
	if err != nil {
		return err
	}

	pgid := info.PGID
	if pgid == 0 {
		pgid = pid
	}

	// Send SIGTERM to entire process group (negative PGID)
	if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil {
		// Process might already be dead
		if err != syscall.ESRCH {
			return fmt.Errorf("SIGTERM process group %d: %w", pgid, err)
		}
		_ = r.Unregister(pid)
		return nil
	}

	// Wait up to 5 seconds for graceful shutdown
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !isProcessAlive(pid) {
			_ = r.Unregister(pid)
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Force kill if still alive
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
	_ = r.Unregister(pid)
	return nil
}

func (r *Registry) pidPath(pid int) string {
	return filepath.Join(r.dir, strconv.Itoa(pid)+".json")
}

// isProcessAlive checks if a process with the given PID exists.
func isProcessAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}
