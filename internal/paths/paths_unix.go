//go:build !windows

package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func stateDir() (string, error) {
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" || !filepath.IsAbs(base) {
		cache, err := os.UserCacheDir()
		if err != nil || !filepath.IsAbs(cache) {
			return "", errors.New("no private place for runtime files: set XDG_RUNTIME_DIR or HOME")
		}
		base = cache
	}
	dir := filepath.Join(base, "periscope")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create state dir: %w", err)
	}
	return dir, checkPrivate(dir)
}

// checkPrivate refuses a directory someone else could have prepared (a symlink, or owned by another
// user) and makes ours private if its mode is too open.
func checkPrivate(dir string) error {
	fi, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("check state dir: %w", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return fmt.Errorf("refusing state dir %s: not a plain directory", dir)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return fmt.Errorf("refusing state dir %s: owned by another user", dir)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("make state dir private: %w", err)
		}
	}
	return nil
}
