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

// Window is a managed top-level window and its frame geometry.
type Window struct {
	ID      uint32
	Desktop int // -1 = on all desktops: panels, but also ordinary windows (GNOME puts every window
	// on a non-primary monitor there by default), so it says nothing about the window's kind
	Rect Rect
}

func run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return string(out), nil
}

func hex(id uint32) string { return fmt.Sprintf("0x%08x", id) }

// Windows lists managed windows with their geometry (wmctrl -lG).
func Windows() ([]Window, error) {
	out, err := run("wmctrl", "-lG")
	if err != nil {
		return nil, err
	}
	return parseWmctrl(out), nil
}

// parseWmctrl reads `wmctrl -lG` lines: id desktop x y w h host title...
func parseWmctrl(out string) []Window {
	var ws []Window
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 6 {
			continue
		}
		id, err := strconv.ParseUint(strings.TrimPrefix(f[0], "0x"), 16, 32)
		if err != nil {
			continue
		}
		var n [5]int
		ok := true
		for i := range n {
			if n[i], err = strconv.Atoi(f[i+1]); err != nil {
				ok = false
			}
		}
		if ok {
			ws = append(ws, Window{ID: uint32(id), Desktop: n[0], Rect: Rect{X: n[1], Y: n[2], W: n[3], H: n[4]}})
		}
	}
	return ws
}

// Info is what decides whether a window may be faded.
type Info struct {
	Class string   // WM_CLASS value, e.g. `"kitty", "kitty"`
	Types []string // _NET_WM_WINDOW_TYPE without the prefix, e.g. NORMAL, DOCK, DESKTOP
}

// Background reports whether the window is part of the desktop itself (wallpaper/icons, panels, docks).
func (i Info) Background() bool {
	for _, t := range i.Types {
		if t == "DESKTOP" || t == "DOCK" {
			return true
		}
	}
	return false
}

// WindowInfo reads a window's class and type in one xprop call.
func WindowInfo(id uint32) Info {
	out, err := run("xprop", "-id", hex(id), "WM_CLASS", "_NET_WM_WINDOW_TYPE")
	if err != nil {
		return Info{}
	}
	return parseInfo(out)
}

func parseInfo(out string) Info {
	var info Info
	for _, line := range strings.Split(out, "\n") {
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			continue // e.g. "_NET_WM_WINDOW_TYPE:  not found."
		}
		switch {
		case strings.HasPrefix(name, "WM_CLASS"):
			info.Class = strings.TrimSpace(value)
		case strings.HasPrefix(name, "_NET_WM_WINDOW_TYPE"):
			for _, t := range strings.Split(value, ",") {
				info.Types = append(info.Types, strings.TrimPrefix(strings.TrimSpace(t), "_NET_WM_WINDOW_TYPE_"))
			}
		}
	}
	return info
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

// Rect is a screen-space rectangle.
type Rect struct{ X, Y, W, H int }

// Contains reports whether the point (x, y) lies inside r.
func (r Rect) Contains(x, y int) bool { return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H }

// Center returns r's center point.
func (r Rect) Center() (int, int) { return r.X + r.W/2, r.Y + r.H/2 }

// Monitor is an active output, numbered as `xrandr --listactivemonitors` numbers it.
type Monitor struct {
	Index   int
	Name    string
	Primary bool
	Rect    Rect
}

func (m Monitor) String() string {
	s := fmt.Sprintf("%d: %s %dx%d+%d+%d", m.Index, m.Name, m.Rect.W, m.Rect.H, m.Rect.X, m.Rect.Y)
	if m.Primary {
		s += " (primary)"
	}
	return s
}

// Monitors lists active monitors (xrandr), or the whole display as monitor 0 if xrandr can't tell.
func Monitors() ([]Monitor, error) {
	if out, err := run("xrandr", "--listactivemonitors"); err == nil {
		if ms := parseMonitors(out); len(ms) > 0 {
			return ms, nil
		}
	}
	out, err := run("xdotool", "getdisplaygeometry")
	if err != nil {
		return nil, err
	}
	var r Rect
	if _, err := fmt.Sscanf(out, "%d %d", &r.W, &r.H); err != nil {
		return nil, fmt.Errorf("parse display geometry %q: %w", out, err)
	}
	return []Monitor{{Index: 0, Name: "display", Primary: true, Rect: r}}, nil
}

// MonitorAt returns monitor n.
func MonitorAt(n int) (Monitor, error) {
	ms, err := Monitors()
	if err != nil {
		return Monitor{}, err
	}
	for _, m := range ms {
		if m.Index == n {
			return m, nil
		}
	}
	return Monitor{}, fmt.Errorf("no monitor %d (there are %d: periscope screens)", n, len(ms))
}

// e.g. " 1: +*HDMI-1 1920/527x1080/296+2560+180  HDMI-1"; '*' marks the primary monitor.
var monitorRe = regexp.MustCompile(`^\s*(\d+):\s+\+?(\*?)(\S+)\s+(\d+)/\d+x(\d+)/\d+\+(\d+)\+(\d+)`)

func parseMonitors(out string) []Monitor {
	var ms []Monitor
	for _, line := range strings.Split(out, "\n") {
		m := monitorRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		var n [5]int
		for i, s := range []string{m[1], m[4], m[5], m[6], m[7]} {
			n[i], _ = strconv.Atoi(s)
		}
		ms = append(ms, Monitor{Index: n[0], Name: m[3], Primary: m[2] == "*",
			Rect: Rect{X: n[3], Y: n[4], W: n[1], H: n[2]}})
	}
	return ms
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
