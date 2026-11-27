// Package viewer runs `periscope view`: mpv as an SRT listener, layout modes, overlay opacity for the
// user's own windows (X11), full-resolution frame capture, and a control socket for all of it.
package viewer

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/crl-sdn-vpz/periscope/internal/config"
	"github.com/crl-sdn-vpz/periscope/internal/control"
	"github.com/crl-sdn-vpz/periscope/internal/logfile"
	"github.com/crl-sdn-vpz/periscope/internal/mpv"
	"github.com/crl-sdn-vpz/periscope/internal/srt"
	"github.com/crl-sdn-vpz/periscope/internal/supervise"
	"github.com/crl-sdn-vpz/periscope/internal/x11"
)

// SocketName is the viewer's control socket inside the state dir.
const SocketName = "view.sock"

type viewer struct {
	cfg    config.View
	port   int
	mpv    *mpv.Client
	log    *log.Logger
	x11err error // why window control is unavailable; nil on a working X11 session

	include, exclude *regexp.Regexp

	mu       sync.Mutex // guards the fields below and serializes window changes
	mode     string
	opacity  float64
	peekFrom float64 // opacity to restore when peek ends; negative when not peeking
	cancel   context.CancelFunc
}

// Run listens for the sender and serves control requests until ctx is done or "stop" arrives.
func Run(ctx context.Context, c config.Config, stateDir, mode string) error {
	if err := c.ValidateView(); err != nil {
		return err
	}
	if mode == "" {
		mode = c.View.DefaultMode
	}
	if !slices.Contains(config.Modes, mode) {
		return fmt.Errorf("unknown mode %q (%s)", mode, strings.Join(config.Modes, " | "))
	}
	ln, err := control.Listen(filepath.Join(stateDir, SocketName))
	if errors.Is(err, control.ErrRunning) {
		return errors.New("the viewer is already running")
	}
	if err != nil {
		return err
	}
	lf, err := logfile.Open(filepath.Join(stateDir, "viewer.log"), 1<<20)
	if err != nil {
		return err
	}
	defer lf.Close()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	v := &viewer{
		cfg:      c.View,
		port:     c.Port,
		mpv:      &mpv.Client{Socket: filepath.Join(stateDir, "mpv.sock")},
		log:      log.New(lf, "", log.LstdFlags),
		x11err:   x11.Available(),
		include:  regexp.MustCompile(c.View.OverlayInclude), // validated by ValidateView
		exclude:  regexp.MustCompile(c.View.OverlayExclude),
		mode:     mode,
		opacity:  1,
		peekFrom: -1,
		cancel:   cancel,
	}
	if v.x11err != nil {
		v.log.Printf("window control off: %v", v.x11err)
	}

	inputConf := filepath.Join(stateDir, "input.conf")
	if err := os.WriteFile(inputConf, []byte(mpv.InputConf), 0o600); err != nil {
		return fmt.Errorf("write mpv input.conf: %w", err)
	}
	_ = os.Remove(v.mpv.Socket)
	url := srt.URL("listener", "0.0.0.0", c.Port, c.LatencyMs, c.Passphrase)
	args := mpv.Args(url, v.mpv.Socket, inputConf, filepath.Join(stateDir, "mpv.log"), c.View.Screen)
	supErr := make(chan error, 1)
	go func() {
		supErr <- supervise.Run(ctx, supervise.Options{
			Name:       "mpv",
			Command:    func(ctx context.Context) *exec.Cmd { return exec.CommandContext(ctx, c.View.Mpv, args...) },
			Log:        v.log,
			MinBackoff: time.Second, MaxBackoff: time.Second,
		})
		cancel()
	}()
	if err := waitForSocket(ctx, v.mpv.Socket, 10*time.Second); err != nil {
		cancel()
		if serr := <-supErr; serr != nil {
			return fmt.Errorf("start mpv: %w", serr)
		}
		return err
	}
	v.log.Printf("listening on srt://0.0.0.0:%d", c.Port)

	if v.x11err == nil {
		go func() {
			if err := x11.WatchClientList(ctx, v.fadeNewWindows); err != nil && ctx.Err() == nil {
				v.log.Printf("window watcher stopped: %v", err)
			}
		}()
	}
	if _, err := v.setMode(mode); err != nil {
		v.log.Printf("initial mode %s: %v", mode, err)
	}

	control.Serve(ctx, ln, v.handle)
	if v.x11err == nil {
		v.mu.Lock()
		if err := v.applyOpacity(1); err != nil {
			v.log.Printf("restore opacity: %v", err)
		}
		v.mu.Unlock()
	}
	_ = os.Remove(v.mpv.Socket)
	v.log.Print("viewer stopped")
	if err := <-supErr; err != nil {
		return fmt.Errorf("mpv: %w", err)
	}
	return nil
}

