// Package menu is a full-screen terminal menu driven by arrows/j/k + Enter or one key per item.
package menu

import (
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/crl-sdn-vpz/periscope/internal/ui"
)

// Item is one menu entry; items with the same Section are listed under one heading.
type Item struct {
	Key     byte
	Label   string
	Section string
	Run     func() (string, error)
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
	case string(b) == "\x1b" || string(b) == "q" || b[0] == 3 || b[0] == 4: // Esc, q, Ctrl+C, Ctrl+D
		return keyQuit, 0
	case b[0] == 0x1b:
		return keyNone, 0 // other escape sequences
	}
	return keyChar, b[0]
}

// Run shows the menu until the user quits. header is redrawn every refresh (e.g. live status).
func Run(title, subtitle string, items []Item, header func() string, refresh time.Duration) error {
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

	type result struct {
		out string
		err error
	}
	sel, msg := 0, ""
	busy := ""                   // label of the action in progress
	done := make(chan result, 1) // actions run in the background so quit keys always work
	tick := time.NewTicker(refresh)
	defer tick.Stop()
	for {
		status := header()
		if busy != "" {
			draw(title, subtitle, items, sel, status, ui.Style(ui.Out, ui.Dim, busy+" ..."))
		} else {
			draw(title, subtitle, items, sel, status, msg)
		}
		var b []byte
		select {
		case k, ok := <-keys:
			if !ok {
				return nil
			}
			b = k
		case r := <-done:
			busy, msg = "", ui.OK(ui.Out, r.out)
			if r.err != nil {
				msg = ui.Fail(ui.Out, ui.Style(ui.Out, ui.Red, strings.TrimSpace(r.err.Error())))
				if r.out != "" {
					msg = r.out + "\n" + msg
				}
			}
			continue
		case <-tick.C:
			continue
		}
		kind, ch := decode(b)
		if kind == keyQuit {
			return nil
		}
		if busy != "" {
			continue // one action at a time
		}
		pick := -1
		switch kind {
		case keyUp:
			sel = (sel + len(items) - 1) % len(items)
		case keyDown:
			sel = (sel + 1) % len(items)
		case keyEnter:
			pick = sel
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
		busy = items[pick].Label
		go func(run func() (string, error)) {
			out, err := run()
			done <- result{out, err}
		}(items[pick].Run)
	}
}

func draw(title, subtitle string, items []Item, sel int, status, msg string) {
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width, height = 80, 24
	}
	var lines []string
	if height >= 34 {
		lines = append(lines, strings.Split(ui.Banner(ui.Out, title, subtitle), "\n")...)
	} else {
		lines = append(lines, " "+ui.Style(ui.Out, ui.Amber, "◉")+" "+ui.Style(ui.Out, ui.Bold, title)+"  "+ui.Style(ui.Out, ui.Dim, subtitle))
	}
	lines = append(lines, "")
	for _, l := range ui.Box(ui.Out, "status", strings.Split(status, "\n"), min(width-6, 70)) {
		lines = append(lines, " "+l)
	}
	lines = append(lines, "")
	lines = append(lines, layout(items, sel, width)...)
	lines = append(lines, "", " "+ui.Style(ui.Out, ui.Dim, "↑↓ / j k move · enter or a key runs · q / esc quits"))
	if msg != "" {
		lines = append(lines, "")
		for _, l := range strings.Split(msg, "\n") {
			lines = append(lines, " "+l)
		}
	}
	// Raw mode doesn't translate \n, so every line ends in \r\n; \x1b[K clears leftovers.
	fmt.Print("\x1b[H" + strings.Join(lines, "\x1b[K\r\n") + "\x1b[K\x1b[J")
}

// layout renders the items grouped by section, in two columns when the terminal is wide enough.
func layout(items []Item, sel, width int) []string {
	type block struct{ lines []string }
	var blocks []block
	for i, it := range items {
		if i == 0 || it.Section != items[i-1].Section {
			blocks = append(blocks, block{lines: []string{ui.Style(ui.Out, ui.Teal+";1", strings.ToUpper(it.Section))}})
		}
		label := fmt.Sprintf("%s  %s", ui.Style(ui.Out, ui.Amber+";1", string(it.Key)), it.Label)
		if i == sel {
			label = ui.Style(ui.Out, ui.Amber, "❯") + " " + ui.Style(ui.Out, ui.Inverse, " "+string(it.Key)+"  "+it.Label+" ")
		} else {
			label = "  " + label
		}
		b := &blocks[len(blocks)-1]
		b.lines = append(b.lines, label)
	}
	col := func(bs []block) []string {
		var out []string
		for i, b := range bs {
			if i > 0 {
				out = append(out, "")
			}
			out = append(out, b.lines...)
		}
		return out
	}
	const colWidth = 36
	if width < 2*colWidth+4 {
		var out []string
		for _, l := range col(blocks) {
			out = append(out, "  "+l)
		}
		return out
	}
	// Split the sections so both columns get about the same number of rows.
	total := 0
	for _, b := range blocks {
		total += len(b.lines) + 1
	}
	split, rows := len(blocks), 0
	for i, b := range blocks {
		if rows >= total/2 {
			split = i
			break
		}
		rows += len(b.lines) + 1
	}
	left, right := col(blocks[:split]), col(blocks[split:])
	var out []string
	for i := 0; i < max(len(left), len(right)); i++ {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		out = append(out, "  "+ui.Pad(l, colWidth)+r)
	}
	return out
}
