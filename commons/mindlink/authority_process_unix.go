//go:build !windows

package mindlink

import "os/exec"

func configureAuthorityProcess(_ *exec.Cmd) {}
