// Package menu is a full-screen terminal menu driven by arrows/j/k + Enter or one key per item.
package menu

import (
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

// Item is one menu entry. A nil Run quits the menu.
type Item struct {
	Key   byte
	Label string
	Run   func() (string, error)
}

type keyKind int

const (
	keyNone keyKind = iota
	keyUp
	keyDown
	keyEnter
	keyQuit
	keyChar
)

// decode turns one read from a raw terminal into a key.
func decode(b []byte) (keyKind, byte) {
	switch {
	case len(b) == 0:
		return keyNone, 0
	case string(b) == "\x1b[A" || string(b) == "\x1bOA" || string(b) == "k":
		return keyUp, 0
	case string(b) == "\x1b[B" || string(b) == "\x1bOB" || string(b) == "j":
		return keyDown, 0
	case b[0] == '\r' || b[0] == '\n':
		return keyEnter, 0
	case string(b) == "\x1b" || b[0] == 3 || b[0] == 4: // Esc, Ctrl+C, Ctrl+D
		return keyQuit, 0
	case b[0] == 0x1b:
		return keyNone, 0 // other escape sequences
	}
	return keyChar, b[0]
}

// Run shows the menu until the user quits. header is redrawn every refresh (e.g. live status).
func Run(title string, items []Item, header func() string, refresh time.Duration) error {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("the menu needs a terminal")
	}
	old, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("raw terminal: %w", err)
	}
	fmt.Print("\x1b[?1049h\x1b[?25l") // alternate screen, hide cursor
	defer func() {
		fmt.Print("\x1b[?25h\x1b[?1049l")
		_ = term.Restore(fd, old)
	}()

	keys := make(chan []byte)
	go func() {
		buf := make([]byte, 16)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				close(keys)
				return
			}
			keys <- append([]byte(nil), buf[:n]...)
		}
	}()

	sel, msg := 0, ""
	tick := time.NewTicker(refresh)
	defer tick.Stop()
	for {
		draw(title, items, sel, header(), msg)
		var b []byte
		select {
		case k, ok := <-keys:
			if !ok {
				return nil
			}
			b = k
		case <-tick.C:
			continue
		}
		kind, ch := decode(b)
		pick := -1
		switch kind {
		case keyUp:
			sel = (sel + len(items) - 1) % len(items)
		case keyDown:
			sel = (sel + 1) % len(items)
		case keyEnter:
			pick = sel
		case keyQuit:
			return nil
		case keyChar:
			for i, it := range items {
				if it.Key == ch {
					pick, sel = i, i
				}
			}
		}
		if pick < 0 {
			continue
		}
		if items[pick].Run == nil {
			return nil
		}
		out, err := items[pick].Run()
		msg = out
		if err != nil {
			msg = strings.TrimSpace(out + "\nerror: " + err.Error())
		}
	}
}

func draw(title string, items []Item, sel int, header, msg string) {
	var b strings.Builder
	b.WriteString("\x1b[H\x1b[J")
	lines := []string{title, "", header, ""}
	for i, it := range items {
		if i == sel {
			lines = append(lines, fmt.Sprintf("  \x1b[7m %c  %-26s\x1b[0m", it.Key, it.Label))
		} else {
			lines = append(lines, fmt.Sprintf("   %c  %s", it.Key, it.Label))
		}
	}
	lines = append(lines, "", "  ↑/↓ or j/k move · Enter or a key runs · q or Esc quits")
	if msg != "" {
		lines = append(lines, "", msg)
	}
	// Raw mode doesn't translate \n, so every line ends in \r\n.
	b.WriteString(strings.ReplaceAll(strings.Join(lines, "\n"), "\n", "\r\n"))
	fmt.Print(b.String())
}