func waitForSocket(ctx context.Context, path string, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if c, err := net.Dial("unix", path); err == nil {
			_ = c.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("viewer stopped while waiting for mpv")
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("mpv did not open its IPC socket within %s", limit)
}

func (v *viewer) handle(_ context.Context, r control.Request) (string, error) {
	arg := ""
	if len(r.Args) > 0 {
		arg = r.Args[0]
	}
	switch r.Cmd {
	case "status":
		return v.status(), nil
	case "mode":
		if arg == "cycle" {
			return v.cycleMode()
		}
		return v.setMode(arg)
	case "peek":
		return v.peek()
	case "opacity":
		return v.setOpacityCmd(arg)
	case "shot":
		return v.shot()
	case "stop":
		go func() { time.Sleep(50 * time.Millisecond); v.cancel() }()
		return "viewer stopping", nil
	}
	return "", fmt.Errorf("unknown viewer command %q", r.Cmd)
}

func (v *viewer) status() string {
	v.mu.Lock()
	mode, opacity, peeking := v.mode, v.opacity, v.peekFrom >= 0
	v.mu.Unlock()
	var b strings.Builder
	fmt.Fprintf(&b, "viewer : running (mode: %s)\n", mode)
	var w, h int
	var fps float64
	// Ask for the rest only after width answers, so a hung mpv costs one timeout, not three.
	switch err := v.mpv.Get("width", &w); {
	case errors.Is(err, mpv.ErrUnavailable):
		fmt.Fprintf(&b, "stream : waiting for the sender on port %d\n", v.port)
	case err != nil:
		fmt.Fprintf(&b, "stream : mpv not answering (%v)\n", err)
	default:
		_ = v.mpv.Get("height", &h)
		// MPEG-TS often has no container rate; mpv's measured decode rate fills in.
		if v.mpv.Get("container-fps", &fps) != nil || fps == 0 {
			_ = v.mpv.Get("estimated-vf-fps", &fps)
		}
		fmt.Fprintf(&b, "stream : receiving %dx%d", w, h)
		if fps > 0 {
			fmt.Fprintf(&b, " @ %.0f fps", fps)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "opacity: %.2f", opacity)
	if peeking {
		b.WriteString(" (peeking)")
	}
	if v.x11err != nil {
		fmt.Fprintf(&b, " (window control off: %v)", v.x11err)
	}
	fmt.Fprintf(&b, "\nshots  : %s", v.cfg.ShotDir)
	return b.String()
}

// ---- layout -----------------------------------------------------------------------

func (v *viewer) mpvWindow() (uint32, error) {
	pattern := "^" + mpv.Title + "$"
	for i := 0; i < 50; i++ {
		if id, err := x11.FindByName(pattern); err == nil {
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
	note := ""
	if v.x11err != nil {
		var err error
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
	} else if err := v.applyMode(mode); err != nil {
		return "", err
	}
	v.mode, v.peekFrom = mode, -1
	return "mode: " + mode + note, nil
}

// applyMode places mpv's window for mode on X11. Callers hold v.mu.
func (v *viewer) applyMode(mode string) error {
	id, err := v.mpvWindow()
	if err != nil {
		return err
	}
	switch mode {
	case "overlay":
		if err := errors.Join(v.mpv.Set("ontop", false), v.mpv.Set("fullscreen", true)); err != nil {
			return err
		}
		if err := x11.SetState(id, "add", "below", "skip_taskbar"); err != nil {
			return err
		}
		return v.applyOpacity(v.cfg.OverlayOpacity)
	case "full":
		if err := x11.SetState(id, "remove", "below"); err != nil {
			return err
		}
		if err := errors.Join(v.mpv.Set("fullscreen", true), v.mpv.Set("ontop", true)); err != nil {
			return err
		}
		return v.applyOpacity(1)
	default: // split-left, split-right
		if err := errors.Join(v.mpv.Set("ontop", false), v.mpv.Set("fullscreen", false)); err != nil {
			return err
		}
		// wmctrl changes at most two state flags per call.
		if err := errors.Join(x11.SetState(id, "remove", "below", "fullscreen"),
			x11.SetState(id, "remove", "maximized_vert", "maximized_horz")); err != nil {
			return err
		}
		m, err := x11.Monitor(v.cfg.Screen)
		if err != nil {
			return err
		}
		w := int(float64(m.W) * v.cfg.SplitRatio)
		x := m.X
		if mode == "split-right" {
			x = m.X + m.W - w
		}
		time.Sleep(150 * time.Millisecond) // let the WM finish leaving fullscreen
		if err := x11.MoveResize(id, x, m.Y, w, m.H); err != nil {
			return err
		}
		return v.applyOpacity(1)
	}
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

func (v *viewer) overlayWindows() ([]uint32, error) {
	ws, err := x11.Windows()
	if err != nil {
		return nil, err
	}
	self, _ := x11.FindByName("^" + mpv.Title + "$")
	var ids []uint32
	for _, w := range ws {
		if w.Desktop == -1 || w.ID == self {
			continue
		}
		class := x11.Class(w.ID)
		if v.cfg.OverlayInclude != "" && !v.include.MatchString(class) {
			continue
		}
		if v.cfg.OverlayExclude != "" && v.exclude.MatchString(class) {
			continue
		}
		ids = append(ids, w.ID)
	}
	return ids, nil
}

// applyOpacity sets every overlay window to o. Callers hold v.mu.
func (v *viewer) applyOpacity(o float64) error {
	if v.x11err != nil {
		return v.x11err
	}
	ids, err := v.overlayWindows()
	if err != nil {
		return err
	}
	var errs []error
	for _, id := range ids {
		if err := x11.SetOpacity(id, o); err != nil {
			errs = append(errs, err)
		}
	}
	v.opacity = o
	if len(errs) > 0 {
		v.log.Printf("set opacity on %d window(s) failed: %v", len(errs), errors.Join(errs...))
	}
	return nil
}

// fadeNewWindows gives windows opened since the last change the current overlay opacity.
func (v *viewer) fadeNewWindows() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.opacity >= 0.999 {
		return
	}
	ids, err := v.overlayWindows()
	if err != nil {
		v.log.Printf("list windows: %v", err)
		return
	}
	for _, id := range ids {
		if !x11.HasOpacity(id) {
			if err := x11.SetOpacity(id, v.opacity); err != nil {
				v.log.Printf("fade new window: %v", err)
			}
		}
	}
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
	if err := v.applyOpacity(o); err != nil {
		return "", err
	}
	v.peekFrom = -1
	return fmt.Sprintf("opacity: %.2f", o), nil
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
	if v.peekFrom >= 0 {
		restore := v.peekFrom
		if err := v.applyOpacity(restore); err != nil {
			return "", err
		}
		v.peekFrom = -1
		return fmt.Sprintf("peek off: opacity %.2f", restore), nil
	}
	from := v.opacity
	if err := v.applyOpacity(v.cfg.PeekOpacity); err != nil {
		return "", err
	}
	v.peekFrom = from
	return fmt.Sprintf("peek on: opacity %.2f", v.cfg.PeekOpacity), nil
}

// ---- screenshots ------------------------------------------------------------------

// shot saves the current decoded frame at source resolution and returns its path.
func (v *viewer) shot() (string, error) {
	var w int
	if err := v.mpv.Get("width", &w); err != nil {
		return "", errors.New("no video from the sender yet")
	}
	if err := os.MkdirAll(v.cfg.ShotDir, 0o700); err != nil {
		return "", fmt.Errorf("create shot dir: %w", err)
	}
	path := filepath.Join(v.cfg.ShotDir, "periscope-"+time.Now().Format("20060102-150405.000")+".png")
	if _, err := v.mpv.Command("screenshot-to-file", path, "video"); err != nil {
		return "", fmt.Errorf("screenshot: %w", err)
	}
	for i := 0; i < 40; i++ {
		if st, err := os.Stat(path); err == nil && st.Size() > 0 {
			latest := filepath.Join(v.cfg.ShotDir, "latest.png")
			_ = os.Remove(latest)
			if err := os.Symlink(path, latest); err != nil {
				v.log.Printf("latest.png: %v", err)
			}
			return path, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return "", errors.New("screenshot was not written (see mpv.log)")
}
