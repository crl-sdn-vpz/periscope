// Package clipboard copies screenshots and text with the desktop's clipboard tool (wl-copy or xclip).
package clipboard

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

func tool(mime string) (*exec.Cmd, error) {
	wayland := os.Getenv("XDG_SESSION_TYPE") == "wayland" || (os.Getenv("WAYLAND_DISPLAY") != "" && os.Getenv("DISPLAY") == "")
	if _, err := exec.LookPath("wl-copy"); err == nil && wayland {
		return exec.Command("wl-copy", "--type", mime), nil
	}
	if _, err := exec.LookPath("xclip"); err == nil {
		return exec.Command("xclip", "-selection", "clipboard", "-t", mime, "-i"), nil
	}
	return nil, errors.New("no clipboard tool (install xclip or wl-clipboard)")
}

func copyFrom(r io.Reader, mime string) error {
	cmd, err := tool(mime)
	if err != nil {
		return err
	}
	cmd.Stdin = r
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w %s", cmd.Path, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// CopyPNG puts the image at path on the clipboard.
func CopyPNG(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return copyFrom(f, "image/png")
}

// CopyText puts s on the clipboard.
func CopyText(s string) error { return copyFrom(strings.NewReader(s), "text/plain") }
