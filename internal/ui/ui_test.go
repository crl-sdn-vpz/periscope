package ui

import (
	"strings"
	"testing"
)

func TestPlainWhenOff(t *testing.T) {
	in := "viewer : running (mode: overlay)\nstream : waiting for the sender on port 9000"
	if Status(false, in) != in || Style(false, Red, "x") != "x" {
		t.Fatal("styling must be a no-op when off")
	}
}

func TestStatusStyling(t *testing.T) {
	out := Status(true, "viewer : running (mode: overlay)\nstream : mpv not answering (x)\nplain line")
	lines := strings.Split(out, "\n")
	if !strings.Contains(lines[0], "\x1b[32m●") || !strings.Contains(lines[0], "running (mode: overlay)") {
		t.Errorf("running line: %q", lines[0])
	}
	if !strings.Contains(lines[1], "\x1b[31m●") {
		t.Errorf("error line: %q", lines[1])
	}
	if lines[2] != "plain line" {
		t.Errorf("other lines pass through: %q", lines[2])
	}
}

func TestBoxAndWidth(t *testing.T) {
	for _, title := range []string{"", "status", "a much longer title"} {
		b := Box(true, title, []string{Style(true, Red, "ab"), "abcd"}, 6)
		for _, l := range b {
			if Width(l) != Width(b[0]) || Width(l) < 10 {
				t.Errorf("title %q: width %d vs %d: %q", title, Width(l), Width(b[0]), l)
			}
		}
	}
	if Width(Logo(true)[0]) != Width(Logo(false)[0]) {
		t.Error("logo width must not depend on color")
	}
}
