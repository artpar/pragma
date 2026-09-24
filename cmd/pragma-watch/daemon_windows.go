//go:build windows

package main

import "errors"

func detachSpawn(exe string, args []string, logfile string) (int, error) {
	return 0, errors.New("--daemon is not supported on windows yet")
}

func daemonAlive(pidfile string) (int, bool) {
	return 0, false
}
