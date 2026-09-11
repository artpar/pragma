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

	"github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/observe"
)

// pidFilePattern guards against reading non-PID files from the registry directory.
// Strict regex prevents accidental data loss (TS bug #34210).
var pidFilePattern = regexp.MustCompile(`^\d+\.json$`)

// Registry manages PID files for active background processes.
// Each running background process writes a {pid}.json file.
type Registry struct {
	dir string
}

// NewRegistry creates a Registry at ~/.pragma/active-sessions/.
func NewRegistry() (*Registry, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	pragmaHome, err := config.PragmaHome()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"resolve pragma home: %w\", err)")
		return nil, fmt.Errorf("resolve pragma home: %w", err)
	}
	dir := filepath.Join(pragmaHome, "active-sessions")
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
	if info.PID <= 0 {
		observe.GlobalTrace("if: info.PID <= 0")
		observe.GlobalTrace("return: fmt.Errorf(\"register background process: invalid PID %d\", info.PID)")
		return fmt.Errorf("register background process: invalid PID %d", info.PID)
	}
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

// UpdateStatus updates the process status and heartbeat in a PID file.
func (r *Registry) UpdateStatus(pid int, ownerToken string, status Status) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.updateOwned(pid, ownerToken, func(info *ProcessInfo, now time.Time) {
		info.Status = status
		info.UpdatedAt = now
		info.HeartbeatAt = now
	})
}

// UpdateSessionID attaches the real session identity after the child runtime
// starts a domain session.
func (r *Registry) UpdateSessionID(pid int, ownerToken string, sessionID string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if sessionID == "" {
		observe.GlobalTrace("if: sessionID == \"\"")
		return
	}
	r.updateOwned(pid, ownerToken, func(info *ProcessInfo, now time.Time) {
		info.SessionID = sessionID
		info.UpdatedAt = now
		info.HeartbeatAt = now
	})
}

// UpdateHeartbeat records child-owned liveness without changing presentation status.
func (r *Registry) UpdateHeartbeat(pid int, ownerToken string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.updateOwned(pid, ownerToken, func(info *ProcessInfo, now time.Time) {
		info.UpdatedAt = now
		info.HeartbeatAt = now
	})
}

func (r *Registry) updateOwned(pid int, ownerToken string, mutate func(*ProcessInfo, time.Time)) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	info, err := r.Get(pid)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		return
	}
	if ownerToken == "" || (info.OwnerToken != "" && info.OwnerToken != ownerToken) {
		observe.GlobalTrace("if: owner token mismatch")
		return
	}
	if info.OwnerToken == "" {
		observe.GlobalTrace("if: info.OwnerToken == \"\"")
		info.OwnerToken = ownerToken
	}
	mutate(&info, time.Now())
	_ = r.Register(info)
}

// ListProcesses returns all active background process records.
// Validates each record has a fresh child-owned heartbeat, sorts by StartedAt descending.
func (r *Registry) ListProcesses() ([]ProcessInfo, error) {
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
	now := time.Now()
	for _, entry := range entries {
		observe.GlobalTrace("range entries")
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			observe.GlobalTrace("if: entry.IsDir() || filepath.Ext(entry.Name()) != \".json\"")
			continue
		}

		data, err := os.ReadFile(filepath.Join(r.dir, entry.Name()))
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			continue
		}

		var info ProcessInfo
		// REG-001: only files carrying a "pid" field are provably registry
		// records. Lenient unmarshaling would treat any JSON object as an
		// empty ProcessInfo; a nil probe PID means the file is not ours and
		// must be left untouched (TS #34210 posture).
		var probe struct {
			PID *int `json:"pid"`
		}
		if err := json.Unmarshal(data, &probe); err != nil || probe.PID == nil {
			observe.GlobalTrace("if: err != nil")
			continue
		}
		if err := json.Unmarshal(data, &info); err != nil {
			observe.GlobalTrace("if: err != nil")
			continue
		}

		if !pidFilePattern.MatchString(entry.Name()) {
			// REG-001: a record under a non-PID name is unreachable by every
			// control path. Sweep it only when it provably belongs to this
			// registry and carries an invalid PID (e.g. gogent-era -1.json).
			observe.GlobalTrace("if: !pidFilePattern.MatchString(entry.Name())")
			if *probe.PID <= 0 {
				observe.GlobalTrace("if: *probe.PID <= 0")
				_ = os.Remove(filepath.Join(r.dir, entry.Name()))
			}
			continue
		}

		if !r.recordActive(info, now) {
			observe.GlobalTrace("if: !r.recordActive(info, now)")
			// REG-001: dead process plus stale heartbeat is provably garbage;
			// sweep it so the registry stays self-cleaning. A live process's
			// record is never removed here, whatever its heartbeat age.
			if info.PID > 0 && !isProcessAlive(info.PID) && !info.HasFreshHeartbeat(now) {
				observe.GlobalTrace("if: info.PID > 0 && !isProcessAlive(info.PID) && !info.HasFreshHeartbeat(now)")
				_ = os.Remove(filepath.Join(r.dir, entry.Name()))
			}
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

func (r *Registry) recordActive(info ProcessInfo, now time.Time) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: info.PID > 0 && info.HasFreshHeartbeat(now) && isProcessAlive(info.PID)")
	return info.PID > 0 && info.HasFreshHeartbeat(now) && isProcessAlive(info.PID)
}

func (r *Registry) validateControlRecord(info ProcessInfo, now time.Time) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if info.PID <= 0 {
		observe.GlobalTrace("if: info.PID <= 0")
		observe.GlobalTrace("return: fmt.Errorf(\"background record has invalid PID %d\", info.PID)")
		return fmt.Errorf("background record has invalid PID %d", info.PID)
	}
	if !info.HasFreshHeartbeat(now) {
		observe.GlobalTrace("if: !info.HasFreshHeartbeat(now)")
		_ = r.Unregister(info.PID)
		observe.GlobalTrace("return: fmt.Errorf(\"background record for PID %d is stale; removed without killing\", ...")
		return fmt.Errorf("background record for PID %d is stale; removed without killing", info.PID)
	}
	if !isProcessAlive(info.PID) {
		observe.GlobalTrace("if: !isProcessAlive(info.PID)")
		_ = r.Unregister(info.PID)
		observe.GlobalTrace("return: fmt.Errorf(\"background process %d is not running; removed stale record\", info...")
		return fmt.Errorf("background process %d is not running; removed stale record", info.PID)
	}
	observe.GlobalTrace("return: nil")
	return nil
}

// ListSessions returns active background processes that have attached to a
// domain session. Startup records without SessionID remain process records.
func (r *Registry) ListSessions() ([]ProcessInfo, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	processes, err := r.ListProcesses()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, err")
		return nil, err
	}
	sessions := make([]ProcessInfo, 0, len(processes))
	for _, info := range processes {
		observe.GlobalTrace("range processes")
		if info.HasSession() {
			observe.GlobalTrace("if: info.HasSession()")
			sessions = append(sessions, info)
		}
	}
	observe.GlobalTrace("return: sessions, nil")
	return sessions, nil
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
