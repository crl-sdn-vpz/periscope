package clipboard

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

// xclip keeps a background process serving the selection; copying must not wait for it.
func TestCopyTextReturnsWhileXclipKeepsServing(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("needs an X display (run under xvfb-run)")
	}
	if _, err := exec.LookPath("xclip"); err != nil {
		t.Skip("xclip not installed")
	}
	t.Setenv("XDG_SESSION_TYPE", "x11")
	done := make(chan error)
	go func() { done <- CopyText("periscope") }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("CopyText blocked on xclip's background process")
	}
	out, err := exec.Command("xclip", "-selection", "clipboard", "-o").Output()
	if err != nil || string(out) != "periscope" {
		t.Fatalf("clipboard = %q, %v", out, err)
	}
}
