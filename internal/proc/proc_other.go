//go:build !linux && !windows

package proc

import (
	"os/exec"
	"syscall"
)

func tie(*exec.Cmd) error { return nil }

func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
