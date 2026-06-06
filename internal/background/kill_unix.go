//go:build !windows

package background

import (
	"fmt"
	"syscall"
	"time"

	"github.com/artpar/pragma/internal/observe"
)

// Kill terminates a background process and its entire process group.
// Sends SIGTERM to the process group, waits up to 5 seconds, then SIGKILL.
// Addresses orphan process accumulation (GitHub #32964, #15945, #26658).
func (r *Registry) Kill(pid int) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	info, err := r.Get(pid)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}

	pgid := info.PGID
	if pgid == 0 {
		observe.GlobalTrace("if: pgid == 0")
		pgid = pid
	}

	if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil {
		observe.GlobalTrace("if: err != nil")
		if err != syscall.ESRCH {
			observe.GlobalTrace("if: err != syscall.ESRCH")
			observe.GlobalTrace("return: fmt.Errorf(\"SIGTERM process group %d: %w\", pgid, err)")
			return fmt.Errorf("SIGTERM process group %d: %w", pgid, err)
		}
		_ = r.Unregister(pid)
		observe.GlobalTrace("return: nil")
		return nil
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		observe.GlobalTrace("for: time.Now().Before(deadline)")
		if !isProcessAlive(pid) {
			observe.GlobalTrace("if: !isProcessAlive(pid)")
			_ = r.Unregister(pid)
			observe.GlobalTrace("return: nil")
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	_ = syscall.Kill(-pgid, syscall.SIGKILL)
	_ = r.Unregister(pid)
	observe.GlobalTrace("return: nil")
	return nil
}

// isProcessAlive checks if a process with the given PID exists.
func isProcessAlive(pid int) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: syscall.Kill(pid, 0) == nil")
	return syscall.Kill(pid, 0) == nil
}
