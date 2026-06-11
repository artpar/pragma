package shellrun

import (
	"github.com/artpar/pragma/internal/observe"
	"os/exec"
	"syscall"
)

func setProcAttr(cmd *exec.Cmd) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
