// Package sender runs `periscope send`: capture → ffmpeg → SRT caller, restarted with backoff,
// with a control socket for status and stop.
package sender

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/crl-sdn-vpz/periscope/internal/config"
	"github.com/crl-sdn-vpz/periscope/internal/control"
	"github.com/crl-sdn-vpz/periscope/internal/ffargs"
	"github.com/crl-sdn-vpz/periscope/internal/logfile"
	"github.com/crl-sdn-vpz/periscope/internal/supervise"
)

// SocketName is the sender's control socket inside the state dir.
const SocketName = "send.sock"

// A run that lasts this long has connected; shorter runs mean the viewer isn't there yet.
const connectedAfter = 5 * time.Second

type state struct {
	mu       sync.Mutex
	started  time.Time // current ffmpeg run; zero while waiting to retry
	lastErr  string
	retryAt  time.Time
	restarts int
}

// Run streams until ctx is done or a "stop" request arrives. Logs go to stateDir/sender.log and
// ffmpeg.log, and to logOut too (stderr when run in a terminal).
func Run(ctx context.Context, c config.Config, stateDir string, logOut io.Writer) error {
	if err := c.ValidateSend(); err != nil {
		return err
	}
	ln, err := control.Listen(filepath.Join(stateDir, SocketName))
	if errors.Is(err, control.ErrRunning) {
		return errors.New("a sender is already running (periscope send status)")
	}
	if err != nil {
		return err
	}
	senderLog, err := logfile.Open(filepath.Join(stateDir, "sender.log"), 1<<20)
	if err != nil {
		return err
	}
	defer senderLog.Close()
	ffLog, err := logfile.Open(filepath.Join(stateDir, "ffmpeg.log"), 1<<20)
	if err != nil {
		return err
	}
	defer ffLog.Close()
	lg := log.New(io.MultiWriter(senderLog, logOut), "", log.LstdFlags)

	ffmpeg, err := ResolveFfmpeg(c.Send.Ffmpeg)
	if err != nil {
		lg.Print(err)
		return err
	}
	args, err := ffargs.Build(c, runtime.GOOS)
	if err != nil {
		lg.Print(err)
		return err
	}
	lg.Printf("sender started (pid %d), ffmpeg %s", os.Getpid(), ffmpeg)
	lg.Printf("target srt://%s:%d monitor=%d fps=%d encoder=%s capture=%s",
		c.Send.TargetHost, c.Port, c.Send.Monitor, c.Send.Fps, c.Send.Encoder, c.Send.Capture)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	st := &state{}
	supErr := make(chan error, 1)
	go func() {
		supErr <- supervise.Run(ctx, supervise.Options{
			Name:       "ffmpeg",
			Command:    func(ctx context.Context) *exec.Cmd { return exec.CommandContext(ctx, ffmpeg, args...) },
			Stderr:     ffLog,
			Log:        lg,
			MinBackoff: 2 * time.Second, MaxBackoff: 15 * time.Second, ResetAfter: 20 * time.Second,
			OnStart: func() {
				st.mu.Lock()
				st.started = time.Now()
				st.mu.Unlock()
			},
			OnExit: func(err error, tail string, retryIn time.Duration) {
				st.mu.Lock()
				st.started, st.lastErr, st.retryAt = time.Time{}, strings.TrimSpace(fmt.Sprint(err, " ", tail)), time.Now().Add(retryIn)
				st.restarts++
				st.mu.Unlock()
			},
		})
		cancel()
	}()

	control.Serve(ctx, ln, func(_ context.Context, r control.Request) (string, error) {
		switch r.Cmd {
		case "status":
			return st.describe(c), nil
		case "pid":
			return fmt.Sprint(os.Getpid()), nil
		case "stop":
			go func() { time.Sleep(50 * time.Millisecond); cancel() }()
			return "sender stopping", nil
		}
		return "", fmt.Errorf("unknown sender command %q", r.Cmd)
	})
	err = <-supErr
	lg.Print("sender stopped")
	return err
}

func (s *state) describe(c config.Config) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	target := fmt.Sprintf("%s:%d", c.Send.TargetHost, c.Port)
	switch {
	case !s.started.IsZero() && time.Since(s.started) >= connectedAfter:
		return fmt.Sprintf("sender : streaming to %s for %s (pid %d)", target, time.Since(s.started).Round(time.Second), os.Getpid())
	case !s.started.IsZero():
		return fmt.Sprintf("sender : connecting to %s (pid %d)", target, os.Getpid())
	default:
		return fmt.Sprintf("sender : waiting for the viewer at %s, retry in %s (pid %d)\nlast   : %s",
			target, time.Until(s.retryAt).Round(time.Second), os.Getpid(), s.lastErr)
	}
}

// ResolveFfmpeg finds ffmpeg: the configured name or path, then (Windows) winget's usual locations.
func ResolveFfmpeg(configured string) (string, error) {
	if p, err := exec.LookPath(configured); err == nil {
		return p, nil
	}
	var candidates []string
	if runtime.GOOS == "windows" {
		local := os.Getenv("LOCALAPPDATA")
		candidates = append(candidates,
			filepath.Join(local, "Microsoft", "WinGet", "Links", "ffmpeg.exe"),
			`C:\ffmpeg\bin\ffmpeg.exe`,
			filepath.Join(os.Getenv("ProgramFiles"), "ffmpeg", "bin", "ffmpeg.exe"))
		// winget can install without a Links shim; its package dir holds ffmpeg-<ver>/bin/ffmpeg.exe.
		matches, _ := filepath.Glob(filepath.Join(local, "Microsoft", "WinGet", "Packages", "Gyan.FFmpeg*", "*", "bin", "ffmpeg.exe"))
		candidates = append(candidates, matches...)
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	hint := "install ffmpeg with your package manager"
	if runtime.GOOS == "windows" {
		hint = "install it with: winget install Gyan.FFmpeg"
	}
	return "", fmt.Errorf("ffmpeg not found (%q); %s, or set send.ffmpeg in the config", configured, hint)
}
