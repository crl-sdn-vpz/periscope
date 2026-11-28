//go:build !windows

package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStateDirIsPrivate(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", base)
	if err := os.Mkdir(filepath.Join(base, "periscope"), 0o777); err != nil { // too open: gets fixed
		t.Fatal(err)
	}
	dir, err := StateDir()
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(dir); st.Mode().Perm() != 0o700 {
		t.Fatalf("mode %v, want 0700", st.Mode().Perm())
	}
}

func TestStateDirRefusesSymlink(t *testing.T) {
	base, elsewhere := t.TempDir(), t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", base)
	if err := os.Symlink(elsewhere, filepath.Join(base, "periscope")); err != nil {
		t.Fatal(err)
	}
	if _, err := StateDir(); err == nil || !strings.Contains(err.Error(), "not a plain directory") {
		t.Fatalf("want a refusal, got %v", err)
	}
}

func TestStateDirNeedsAPrivateBase(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")
	if _, err := StateDir(); err == nil {
		t.Fatal("no XDG_RUNTIME_DIR and no HOME must fail, not fall back to a shared /tmp path")
	}
}
