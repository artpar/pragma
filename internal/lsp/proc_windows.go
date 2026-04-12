//go:build windows

package lsp

import "os/exec"

func setProcAttr(cmd *exec.Cmd) {
	// Windows does not support Setpgid.
}
