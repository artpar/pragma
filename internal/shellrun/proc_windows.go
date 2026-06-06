//go:build windows

package shellrun

import "os/exec"

func setProcAttr(cmd *exec.Cmd) {
	// Windows does not support Setpgid or process group signals.
}
