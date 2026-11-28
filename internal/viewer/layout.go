package viewer

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/crl-sdn-vpz/periscope/internal/config"
	"github.com/crl-sdn-vpz/periscope/internal/mpv"
	"github.com/crl-sdn-vpz/periscope/internal/x11"
)

// ---- monitors ---------------------------------------------------------------------

// screenLabel describes the feed's monitor for status lines.
func (v *viewer) screenLabel() string {
	v.mu.Lock()
	n := v.screen
	v.mu.Unlock()
	if v.x11err != nil {
		return fmt.Sprintf("monitor %d", n)
	}
	ms, err := x11.Monitors()
	if err != nil {
		return fmt.Sprintf("monitor %d (%v)", n, err)
	}
	for _, m := range ms {
		if m.Index == n {
			return fmt.Sprintf("%s, %d of %d", m, n+1, len(ms))
		}
	}
	return fmt.Sprintf("monitor %d (not connected; periscope screens)", n)
}

// setScreen moves the feed to monitor arg ("next" cycles); an empty arg reports the current one.
func (v *viewer) setScreen(arg string) (string, error) {
	if arg == "" {
		return "feed on " + v.screenLabel(), nil
	}
	count := 1
	if v.x11err == nil {
		ms, err := x11.Monitors()
		if err != nil {
			return "", err
		}
		count = len(ms)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	n := v.screen
	if arg == "next" {
		n = (v.screen + 1) % count
	} else {
		var err error
		if n, err = strconv.Atoi(arg); err != nil || n < 0 || (v.x11err == nil && n >= count) {
			return "", fmt.Errorf("monitor must be next or 0-%d (periscope screens), got %q", count-1, arg)
		}
	}
	if v.x11err != nil {
		// No window control: let mpv pick the output, then re-enter fullscreen there.
		if err := errors.Join(v.mpv.Set("fs-screen", n), v.mpv.Set("screen", n),
			v.mpv.Set("fullscreen", false), v.mpv.Set("fullscreen", v.mode != "split-left" && v.mode != "split-right")); err != nil {
			return "", err
		}
		v.screen = n
		return fmt.Sprintf("feed on monitor %d", n), nil
	}
	prev := v.screen
	v.screen = n
	msg, err := v.applyMode(v.mode)
	if err != nil {
		v.screen = prev
		return "", err
	}
	return msg, nil
}

// ---- layout -----------------------------------------------------------------------

// mpvWindow finds mpv's window, waiting up to 5 s for it to map. Callers hold v.mu.
func (v *viewer) mpvWindow() (uint32, error) {
	pattern := "^" + mpv.Title + "$"
	for i := 0; i < 50; i++ {
		if id, err := x11.FindByName(pattern); err == nil {
			v.mpvID = id
			return id, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return 0, errors.New("viewer window not found")
}

func (v *viewer) setMode(mode string) (string, error) {
	if !slices.Contains(config.Modes, mode) {
		return "", fmt.Errorf("unknown mode %q (%s)", mode, strings.Join(config.Modes, " | "))
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.x11err != nil {
		var err error
		note := ""
		switch mode {
		case "full":
			err = errors.Join(v.mpv.Set("fullscreen", true), v.mpv.Set("ontop", true))
		case "overlay":
			err = errors.Join(v.mpv.Set("ontop", false), v.mpv.Set("fullscreen", true))
		default:
			err = errors.Join(v.mpv.Set("ontop", false), v.mpv.Set("fullscreen", false))
			note = " (place the window with your desktop's tiling keys: " + v.x11err.Error() + ")"
		}
		if err != nil {
			return "", err
		}
		v.mode, v.peekFrom = mode, -1
		return "mode: " + mode + note, nil
	}
	msg, err := v.applyMode(mode)
	if err != nil {
		return "", err
	}
	v.mode, v.peekFrom = mode, -1
	return msg, nil
}

// applyMode places mpv's window for mode on the feed monitor and fades windows to match.
// Callers hold v.mu.
func (v *viewer) applyMode(mode string) (string, error) {
	id, err := v.mpvWindow()
	if err != nil {
		return "", err
	}
	mon, err := x11.MonitorAt(v.screen)
	if err != nil {
		return "", err
	}
	where := fmt.Sprintf(" on monitor %d (%s)", mon.Index, mon.Name)
	switch mode {
	case "overlay", "full":
		if err := v.moveToMonitor(id, mon); err != nil {
			return "", err
		}
		if mode == "overlay" {
			if err := errors.Join(v.mpv.Set("ontop", false), v.mpv.Set("fullscreen", true),
				x11.SetState(id, "add", "below", "skip_taskbar")); err != nil {
				return "", err
			}
			n, err := v.applyOpacity(v.cfg.OverlayOpacity)
			if err != nil {
				return "", err
			}
			return "mode: overlay" + where + fadedNote(n, v.cfg.OverlayOpacity, mon), nil
		}
		if err := errors.Join(x11.SetState(id, "remove", "below"), v.mpv.Set("fullscreen", true),
			v.mpv.Set("ontop", true)); err != nil {
			return "", err
		}
		if _, err := v.applyOpacity(1); err != nil {
			return "", err
		}
		return "mode: full" + where, nil
	default: // split-left, split-right
		if err := errors.Join(v.mpv.Set("ontop", false), v.mpv.Set("fullscreen", false)); err != nil {
			return "", err
		}
		// wmctrl changes at most two state flags per call.
		if err := errors.Join(x11.SetState(id, "remove", "below", "fullscreen"),
			x11.SetState(id, "remove", "maximized_vert", "maximized_horz")); err != nil {
			return "", err
		}
		r := mon.Rect
		w := int(float64(r.W) * v.cfg.SplitRatio)
		x := r.X
		if mode == "split-right" {
			x = r.X + r.W - w
		}
		time.Sleep(150 * time.Millisecond) // let the WM finish leaving fullscreen
		if err := x11.MoveResize(id, x, r.Y, w, r.H); err != nil {
			return "", err
		}
		if _, err := v.applyOpacity(1); err != nil {
			return "", err
		}
		return "mode: " + mode + where, nil
	}
}

// moveToMonitor puts a (possibly fullscreen) window on mon unless it is already there. The WM
// fullscreens a window on the monitor that holds it, so it leaves fullscreen, moves, and the caller
// re-enters fullscreen.
func (v *viewer) moveToMonitor(id uint32, mon x11.Monitor) error {
	ws, err := x11.Windows()
	if err != nil {
		return err
	}
	for _, w := range ws {
		if w.ID == id && mon.Rect.Contains(w.Rect.Center()) {
			return nil
		}
	}
	if err := v.mpv.Set("fullscreen", false); err != nil {
		return err
	}
	time.Sleep(150 * time.Millisecond)
	r := mon.Rect
	if err := x11.MoveResize(id, r.X+r.W/4, r.Y+r.H/4, r.W/2, r.H/2); err != nil {
		return err
	}
	time.Sleep(150 * time.Millisecond)
	return nil
}

func (v *viewer) cycleMode() (string, error) {
	v.mu.Lock()
	cur := v.mode
	v.mu.Unlock()
	next := map[string]string{"overlay": "split-left", "split-left": "full"}[cur]
	if next == "" {
		next = "overlay"
	}
	return v.setMode(next)
}

// ---- opacity ----------------------------------------------------------------------

// eligible reports whether a window may be faded: managed, not sticky (desktop, panels, docks),
// not the feed itself, and matching the class filters. Callers hold v.mu.
func (v *viewer) eligible(w x11.Window) bool {
	if w.Desktop == -1 || w.ID == v.mpvID {
		return false
	}
	class, ok := v.classes[w.ID]
	if !ok {
		class = x11.Class(w.ID)
		v.classes[w.ID] = class
	}
	if v.cfg.OverlayInclude != "" && !v.include.MatchString(class) {
		return false
	}
	return v.cfg.OverlayExclude == "" || !v.exclude.MatchString(class)
}

// applyOpacity fades the eligible windows centered on the feed's monitor to o and un-fades the
// rest (fading a window on another monitor only dims it). It returns how many windows are at o.
// With o = 1 it clears the property on every eligible window, also ones a crashed run left faded.
// Callers hold v.mu.
func (v *viewer) applyOpacity(o float64) (int, error) {
	if v.x11err != nil {
		return 0, v.x11err
	}
	ws, err := x11.Windows()
	if err != nil {
		return 0, err
	}
	mon, err := x11.MonitorAt(v.screen)
	if err != nil {
		return 0, err
	}
	if id, err := x11.FindByName("^" + mpv.Title + "$"); err == nil {
		v.mpvID = id
	}
	seen := map[uint32]bool{}
	n := 0
	var errs []error
	for _, w := range ws {
		if !v.eligible(w) {
			continue
		}
		seen[w.ID] = true
		want := 1.0
		if o < 0.999 && mon.Rect.Contains(w.Rect.Center()) {
			want = o
		}
		if want >= 0.999 {
			if _, faded := v.faded[w.ID]; faded || o >= 0.999 {
				if err := x11.SetOpacity(w.ID, 1); err != nil {
					errs = append(errs, err)
				}
				delete(v.faded, w.ID)
			}
			continue
		}
		n++
		if cur, ok := v.faded[w.ID]; !ok || math.Abs(cur-want) > 0.001 {
			if err := x11.SetOpacity(w.ID, want); err != nil {
				errs = append(errs, err)
				continue
			}
			v.faded[w.ID] = want
		}
	}
	for id := range v.faded {
		if !seen[id] {
			delete(v.faded, id)
		}
	}
	for id := range v.classes {
		if !seen[id] {
			delete(v.classes, id)
		}
	}
	v.opacity = o
	if len(errs) > 0 {
		v.log.Printf("set opacity on %d window(s) failed: %v", len(errs), errors.Join(errs...))
	}
	return n, nil
}

// reconcile keeps fading in step with windows that opened, closed or moved between monitors.
func (v *viewer) reconcile() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.opacity >= 0.999 && len(v.faded) == 0 {
		return
	}
	if _, err := v.applyOpacity(v.opacity); err != nil {
		v.log.Printf("reconcile windows: %v", err)
	}
}

// fadedNote tells the user what an opacity change touched, including when it touched nothing.
func fadedNote(n int, o float64, mon x11.Monitor) string {
	if n == 0 && o < 0.999 {
		return fmt.Sprintf(" (none of your windows are on monitor %d (%s) yet: move one there, or put the feed where you work: periscope screen next)", mon.Index, mon.Name)
	}
	if o < 0.999 {
		return fmt.Sprintf(", %d window(s) at %.2f", n, o)
	}
	return ""
}

func (v *viewer) setOpacityCmd(arg string) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	var o float64
	switch arg {
	case "":
		return fmt.Sprintf("%.2f", v.opacity), nil
	case "reset":
		o = 1
	case "cycle":
		o = nextStep(v.cfg.OpacitySteps, v.opacity)
	default:
		var err error
		o, err = strconv.ParseFloat(arg, 64)
		if err != nil || o < 0 || o > 1 {
			return "", fmt.Errorf("opacity must be between 0 and 1, cycle or reset (got %q)", arg)
		}
	}
	if err := v.overlayOnly(); err != nil {
		return "", err
	}
	n, err := v.applyOpacity(o)
	if err != nil {
		return "", err
	}
	v.peekFrom = -1
	mon, _ := x11.MonitorAt(v.screen)
	return fmt.Sprintf("opacity: %.2f", o) + fadedNote(n, o, mon), nil
}

// overlayOnly refuses opacity changes outside overlay mode, where the feed is not behind the
// windows and fading them would only dim them. Callers hold v.mu.
func (v *viewer) overlayOnly() error {
	if v.mode != "overlay" {
		return fmt.Errorf("opacity and peek work in overlay mode (now: %s); switch with: periscope mode overlay", v.mode)
	}
	return nil
}

// nextStep returns the step after cur, or the first step when cur isn't one of them.
func nextStep(steps []float64, cur float64) float64 {
	for i, s := range steps {
		if math.Abs(s-cur) < 0.005 && i+1 < len(steps) {
			return steps[i+1]
		}
	}
	return steps[0]
}

func (v *viewer) peek() (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.overlayOnly(); err != nil {
		return "", err
	}
	mon, _ := x11.MonitorAt(v.screen)
	if v.peekFrom >= 0 {
		restore := v.peekFrom
		n, err := v.applyOpacity(restore)
		if err != nil {
			return "", err
		}
		v.peekFrom = -1
		return fmt.Sprintf("peek off: opacity %.2f", restore) + fadedNote(n, restore, mon), nil
	}
	from := v.opacity
	n, err := v.applyOpacity(v.cfg.PeekOpacity)
	if err != nil {
		return "", err
	}
	v.peekFrom = from
	return fmt.Sprintf("peek on: opacity %.2f", v.cfg.PeekOpacity) + fadedNote(n, v.cfg.PeekOpacity, mon), nil
}
