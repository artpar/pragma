package lsp

import (
	"github.com/artpar/gogent/internal/observe"
	"os/exec"
	"syscall"
)

func setProcAttr(cmd *exec.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
