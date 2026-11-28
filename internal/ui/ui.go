// Package ui styles terminal output: colors, the logo, status lines and boxes. Styling is applied only
// when the stream is a terminal and NO_COLOR is unset, so pipes, logs and tests get plain text.
package ui

import (
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

// Out and Err report whether stdout / stderr get styled output.
var (
	Out = styled(os.Stdout)
	Err = styled(os.Stderr)
)

func styled(f *os.File) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" || !term.IsTerminal(int(f.Fd())) {
		return false
	}
	return enableANSI(f)
}

// Style wraps s in an SGR sequence when on is true.
func Style(on bool, code, s string) string {
	if !on || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// SGR codes used across the CLI.
const (
	Bold    = "1"
	Dim     = "2"
	Red     = "31"
	Green   = "32"
	Yellow  = "33"
	Blue    = "34"
	Magenta = "35"
	Cyan    = "36"
	Teal    = "38;5;43"  // the icon's tube
	Amber   = "38;5;214" // the icon's lens
	Inverse = "7"
)

// Logo is the icon in text: a screen with a periscope looking out of it.
func Logo(on bool) []string {
	t := func(s string) string { return Style(on, Teal, s) }
	a := func(s string) string { return Style(on, Amber, s) }
	return []string{
		"   " + t("╭───") + a("◉") + " " + a("))"),
		"   " + t("│"),
		" " + t("╭─┴──────────╮"),
		" " + t("│") + Style(on, Dim, " ══════ ═══ ") + t("│"),
		" " + t("╰────────────╯"),
	}
}

// Banner puts the logo next to a title and a subtitle.
func Banner(on bool, title, subtitle string) string {
	logo := Logo(on)
	right := []string{"", Style(on, Bold, title), Style(on, Dim, subtitle), "", ""}
	var b strings.Builder
	for i, l := range logo {
		b.WriteString(Pad(l, 18) + right[i])
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// Width is the printed width of s, ignoring escape sequences.
func Width(s string) int { return utf8.RuneCountInString(ansi.ReplaceAllString(s, "")) }

// Pad right-pads s with spaces to width w (escape sequences don't count).
func Pad(s string, w int) string {
	if n := Width(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

// Box frames lines with a rounded border and an optional title, at least w wide inside.
func Box(on bool, title string, lines []string, w int) []string {
	for _, l := range lines {
		w = max(w, Width(l))
	}
	frame := func(s string) string { return Style(on, Dim, s) }
	label := "" // "─ title " inside the top border
	if title != "" {
		label = frame("─ ") + Style(on, Bold, title) + " "
		w = max(w, Width(label)-2) // the title must fit in the top border
	}
	top := frame("╭") + label + frame(strings.Repeat("─", max(0, w+2-Width(label)))) + frame("╮")
	out := []string{top}
	for _, l := range lines {
		out = append(out, frame("│ ")+Pad(l, w)+frame(" │"))
	}
	return append(out, frame("╰"+strings.Repeat("─", w+2)+"╯"))
}

// state words → color, for status lines.
var (
	good = regexp.MustCompile(`\b(running|receiving|streaming|faded to)\b`)
	wait = regexp.MustCompile(`\b(waiting|connecting|stopped|stopping|retry|solid)\b`)
	bad  = regexp.MustCompile(`(?i)\b(error|failed|not answering|not running|not connected|skipped)\b`)
)

// Dot is a colored state marker for a status value.
func Dot(on bool, value string) string {
	switch {
	case bad.MatchString(value):
		return Style(on, Red, "●")
	case good.MatchString(value):
		return Style(on, Green, "●")
	case wait.MatchString(value):
		return Style(on, Yellow, "●")
	}
	return Style(on, Dim, "·")
}

// Status restyles "label : value" lines (as `periscope status` prints them) with a state dot and
// a bold label. Other lines pass through. Plain text is returned unchanged when on is false.
func Status(on bool, text string) string {
	if !on {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		label, value, ok := strings.Cut(l, ":")
		label = strings.TrimSpace(label)
		if !ok || label == "" || len(label) > 8 || strings.Contains(label, " ") {
			continue
		}
		value = strings.TrimSpace(value)
		lines[i] = Dot(on, value) + " " + Style(on, Bold, Pad(label, 7)) + " " + value
	}
	return strings.Join(lines, "\n")
}

// OK, Warn and Fail prefix a message with a colored mark.
func OK(on bool, s string) string   { return Style(on, Green, "✓") + " " + s }
func Warn(on bool, s string) string { return Style(on, Yellow, "!") + " " + s }
func Fail(on bool, s string) string { return Style(on, Red, "✗") + " " + s }
