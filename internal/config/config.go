// Package config loads, validates and saves the per-device periscope config (JSON).
package config

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Config is shared by both roles: the transport fields must match on sender and viewer.
type Config struct {
	Port       int    `json:"port"`
	Passphrase string `json:"passphrase"`
	LatencyMs  int    `json:"latency_ms"`
	Send       Send   `json:"send"`
	View       View   `json:"view"`
}

// Send configures `periscope send`.
type Send struct {
	TargetHost string `json:"target_host"`
	Monitor    int    `json:"monitor"`
	Fps        int    `json:"fps"`
	Encoder    string `json:"encoder"`   // x264 | nvenc | qsv | amf
	Quality    int    `json:"quality"`   // CRF / QP; lower is sharper
	Chroma444  bool   `json:"chroma444"` // x264 and nvenc only
	MaxWidth   int    `json:"max_width"` // 0 = native
	Capture    string `json:"capture"`   // auto | ddagrab | gdigrab | x11grab | test
	Ffmpeg     string `json:"ffmpeg"`    // binary name on PATH or a full path
}

// View configures `periscope view`.
type View struct {
	Screen         int       `json:"screen"`
	ShotDir        string    `json:"shot_dir"`
	OverlayOpacity float64   `json:"overlay_opacity"`
	PeekOpacity    float64   `json:"peek_opacity"`
	OpacitySteps   []float64 `json:"opacity_steps"`
	OverlayInclude string    `json:"overlay_include"` // regex on WM_CLASS; empty = every window
	OverlayExclude string    `json:"overlay_exclude"`
	SplitRatio     float64   `json:"split_ratio"`
	DefaultMode    string    `json:"default_mode"`
	Mpv            string    `json:"mpv"`
}

// Modes are the viewer layouts, in menu/cycle order.
var Modes = []string{"overlay", "split-left", "split-right", "full"}

// Encoders and Captures are the accepted values of Send.Encoder and Send.Capture.
var (
	Encoders = []string{"x264", "nvenc", "qsv", "amf"}
	Captures = []string{"auto", "ddagrab", "gdigrab", "x11grab", "test"}
)

const placeholderPassphrase = "change-me"

// hostRe accepts a host name, an IPv4 address or a bracketed IPv6 address.
var hostRe = regexp.MustCompile(`^(?:[A-Za-z0-9](?:[A-Za-z0-9-]{0,62}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,62}[A-Za-z0-9])?)*|\[[0-9A-Fa-f:.]+\])$`)

// Default returns a config with every field set; Passphrase is a placeholder that Validate rejects.
func Default() Config {
	home, _ := os.UserHomeDir()
	return Config{
		Port:       9000,
		Passphrase: placeholderPassphrase,
		LatencyMs:  80,
		Send: Send{
			Monitor: 0, Fps: 30, Encoder: "x264", Quality: 18, Chroma444: true,
			Capture: "auto", Ffmpeg: "ffmpeg",
		},
		View: View{
			ShotDir:        filepath.Join(home, "Pictures", "periscope"),
			OverlayOpacity: 0.80,
			PeekOpacity:    0.12,
			OpacitySteps:   []float64{0.90, 0.75, 0.55, 0.35},
			OverlayExclude: "kitty|Alacritty|wezterm",
			SplitRatio:     0.60,
			DefaultMode:    "overlay",
			Mpv:            "mpv",
		},
	}
}

// Path is the config file location: $XDG_CONFIG_HOME/periscope (Linux), %APPDATA%\periscope (Windows),
// ~/Library/Application Support/periscope (macOS).
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config dir: %w", err)
	}
	return filepath.Join(dir, "periscope", "config.json"), nil
}

// ErrNotFound is returned by Load when no config file exists yet.
var ErrNotFound = errors.New("no config file")

// Load reads path over the defaults, so a partial file is fine. It does not validate.
func Load(path string) (Config, error) {
	c := Default()
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, fmt.Errorf("%w at %s (run: periscope config init)", ErrNotFound, path)
	}
	if err != nil {
		return c, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("parse %s: %w", path, err)
	}
	return c, nil
}

// Save writes c to path readable only by the owner (it holds the passphrase).
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}

// Validate checks the fields both roles depend on; role-specific checks are ValidateSend/ValidateView.
func (c Config) Validate() error {
	n := len(c.Passphrase)
	if c.Passphrase == placeholderPassphrase || n < 10 || n > 79 {
		return fmt.Errorf("passphrase must be 10-79 characters and the same on both devices (it is %d)", n)
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port %d is out of range", c.Port)
	}
	if c.LatencyMs < 20 || c.LatencyMs > 5000 {
		return fmt.Errorf("latency_ms %d must be 20-5000", c.LatencyMs)
	}
	return nil
}

