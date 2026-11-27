package menu

import "testing"

func TestDecode(t *testing.T) {
	cases := []struct {
		in   string
		kind keyKind
		ch   byte
	}{
		{"\x1b[A", keyUp, 0}, {"k", keyUp, 0}, {"\x1bOB", keyDown, 0}, {"j", keyDown, 0},
		{"\r", keyEnter, 0}, {"\x1b", keyQuit, 0}, {"\x03", keyQuit, 0},
		{"\x1b[C", keyNone, 0}, {"s", keyChar, 's'}, {"0", keyChar, '0'},
	}
	for _, c := range cases {
		if k, ch := decode([]byte(c.in)); k != c.kind || ch != c.ch {
			t.Errorf("decode(%q) = %v,%q want %v,%q", c.in, k, ch, c.kind, c.ch)
		}
	}
}
