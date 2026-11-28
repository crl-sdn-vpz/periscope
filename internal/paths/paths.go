// Package paths locates periscope's runtime state: control sockets, logs and mpv's IPC socket.
// Those sockets accept commands (mpv's can even start programs), so the directory must be private.
package paths

// StateDir returns (and creates, private to the user) the state directory:
// $XDG_RUNTIME_DIR/periscope (or the user cache dir) on Linux, %LOCALAPPDATA%\periscope on Windows.
func StateDir() (string, error) { return stateDir() }
