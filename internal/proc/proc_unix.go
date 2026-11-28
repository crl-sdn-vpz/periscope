//go:build !windows

package proc

import (
	"syscall"
	"time"
)

// waitExit polls with signal 0 until the process is gone.
func waitExit(pid int, timeout time.Duration) bool {
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if syscall.Kill(pid, 0) != nil {
			return true
		}
	}
	return false
}
