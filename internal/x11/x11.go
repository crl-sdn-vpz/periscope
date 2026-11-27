// Package x11 controls other windows on an X11 desktop (EWMH) through wmctrl, xprop, xrandr and xdotool,
// like the bash viewer did. A pure-Go X protocol client can replace it later without changing callers.
package x11

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// Available reports why window control can't work in this session, or nil if it can.
func Available() error {
	if os.Getenv("XDG_SESSION_TYPE") == "wayland" || (os.Getenv("WAYLAND_DISPLAY") != "" && os.Getenv("DISPLAY") == "") {
		return errors.New("window opacity and placement need an X11 session (see README > Wayland)")
	}
	if os.Getenv("DISPLAY") == "" {
		return errors.New("no X11 display (DISPLAY is empty)")
	}
	for _, tool := range []string{"wmctrl", "xprop", "xdotool"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("%s is not installed", tool)
		}
	}
	return nil
}

// Window is a managed top-level window.
type Window struct {
	ID      uint32
	Desktop int // -1 = sticky (desktop, panels, docks)
}

func run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return string(out), nil
}

func hex(id uint32) string { return fmt.Sprintf("0x%08x", id) }

// Windows lists managed windows (wmctrl -l).
func Windows() ([]Window, error) {
	out, err := run("wmctrl", "-l")
	if err != nil {
		return nil, err
	}
	return parseWmctrl(out), nil
}

func parseWmctrl(out string) []Window {
	var ws []Window
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		id, err1 := strconv.ParseUint(strings.TrimPrefix(f[0], "0x"), 16, 32)
		desk, err2 := strconv.Atoi(f[1])
		if err1 != nil || err2 != nil {
			continue
		}
		ws = append(ws, Window{ID: uint32(id), Desktop: desk})
	}
	return ws
}

// Class returns the WM_CLASS value line of a window ("" if it has none).
func Class(id uint32) string {
	out, err := run("xprop", "-id", hex(id), "WM_CLASS")
	if err != nil {
		return ""
	}
	if _, v, ok := strings.Cut(out, "="); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// Cardinal converts an opacity in [0,1] to the _NET_WM_WINDOW_OPACITY value.
func Cardinal(o float64) uint32 {
	o = max(0, min(1, o))
	return uint32(o * 0xffffffff)
}

// SetOpacity sets a window's opacity; 1 removes the property so the compositor draws it opaque.
func SetOpacity(id uint32, o float64) error {
	if o >= 0.999 {
		_, err := run("xprop", "-id", hex(id), "-remove", "_NET_WM_WINDOW_OPACITY")
		return err
	}
	_, err := run("xprop", "-id", hex(id), "-f", "_NET_WM_WINDOW_OPACITY", "32c",
		"-set", "_NET_WM_WINDOW_OPACITY", strconv.FormatUint(uint64(Cardinal(o)), 10))
	return err
}

// HasOpacity reports whether a window already carries _NET_WM_WINDOW_OPACITY.
func HasOpacity(id uint32) bool {
	out, err := run("xprop", "-id", hex(id), "_NET_WM_WINDOW_OPACITY")
	return err == nil && strings.Contains(out, "=")
}

// SetState adds or removes _NET_WM_STATE flags, e.g. SetState(id, "add", "below", "skip_taskbar").
func SetState(id uint32, action string, flags ...string) error {
	_, err := run("wmctrl", "-i", "-r", hex(id), "-b", action+","+strings.Join(flags, ","))
	return err
}

// MoveResize places a window.
func MoveResize(id uint32, x, y, w, h int) error {
	_, err := run("wmctrl", "-i", "-r", hex(id), "-e", fmt.Sprintf("0,%d,%d,%d,%d", x, y, w, h))
	return err
}

// FindByName returns the first window whose title matches the regex.
func FindByName(pattern string) (uint32, error) {
	out, err := run("xdotool", "search", "--name", pattern)
	if err != nil {
		return 0, err
	}
	first, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	id, err := strconv.ParseUint(first, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("no window named %s", pattern)
	}
	return uint32(id), nil
}

// Rect is a monitor's geometry.
type Rect struct{ X, Y, W, H int }

// Monitor returns the geometry of monitor n (xrandr), or the whole display if xrandr can't tell.
func Monitor(n int) (Rect, error) {
	if out, err := run("xrandr", "--listactivemonitors"); err == nil {
		if r, ok := parseMonitor(out, n); ok {
			return r, nil
		}
	}
	out, err := run("xdotool", "getdisplaygeometry")
	if err != nil {
		return Rect{}, err
	}
	var r Rect
	if _, err := fmt.Sscanf(out, "%d %d", &r.W, &r.H); err != nil {
		return Rect{}, fmt.Errorf("parse display geometry %q: %w", out, err)
	}
	return r, nil
}

var monitorRe = regexp.MustCompile(`^\s*(\d+):\s+\S+\s+(\d+)/\d+x(\d+)/\d+\+(\d+)\+(\d+)`)

func parseMonitor(out string, n int) (Rect, bool) {
	for _, line := range strings.Split(out, "\n") {
		m := monitorRe.FindStringSubmatch(line)
		if m == nil || m[1] != strconv.Itoa(n) {
			continue
		}
		w, _ := strconv.Atoi(m[2])
		h, _ := strconv.Atoi(m[3])
		x, _ := strconv.Atoi(m[4])
		y, _ := strconv.Atoi(m[5])
		return Rect{X: x, Y: y, W: w, H: h}, true
	}
	return Rect{}, false
}

// WatchClientList calls fn whenever the window list changes, until ctx is done.
func WatchClientList(ctx context.Context, fn func()) error {
	cmd := exec.CommandContext(ctx, "xprop", "-root", "-spy", "_NET_CLIENT_LIST")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start xprop -spy: %w", err)
	}
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		fn()
	}
	return cmd.Wait()
}
