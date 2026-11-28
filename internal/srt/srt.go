// Package srt builds the SRT URLs both roles hand to ffmpeg/mpv (libsrt via libavformat).
package srt

import (
	"fmt"
	"strings"
)

// Base returns an srt:// URL without the passphrase: AES-128, and encryption enforced, so a peer
// without the passphrase (or with another one) is refused rather than talked to in clear.
// mode is "listener" (viewer) or "caller" (sender).
func Base(mode, host string, port, latencyMs int) string {
	return fmt.Sprintf("srt://%s:%d?mode=%s&latency=%d&pbkeylen=16&enforced_encryption=1",
		host, port, mode, latencyMs*1000)
}

// URL is Base with the passphrase in it. It ends up in the process's command line: prefer passing
// the passphrase out of band where the program allows it (the viewer gives mpv an options file).
func URL(mode, host string, port, latencyMs int, passphrase string) string {
	return Base(mode, host, port, latencyMs) + "&passphrase=" + Escape(passphrase)
}

// Escape percent-encodes everything except RFC 3986 unreserved characters, like .NET's
// Uri.EscapeDataString. url.QueryEscape is not used because it turns spaces into '+'.
func Escape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || strings.IndexByte("-._~", c) >= 0 {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}
