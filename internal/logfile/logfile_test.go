package logfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRotatesPastLimit(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.log")
	l, err := Open(p, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	_, _ = l.Write([]byte("12345678\n"))
	_, _ = l.Write([]byte("abcdefgh\n"))
	old, _ := os.ReadFile(p + ".1")
	cur, _ := os.ReadFile(p)
	if string(old) != "12345678\n" || string(cur) != "abcdefgh\n" {
		t.Fatalf("old=%q cur=%q", old, cur)
	}
}
