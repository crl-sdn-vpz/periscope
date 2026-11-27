package control

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRoundTripAndSingleInstance(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "c.sock")
	ln, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Serve(ctx, ln, func(_ context.Context, r Request) (string, error) {
		if r.Cmd == "fail" {
			return "", errors.New("nope")
		}
		return r.Cmd + ":" + strings.Join(r.Args, ","), nil
	})

	if out, err := Call(sock, Request{Cmd: "mode", Args: []string{"full"}}, time.Second); err != nil || out != "mode:full" {
		t.Fatalf("Call = %q, %v", out, err)
	}
	if _, err := Call(sock, Request{Cmd: "fail"}, time.Second); err == nil || err.Error() != "nope" {
		t.Fatalf("want handler error, got %v", err)
	}
	if _, err := Listen(sock); !errors.Is(err, ErrRunning) {
		t.Fatalf("second Listen = %v, want ErrRunning", err)
	}
	cancel()
	time.Sleep(50 * time.Millisecond)
	if _, err := Call(sock, Request{Cmd: "x"}, time.Second); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("after stop: %v, want ErrNotRunning", err)
	}
	if ln2, err := Listen(sock); err != nil {
		t.Fatalf("stale socket not replaced: %v", err)
	} else {
		_ = ln2.Close()
	}
}
