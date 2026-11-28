package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// %LOCALAPPDATA% is private to the user by its ACL.
func stateDir() (string, error) {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" || !filepath.IsAbs(local) {
		return "", errors.New("LOCALAPPDATA is not set to an absolute path")
	}
	dir := filepath.Join(local, "periscope")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create state dir: %w", err)
	}
	return dir, nil
}
