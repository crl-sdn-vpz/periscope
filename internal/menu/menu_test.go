package menu

import (
	"strings"
	"testing"
)

func TestDecode(t *testing.T) {
	cases := []struct {
		in   string
		kind keyKind
		ch   byte
	}{
		{"\x1b[A", keyUp, 0}, {"k", keyUp, 0}, {"\x1bOB", keyDown, 0}, {"j", keyDown, 0},
		{"\r", keyEnter, 0}, {"\x1b", keyQuit, 0}, {"\x03", keyQuit, 0}, {"q", keyQuit, 0},
		{"\x1b[C", keyNone, 0}, {"s", keyChar, 's'}, {"0", keyChar, '0'},
	}
	for _, c := range cases {
		if k, ch := decode([]byte(c.in)); k != c.kind || ch != c.ch {
			t.Errorf("decode(%q) = %v,%q want %v,%q", c.in, k, ch, c.kind, c.ch)
		}
	}
}

func TestLayoutColumns(t *testing.T) {
	items := []Item{{Key: 'v', Label: "Start", Section: "Viewer"}, {Key: 'x', Label: "Stop", Section: "Viewer"},
		{Key: 'o', Label: "Overlay", Section: "Layout"}, {Key: 's', Label: "Shot", Section: "Capture"}}
	wide := layout(items, 0, 120)
	narrow := layout(items, 0, 60)
	if len(wide) >= len(narrow) {
		t.Fatalf("wide terminals should use two columns: %d vs %d rows", len(wide), len(narrow))
	}
	if !strings.Contains(strings.Join(narrow, "\n"), "❯") {
		t.Fatal("selection marker missing")
	}
}
