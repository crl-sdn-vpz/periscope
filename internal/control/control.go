// Package control is the local command channel of a running role (view or send): one JSON request
// and one JSON response per connection over a Unix socket (AF_UNIX also exists on Windows 10+).
package control

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

// Request is a command and its arguments, e.g. {"cmd":"mode","args":["overlay"]}.
type Request struct {
	Cmd  string   `json:"cmd"`
	Args []string `json:"args,omitempty"`
}

// Response carries the command's output, or its error.
type Response struct {
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Handler runs one request.
type Handler func(ctx context.Context, r Request) (string, error)

// ErrRunning means another process already serves this socket.
var ErrRunning = errors.New("already running")

// ErrNotRunning means nothing serves this socket.
var ErrNotRunning = errors.New("not running")

// Listen claims path. A socket file nobody answers on is stale and is replaced.
func Listen(path string) (net.Listener, error) {
	if c, err := net.DialTimeout("unix", path, time.Second); err == nil {
		_ = c.Close()
		return nil, ErrRunning
	}
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", path, err)
	}
	_ = os.Chmod(path, 0o600)
	return ln, nil
}

// Serve answers requests until ctx is done, then closes ln.
func Serve(ctx context.Context, ln net.Listener, h Handler) {
	go func() { <-ctx.Done(); _ = ln.Close() }()
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go serveConn(ctx, c, h)
	}
}

func serveConn(ctx context.Context, c net.Conn, h Handler) {
	defer c.Close()
	var req Request
	if err := json.NewDecoder(bufio.NewReader(c)).Decode(&req); err != nil {
		_ = json.NewEncoder(c).Encode(Response{Error: "bad request: " + err.Error()})
		return
	}
	out, err := h(ctx, req)
	resp := Response{Output: out}
	if err != nil {
		resp.Error = err.Error()
	}
	_ = json.NewEncoder(c).Encode(resp)
}

// Call sends one request to the process serving path and returns its output.
func Call(path string, req Request, timeout time.Duration) (string, error) {
	c, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		return "", ErrNotRunning
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(timeout))
	if err := json.NewEncoder(c).Encode(req); err != nil {
		return "", fmt.Errorf("send %s: %w", req.Cmd, err)
	}
	var resp Response
	if err := json.NewDecoder(c).Decode(&resp); err != nil {
		return "", fmt.Errorf("no answer to %s: %w", req.Cmd, err)
	}
	if resp.Error != "" {
		return resp.Output, errors.New(resp.Error)
	}
	return resp.Output, nil
}
