//go:build !windows

package hook

import (
	"os/exec"
	"syscall"

	"github.com/artpar/gogent/internal/observe"
)

func setProcAttr(cmd *exec.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
