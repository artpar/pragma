//go:build windows

package cli

import (
	"syscall"

	"github.com/artpar/pragma/internal/observe"
)

func daemonSysProcAttr() *syscall.SysProcAttr {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &syscall.SysProcAttr{}")
	return &syscall.SysProcAttr{}
}
