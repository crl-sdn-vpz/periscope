package srt

import "testing"

func TestEscapeMatchesTheBashAndPowerShellEncoders(t *testing.T) {
	// Same input and output as linux/d1's urlencode (verified there by hand).
	got := Escape("aZ09-._~ /&=%+é")
	want := "aZ09-._~%20%2F%26%3D%25%2B%C3%A9"
	if got != want {
		t.Fatalf("Escape = %q, want %q", got, want)
	}
}

func TestURL(t *testing.T) {
	got := URL("caller", "192.168.1.50", 9000, 80, "pass word&x")
	want := "srt://192.168.1.50:9000?mode=caller&latency=80000&pbkeylen=16&passphrase=pass%20word%26x"
	if got != want {
		t.Fatalf("URL =\n %s\nwant\n %s", got, want)
	}
}
