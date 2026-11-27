package mpv

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"
)

// fakeMpv answers like mpv: an unrelated event first, then the reply carrying the request_id.
func fakeMpv(t *testing.T, answer func(cmd []any) (string, any)) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "mpv.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				var req struct {
					Command   []any `json:"command"`
					RequestID int64 `json:"request_id"`
				}
				if err := json.NewDecoder(bufio.NewReader(c)).Decode(&req); err != nil {
					return
				}
				_, _ = c.Write([]byte(`{"event":"video-reconfig"}` + "\n"))
				status, data := answer(req.Command)
				b, _ := json.Marshal(map[string]any{"request_id": req.RequestID, "error": status, "data": data})
				_, _ = c.Write(append(b, '\n'))
			}()
		}
	}()
	return sock
}

func TestGetSkipsEventsAndDecodes(t *testing.T) {
	sock := fakeMpv(t, func(cmd []any) (string, any) {
		if cmd[1] == "width" {
			return "success", 2560
		}
		return "property unavailable", nil
	})
	c := &Client{Socket: sock, Timeout: time.Second}
	var w int
	if err := c.Get("width", &w); err != nil || w != 2560 {
		t.Fatalf("width = %d, %v", w, err)
	}
	if err := c.Get("height", &w); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
}

func TestHungMpvTimesOut(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "mpv.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, _ := ln.Accept()
		if c != nil {
			time.Sleep(2 * time.Second)
			_ = c.Close()
		}
	}()
	c := &Client{Socket: sock, Timeout: 200 * time.Millisecond}
	if _, err := c.Command("get_property", "width"); err == nil {
		t.Fatal("want a timeout error")
	}
}
