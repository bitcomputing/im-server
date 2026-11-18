//go:build windows

package mindlink

import (
	"os/exec"
	"syscall"
)

func configureAuthorityProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
