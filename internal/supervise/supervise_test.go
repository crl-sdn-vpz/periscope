package supervise

import (
	"context"
	"io"
	"log"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"
)

func TestRestartsUntilCancelled(t *testing.T) {
	var runs atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		done <- Run(ctx, Options{
			Name: "false",
			Command: func(ctx context.Context) *exec.Cmd {
				if runs.Add(1) == 3 {
					cancel()
				}
				return exec.CommandContext(ctx, "sh", "-c", "echo boom >&2; exit 1")
			},
			Log:        log.New(io.Discard, "", 0),
			MinBackoff: time.Millisecond, MaxBackoff: 4 * time.Millisecond, ResetAfter: time.Hour,
		})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	if n := runs.Load(); n != 3 {
		t.Fatalf("runs = %d, want 3", n)
	}
}

func TestMissingBinaryFails(t *testing.T) {
	err := Run(context.Background(), Options{
		Command: func(ctx context.Context) *exec.Cmd { return exec.CommandContext(ctx, "/nonexistent/ffmpeg") },
		Log:     log.New(io.Discard, "", 0),
	})
	if err == nil {
		t.Fatal("want an error for a missing binary")
	}
}

func TestTailWriterKeepsLastLines(t *testing.T) {
	w := &tailWriter{max: 2}
	_, _ = w.Write([]byte("a\nb\n"))
	_, _ = w.Write([]byte("c\npartial"))
	if got := w.String(); got != "b | c" {
		t.Fatalf("tail = %q", got)
	}
}

// A grandchild that inherits stderr (e.g. the real ffmpeg behind a launcher shim) must not keep Run
// from returning after cancel: exec's Wait would block until every writer closes the pipe.
func TestCancelDoesNotWaitForGrandchildren(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		done <- Run(ctx, Options{
			Name:       "shim",
			Command:    func(ctx context.Context) *exec.Cmd { return exec.CommandContext(ctx, "sh", "-c", "sleep 30 & wait") },
			Log:        log.New(io.Discard, "", 0),
			MinBackoff: time.Second, MaxBackoff: time.Second, ResetAfter: time.Hour,
		})
	}()
	time.Sleep(300 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run blocked on a grandchild holding stderr")
	}
}
