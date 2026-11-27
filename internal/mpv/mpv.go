// Package mpv builds the viewer's mpv command line and talks to mpv over its JSON IPC socket.
package mpv

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync/atomic"
	"time"
)

// Title is the fixed window title used to find mpv's window.
const Title = "PERISCOPE-VIEW"

// Args returns mpv's arguments for a low-latency, input-less, fullscreen SRT listener.
func Args(url, ipcSocket, inputConf, logFile string, screen int) []string {
	s := strconv.Itoa(screen)
	return []string{
		url,
		"--title=" + Title, "--x11-name=periscope", "--wayland-app-id=periscope",
		"--profile=low-latency", "--untimed", "--no-cache",
		"--demuxer-lavf-o-add=fflags=+nobuffer", "--demuxer-lavf-probesize=32", "--demuxer-lavf-analyzeduration=0.2",
		"--loop-playlist=inf", "--idle=yes", "--force-window=immediate",
		"--no-audio", "--hwdec=auto-safe", "--keepaspect=yes", "--border=no",
		"--fs", "--fs-screen=" + s, "--screen=" + s, "--ontop=no",
		"--cursor-autohide=always", "--osd-level=0", "--no-osc",
		"--no-input-default-bindings", "--input-conf=" + inputConf,
		"--input-ipc-server=" + ipcSocket,
		"--screenshot-format=png", "--screenshot-png-compression=1", "--screenshot-tag-colorspace=no",
		"--msg-level=all=warn", "--log-file=" + logFile,
	}
}

// InputConf disables every key except fullscreen, so stray keys can't close the feed.
const InputConf = "# Default bindings are off so stray keys can't close the feed.\nf cycle fullscreen\n"

// Client sends commands to mpv's IPC socket, one connection per command.
type Client struct {
	Socket  string
	Timeout time.Duration
	nextID  atomic.Int64
}

// ErrUnavailable means mpv answered with an error such as "property unavailable" (no video yet).
var ErrUnavailable = errors.New("property unavailable")

type reply struct {
	RequestID int64           `json:"request_id"`
	Error     string          `json:"error"`
	Data      json.RawMessage `json:"data"`
	Event     string          `json:"event"`
}

// Command runs an mpv input command, e.g. Command("set_property", "fullscreen", true).
func (c *Client) Command(args ...any) (json.RawMessage, error) {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 3 * time.Second
	}
	conn, err := net.DialTimeout("unix", c.Socket, timeout)
	if err != nil {
		return nil, fmt.Errorf("mpv is not reachable: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	id := c.nextID.Add(1)
	req, err := json.Marshal(map[string]any{"command": args, "request_id": id})
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(append(req, '\n')); err != nil {
		return nil, fmt.Errorf("write to mpv: %w", err)
	}
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		var r reply
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.Event != "" || r.RequestID != id {
			continue
		}
		switch r.Error {
		case "success":
			return r.Data, nil
		case "property unavailable":
			return nil, ErrUnavailable
		default:
			return nil, fmt.Errorf("mpv %v: %s", args[0], r.Error)
		}
	}
	return nil, fmt.Errorf("mpv did not answer %v: %w", args[0], errors.Join(sc.Err(), errors.New("no reply")))
}

// Get decodes a property into out.
func (c *Client) Get(name string, out any) error {
	data, err := c.Command("get_property", name)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

// Set sets a property.
func (c *Client) Set(name string, value any) error {
	_, err := c.Command("set_property", name, value)
	return err
}
