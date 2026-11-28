package main

import (
	"strings"

	"github.com/crl-sdn-vpz/periscope/internal/ui"
)

// style dresses a command's output for a terminal. Pipes and NO_COLOR get the plain text, which
// scripts and tests parse.
func style(cmd string, rest []string, out string) string {
	if !ui.Out || out == "" {
		return out
	}
	switch cmd {
	case "status":
		return ui.Status(true, out)
	case "send":
		if len(rest) > 0 && rest[0] == "status" {
			return ui.Status(true, out)
		}
		return ui.OK(true, out)
	case "screens":
		return strings.ReplaceAll(out, "<- feed", ui.Style(true, ui.Green+";1", "◀ feed"))
	case "windows":
		return styleWindows(out)
	case "pair":
		return stylePair(out)
	case "help", "-h", "--help":
		return styleUsage(out)
	case "logs":
		return out // already colored per line
	case "uninstall":
		if out == "periscope is uninstalled" {
			return ui.OK(true, out)
		}
		return out
	case "view", "start", "mode", "peek", "opacity", "screen", "stop", "toggle", "shot", "snap", "bind-gnome":
		return ui.OK(true, out)
	case "config":
		if len(rest) > 0 && (rest[0] == "init" || rest[0] == "set") {
			return ui.OK(true, out)
		}
	}
	return out
}

func styleUsage(out string) string {
	var b strings.Builder
	b.WriteString(ui.Banner(true, "periscope "+version, "see another screen through your windows") + "\n")
	for _, l := range strings.Split(out, "\n")[1:] { // the first line is replaced by the banner
		switch {
		case strings.HasSuffix(l, ":") && !strings.HasPrefix(l, " "):
			l = ui.Style(true, ui.Teal+";1", l)
		case strings.HasPrefix(l, "  periscope "):
			cmd, desc := l, ""
			if i := strings.Index(strings.TrimLeft(l, " "), "  "); i > 0 {
				cut := len(l) - len(strings.TrimLeft(l, " ")) + i
				cmd, desc = l[:cut], l[cut:]
			}
			l = ui.Style(true, ui.Bold, cmd) + ui.Style(true, ui.Dim, desc)
		case strings.HasPrefix(l, "Config:"):
			l = ui.Style(true, ui.Dim, l)
		}
		b.WriteString(l + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func styleWindows(out string) string {
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		switch {
		case i < 2:
			lines[i] = ui.Style(true, ui.Bold, l)
		case strings.Contains(l, "faded to"):
			lines[i] = strings.Replace(l, "faded to", ui.Style(true, ui.Green, "faded to"), 1)
		case strings.Contains(l, "skipped:"):
			lines[i] = ui.Style(true, ui.Yellow, l)
		case strings.Contains(l, "solid:"):
			lines[i] = ui.Style(true, ui.Dim, l)
		}
	}
	return strings.Join(lines, "\n")
}

func stylePair(out string) string {
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		t := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(t, "powershell ") || strings.HasPrefix(t, "curl "):
			lines[i] = "  " + ui.Style(true, ui.Amber, t)
		case strings.HasSuffix(t, ":"):
			lines[i] = ui.Style(true, ui.Teal+";1", l)
		case strings.HasPrefix(t, "Show this again"):
			lines[i] = ui.Style(true, ui.Dim, l)
		}
	}
	return strings.Join(lines, "\n")
}

// colorLogLine highlights problems and milestones in a log line.
func colorLogLine(l string) string {
	low := strings.ToLower(l)
	switch {
	case !ui.Out:
		return l
	case strings.Contains(low, "error") || strings.Contains(low, "failed") || strings.Contains(low, "fatal"):
		return ui.Style(true, ui.Red, l)
	case strings.Contains(low, "exited") || strings.Contains(low, "retrying") || strings.Contains(low, "warn"):
		return ui.Style(true, ui.Yellow, l)
	case strings.Contains(low, "started") || strings.Contains(low, "listening") || strings.Contains(low, "stopped"):
		return ui.Style(true, ui.Green, l)
	}
	return l
}
