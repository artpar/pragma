package powershell

import (
	"github.com/artpar/gogent/internal/observe"
	"os/exec"
	"syscall"
)

func setProcAttr(cmd *exec.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
