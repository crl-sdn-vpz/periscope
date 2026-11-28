// Package redact keeps secrets (the passphrase, raw and URL-escaped) out of logs and status text.
package redact

import (
	"bytes"
	"io"
	"strings"
	"sync"
)

// Mask replaces each secret.
const Mask = "********"

// Redactor replaces a fixed set of secrets.
type Redactor struct{ r *strings.Replacer }

// New returns a redactor for the given secrets (empty ones are ignored).
func New(secrets ...string) Redactor {
	var pairs []string
	for _, s := range secrets {
		if s != "" {
			pairs = append(pairs, s, Mask)
		}
	}
	return Redactor{strings.NewReplacer(pairs...)}
}

// String redacts s.
func (r Redactor) String(s string) string { return r.r.Replace(s) }

// Writer redacts what passes through it, a line at a time so a secret split across two writes is
// still caught. A partial last line is held until its newline arrives (or Flush).
func (r Redactor) Writer(w io.Writer) *Writer { return &Writer{r: r, w: w} }

// Writer is the io.Writer returned by Redactor.Writer.
type Writer struct {
	mu  sync.Mutex
	r   Redactor
	w   io.Writer
	buf []byte
}

func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	if i := bytes.LastIndexByte(w.buf, '\n'); i >= 0 {
		if _, err := io.WriteString(w.w, w.r.String(string(w.buf[:i+1]))); err != nil {
			return 0, err
		}
		w.buf = append(w.buf[:0], w.buf[i+1:]...)
	}
	if len(w.buf) > 64<<10 { // a runaway line without newline: don't buffer forever
		if _, err := io.WriteString(w.w, w.r.String(string(w.buf))); err != nil {
			return 0, err
		}
		w.buf = w.buf[:0]
	}
	return len(p), nil
}

// Flush writes a held partial line.
func (w *Writer) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) == 0 {
		return nil
	}
	_, err := io.WriteString(w.w, w.r.String(string(w.buf)))
	w.buf = w.buf[:0]
	return err
}
