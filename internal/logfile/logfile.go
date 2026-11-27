// Package logfile is an append-only log file that rotates to <name>.1 when it passes a size limit.
package logfile

import (
	"fmt"
	"os"
	"sync"
)

// File is safe for concurrent writers.
type File struct {
	mu   sync.Mutex
	path string
	max  int64
	f    *os.File
	size int64
}

// Open opens (or creates) path for appending; it rotates once it grows past max bytes.
func Open(path string, max int64) (*File, error) {
	l := &File{path: path, max: max}
	if err := l.open(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *File) open() error {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open log %s: %w", l.path, err)
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("stat log %s: %w", l.path, err)
	}
	l.f, l.size = f, st.Size()
	return nil
}

func (l *File) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.size+int64(len(p)) > l.max && l.size > 0 {
		_ = l.f.Close()
		if err := os.Rename(l.path, l.path+".1"); err != nil {
			return 0, fmt.Errorf("rotate log: %w", err)
		}
		if err := l.open(); err != nil {
			return 0, err
		}
	}
	n, err := l.f.Write(p)
	l.size += int64(n)
	return n, err
}

// Close closes the file.
func (l *File) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.f.Close()
}
