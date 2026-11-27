// Package supervise keeps a child process (ffmpeg, mpv) running, restarting it with backoff.
package supervise

import (
	"context"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/crl-sdn-vpz/periscope/internal/proc"
)

// Options configure Run.
type Options struct {
	Name string
	// Command returns a fresh, unstarted command for each run. It is killed when ctx ends.
	Command func(ctx context.Context) *exec.Cmd
	// Stderr receives the child's stderr (nil discards it). Its last lines are also logged on exit.
	Stderr io.Writer
	Log    *log.Logger
	// MinBackoff doubles after each short run up to MaxBackoff; a run longer than ResetAfter resets it.
	MinBackoff, MaxBackoff, ResetAfter time.Duration
	// OnStart and OnExit (optional) report each run, e.g. for a status command.
	OnStart func()
	OnExit  func(err error, stderrTail string, retryIn time.Duration)
}

// Run restarts the command until ctx is done. It returns only then (or if the command can't be started
// at all, e.g. a missing binary).
func Run(ctx context.Context, o Options) error {
	backoff := o.MinBackoff
	for ctx.Err() == nil {
		tail := &tailWriter{max: 2}
		cmd := o.Command(ctx)
		// After cancel, stop waiting for the stderr pipe once the child is dead: a grandchild that
		// inherited it (the real ffmpeg behind a launcher shim) would otherwise block Wait forever.
		cmd.WaitDelay = 2 * time.Second
		if o.Stderr != nil {
			cmd.Stderr = io.MultiWriter(o.Stderr, tail)
		} else {
			cmd.Stderr = tail
		}
		started := time.Now()
		if err := proc.Start(cmd); err != nil {
			if ctx.Err() != nil {
				return nil // cancelled between runs
			}
			return err
		}
		if o.OnStart != nil {
			o.OnStart()
		}
		err := cmd.Wait()
		if ctx.Err() != nil {
			return nil
		}
		ran := time.Since(started)
		if ran > o.ResetAfter {
			backoff = o.MinBackoff
		}
		o.Log.Printf("%s exited (%v, ran %s); retrying in %s. %s", o.Name, err, ran.Round(time.Second), backoff, tail.String())
		if o.OnExit != nil {
			o.OnExit(err, tail.String(), backoff)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, o.MaxBackoff)
	}
	return nil
}

// tailWriter keeps the last max lines written to it.
type tailWriter struct {
	mu    sync.Mutex
	max   int
	lines []string
	part  string
}

func (t *tailWriter) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parts := strings.Split(t.part+string(p), "\n")
	t.part = parts[len(parts)-1]
	for _, l := range parts[:len(parts)-1] {
		if l = strings.TrimSpace(l); l != "" {
			t.lines = append(t.lines, l)
		}
	}
	if len(t.lines) > t.max {
		t.lines = t.lines[len(t.lines)-t.max:]
	}
	return len(p), nil
}

func (t *tailWriter) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.Join(t.lines, " | ")
}
