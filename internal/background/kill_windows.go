//go:build windows

package background

import (
	"os"
	"time"

	"github.com/artpar/pragma/internal/observe"
)

// Kill terminates a background process on Windows.
// Windows has no process groups or SIGTERM; we find the process and kill it directly.
func (r *Registry) Kill(pid int) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	info, err := r.Get(pid)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		return err
	}
	if err := r.validateControlRecord(info, time.Now()); err != nil {
		observe.GlobalTrace("if: r.validateControlRecord != nil")
		return err
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		_ = r.Unregister(pid)
		observe.GlobalTrace("return: nil")
		return nil
	}
	if err := proc.Kill(); err != nil {
		observe.GlobalTrace("if: err != nil")
		_ = r.Unregister(pid)
		observe.GlobalTrace("return: nil")
		return nil
	}
	_ = r.Unregister(pid)
	observe.GlobalTrace("return: nil")
	return nil
}

// isProcessAlive checks if a process with the given PID exists on Windows.
func isProcessAlive(pid int) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	proc, err := os.FindProcess(pid)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: false")
		return false
	}

	_ = proc
	observe.GlobalTrace("return: true")
	return true
}
