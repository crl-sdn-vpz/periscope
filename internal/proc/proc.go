// Package proc ties child processes (ffmpeg, mpv) to periscope's lifetime and detaches daemons.
package proc

import "os/exec"

// Start starts cmd so that it dies with this process where the OS allows it
// (Linux: parent-death signal; Windows: a kill-on-close job object).
func Start(cmd *exec.Cmd) error {
	tie(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return afterStart(cmd)
}

// Detach makes cmd outlive this process and its terminal (a new session / process group).
func Detach(cmd *exec.Cmd) { detach(cmd) }
