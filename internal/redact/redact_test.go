package redact

import (
	"strings"
	"testing"
)

func TestStringAndSplitWrites(t *testing.T) {
	r := New("s3cret pass", "s3cret%20pass", "")
	if got := r.String("url?passphrase=s3cret%20pass&x"); got != "url?passphrase=********&x" {
		t.Fatalf("String = %q", got)
	}
	var out strings.Builder
	w := r.Writer(&out)
	for _, chunk := range []string{"opening srt://h?passphrase=s3cr", "et pass ok\nnext ", "line s3cret pass"} {
		if _, err := w.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	_ = w.Flush()
	if strings.Contains(out.String(), "s3cret") || out.String() != "opening srt://h?passphrase=******** ok\nnext line ********" {
		t.Fatalf("writer output %q", out.String())
	}
}