// ValidateSend checks the sender fields.
func (c Config) ValidateSend() error {
	if err := c.Validate(); err != nil {
		return err
	}
	s := c.Send
	if s.TargetHost == "" {
		return errors.New("send.target_host is empty: set it to the viewer's LAN IP")
	}
	if !hostRe.MatchString(s.TargetHost) { // it goes into the SRT URL: no room for extra options
		return fmt.Errorf("send.target_host %q must be a host name or an IP address", s.TargetHost)
	}
	if !slices.Contains(Encoders, s.Encoder) {
		return fmt.Errorf("send.encoder %q must be one of %s", s.Encoder, strings.Join(Encoders, ", "))
	}
	if !slices.Contains(Captures, s.Capture) {
		return fmt.Errorf("send.capture %q must be one of %s", s.Capture, strings.Join(Captures, ", "))
	}
	if s.Fps < 1 || s.Fps > 240 {
		return fmt.Errorf("send.fps %d must be 1-240", s.Fps)
	}
	if s.Quality < 0 || s.Quality > 51 {
		return fmt.Errorf("send.quality %d must be 0-51", s.Quality)
	}
	if s.Monitor < 0 || s.MaxWidth < 0 {
		return errors.New("send.monitor and send.max_width can't be negative")
	}
	return nil
}

// ValidateView checks the viewer fields.
func (c Config) ValidateView() error {
	if err := c.Validate(); err != nil {
		return err
	}
	v := c.View
	if !slices.Contains(Modes, v.DefaultMode) {
		return fmt.Errorf("view.default_mode %q must be one of %s", v.DefaultMode, strings.Join(Modes, ", "))
	}
	for _, o := range append([]float64{v.OverlayOpacity, v.PeekOpacity}, v.OpacitySteps...) {
		if o < 0 || o > 1 {
			return fmt.Errorf("opacity %v must be between 0 and 1", o)
		}
	}
	if v.SplitRatio <= 0.1 || v.SplitRatio >= 0.9 {
		return fmt.Errorf("view.split_ratio %v must be between 0.1 and 0.9", v.SplitRatio)
	}
	for _, re := range []string{v.OverlayInclude, v.OverlayExclude} {
		if _, err := regexp.Compile(re); err != nil {
			return fmt.Errorf("bad overlay regex %q: %w", re, err)
		}
	}
	return nil
}

// NewPassphrase returns 32 random letters and digits (safe in URLs, shells and PowerShell quotes).
func NewPassphrase() (string, error) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 32)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", fmt.Errorf("generate passphrase: %w", err)
		}
		b[i] = alphabet[n.Int64()]
	}
	return string(b), nil
}

// Format returns c as indented JSON.
func Format(c Config) (string, error) {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode config: %w", err)
	}
	return string(b), nil
}

// Get returns the value at a dotted JSON key such as "port" or "send.target_host".
func Get(c Config, key string) (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	var node any
	if err := json.Unmarshal(b, &node); err != nil {
		return "", err
	}
	for _, p := range strings.Split(key, ".") {
		m, ok := node.(map[string]any)
		if !ok {
			return "", fmt.Errorf("unknown config key %q", key)
		}
		if node, ok = m[p]; !ok {
			return "", fmt.Errorf("unknown config key %q", key)
		}
	}
	if _, ok := node.(map[string]any); ok {
		return "", fmt.Errorf("%s is a section, not a value", key)
	}
	out, err := json.Marshal(node)
	if err != nil {
		return "", err
	}
	return strings.Trim(string(out), `"`), nil
}

// Set assigns value to a dotted JSON key such as "send.encoder" or "view.opacity_steps",
// parsing value as that field's type (lists are comma-separated).
func Set(c Config, key, value string) (Config, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return c, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return c, err
	}
	parts := strings.Split(key, ".")
	node := m
	for _, p := range parts[:len(parts)-1] {
		sub, ok := node[p].(map[string]any)
		if !ok {
			return c, fmt.Errorf("unknown config key %q", key)
		}
		node = sub
	}
	last := parts[len(parts)-1]
	cur, ok := node[last]
	if !ok {
		return c, fmt.Errorf("unknown config key %q", key)
	}
	switch cur.(type) {
	case string:
		node[last] = value
	case bool:
		v, err := strconv.ParseBool(value)
		if err != nil {
			return c, fmt.Errorf("%s needs true or false", key)
		}
		node[last] = v
	case float64:
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return c, fmt.Errorf("%s needs a number", key)
		}
		node[last] = v
	case []any:
		var list []float64
		for _, s := range strings.Split(value, ",") {
			v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
			if err != nil {
				return c, fmt.Errorf("%s needs comma-separated numbers", key)
			}
			list = append(list, v)
		}
		node[last] = list
	default:
		return c, fmt.Errorf("%s is a section, not a value", key)
	}
	b, err = json.Marshal(m)
	if err != nil {
		return c, err
	}
	var out Config
	if err := json.Unmarshal(b, &out); err != nil {
		return c, fmt.Errorf("%s: %w", key, err)
	}
	return out, out.Validate()
}
