// Package paths locates periscope's runtime state: control sockets, logs and mpv's IPC socket.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
)

// StateDir returns (and creates, mode 0700) the per-user state directory:
// $XDG_RUNTIME_DIR/periscope on Linux, %LOCALAPPDATA%\periscope on Windows, the user cache dir elsewhere.
func StateDir() (string, error) {
	var dir string
	switch {
	case runtime.GOOS == "windows":
		dir = filepath.Join(os.Getenv("LOCALAPPDATA"), "periscope")
	case os.Getenv("XDG_RUNTIME_DIR") != "":
		dir = filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "periscope")
	default:
		cache, err := os.UserCacheDir()
		if err != nil {
			cache = filepath.Join(os.TempDir(), "periscope-"+strconv.Itoa(os.Getuid()))
		}
		dir = filepath.Join(cache, "periscope")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create state dir: %w", err)
	}
	return dir, nil
}
