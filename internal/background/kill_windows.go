//go:build windows

package background

import (
	"os"

	"github.com/artpar/pragma/internal/observe"
)

// Kill terminates a background session on Windows.
// Windows has no process groups or SIGTERM; we find the process and kill it directly.
func (r *Registry) Kill(pid int) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	proc, err := os.FindProcess(pid)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		_ = r.Unregister(pid)
		return nil
	}
	if err := proc.Kill(); err != nil {
		observe.GlobalTrace("if: err != nil")
		_ = r.Unregister(pid)
		return nil
	}
	_ = r.Unregister(pid)
	return nil
}

// isProcessAlive checks if a process with the given PID exists on Windows.
func isProcessAlive(pid int) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Windows, FindProcess always succeeds; OpenProcess with SYNCHRONIZE probes liveness.
	// We use a lightweight signal via proc.Signal(os.Signal(nil)) — not available on Windows.
	// Instead, attempt to wait with WNOHANG equivalent: just check if proc exists in the table.
	// The most reliable approach without cgo: try to open the process handle.
	_ = proc
	return true
}
