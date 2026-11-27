// Package proc ties child processes (ffmpeg, mpv) to periscope's lifetime and detaches daemons.
package proc

import "os/exec"

// Start starts cmd so that it dies with this process where the OS allows it (Linux: parent-death
// signal; Windows: this process joins a kill-on-close job, which every child is created in).
func Start(cmd *exec.Cmd) error {
	if err := tie(cmd); err != nil {
		return err
	}
	return cmd.Start()
}

// Detach makes cmd outlive this process and its terminal (a new session / process group).
func Detach(cmd *exec.Cmd) { detach(cmd) }
