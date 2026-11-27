// Package srt builds the SRT URLs both roles hand to ffmpeg/mpv (libsrt via libavformat).
package srt

import (
	"fmt"
	"strings"
)

// URL returns an srt:// URL with AES-128 encryption. mode is "listener" (viewer) or "caller" (sender).
func URL(mode, host string, port, latencyMs int, passphrase string) string {
	return fmt.Sprintf("srt://%s:%d?mode=%s&latency=%d&pbkeylen=16&passphrase=%s",
		host, port, mode, latencyMs*1000, Escape(passphrase))
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
