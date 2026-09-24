//go:build !windows

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// detachSpawn starts a grandchild-proof daemon: its own session (Setsid),
// stdio redirected to logfile, parent does not wait.
func detachSpawn(exe string, args []string, logfile string) (int, error) {
	logf, err := os.OpenFile(logfile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, fmt.Errorf("open daemon log: %w", err)
	}
	defer logf.Close()
	devnull, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0)
	if err != nil {
		return 0, fmt.Errorf("open devnull: %w", err)
	}
	defer devnull.Close()
	attr := &os.ProcAttr{
		Dir:   ".",
		Env:   os.Environ(),
		Files: []*os.File{devnull, logf, logf},
		Sys:   &syscall.SysProcAttr{Setsid: true},
	}
	proc, err := os.StartProcess(exe, append([]string{exe}, args...), attr)
	if err != nil {
		return 0, fmt.Errorf("start process: %w", err)
	}
	pid := proc.Pid
	_ = proc.Release()
	return pid, nil
}

// daemonAlive reports whether the pid recorded in pidfile is still
// running. A missing/stale pidfile means "not running".
func daemonAlive(pidfile string) (int, bool) {
	b, err := os.ReadFile(pidfile)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	// Signal 0: existence check. ESRCH → dead; EPERM → alive (other owner).
	if err := syscall.Kill(pid, 0); err != nil {
		return 0, false
	}
	return pid, true
}
