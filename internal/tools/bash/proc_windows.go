//go:build windows

package bash

import "os/exec"

func setProcAttr(cmd *exec.Cmd) {
	// Windows does not support Setpgid or process group signals.
	// The default Cancel (Process.Kill) is used.
}
