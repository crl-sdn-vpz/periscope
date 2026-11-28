package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func valid() Config {
	c := Default()
	c.Passphrase = "abcdefghij0123456789"
	c.Send.TargetHost = "10.0.0.2"
	return c
}

func TestDefaultsNeedAPassphrase(t *testing.T) {
	if err := Default().Validate(); err == nil {
		t.Fatal("the placeholder passphrase must be rejected")
	}
	c := valid()
	if err := errors.Join(c.ValidateSend(), c.ValidateView()); err != nil {
		t.Fatal(err)
	}
}

func TestValidation(t *testing.T) {
	cases := map[string]func(*Config){
		"short passphrase": func(c *Config) { c.Passphrase = "short" },
		"long passphrase":  func(c *Config) { c.Passphrase = strings.Repeat("x", 80) },
		"bad port":         func(c *Config) { c.Port = 70000 },
		"no target":        func(c *Config) { c.Send.TargetHost = "" },
		"target injects":   func(c *Config) { c.Send.TargetHost = "10.0.0.2:1?passphrase=x&" },
		"target spaces":    func(c *Config) { c.Send.TargetHost = "10.0.0.2 x" },
		"bad encoder":      func(c *Config) { c.Send.Encoder = "hevc" },
		"bad mode":         func(c *Config) { c.View.DefaultMode = "bogus" },
		"bad opacity":      func(c *Config) { c.View.OpacitySteps = []float64{1.5} },
		"bad regex":        func(c *Config) { c.View.OverlayExclude = "(" },
	}
	for name, mutate := range cases {
		c := valid()
		mutate(&c)
		if errors.Join(c.ValidateSend(), c.ValidateView()) == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestTargetHostForms(t *testing.T) {
	for _, h := range []string{"192.168.1.50", "viewer.local", "my-pc", "[fe80::1]"} {
		c := valid()
		c.Send.TargetHost = h
		if err := c.ValidateSend(); err != nil {
			t.Errorf("%s rejected: %v", h, err)
		}
	}
}

func TestSaveLoadAndPermissions(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "config.json")
	if _, err := Load(p); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing file: %v", err)
	}
	c := valid()
	c.View.SplitRatio = 0.5
	if err := Save(p, c); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil || got.View.SplitRatio != 0.5 || got.Passphrase != c.Passphrase {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	if runtime.GOOS != "windows" {
		if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
			t.Fatalf("mode %v, want 0600", st.Mode().Perm())
		}
	}
}

func TestPartialFileKeepsDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(`{"port": 9137, "view": {"split_ratio": 0.7}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil || c.Port != 9137 || c.View.SplitRatio != 0.7 || c.View.DefaultMode != "overlay" || c.Send.Fps != 30 {
		t.Fatalf("got %+v %v", c, err)
	}
}

func TestSet(t *testing.T) {
	c := valid()
	var err error
	for _, kv := range [][2]string{
		{"send.encoder", "nvenc"}, {"send.chroma444", "false"}, {"send.max_width", "1920"},
		{"view.opacity_steps", "0.9, 0.5"}, {"port", "9001"},
	} {
		if c, err = Set(c, kv[0], kv[1]); err != nil {
			t.Fatalf("Set %s: %v", kv[0], err)
		}
	}
	if c.Send.Encoder != "nvenc" || c.Send.Chroma444 || c.Send.MaxWidth != 1920 || len(c.View.OpacitySteps) != 2 || c.Port != 9001 {
		t.Fatalf("got %+v", c)
	}
	for _, kv := range [][2]string{{"nope", "1"}, {"send", "x"}, {"send.fps", "fast"}, {"send.fps", "29.5"}, {"passphrase", "short"}} {
		if _, err := Set(c, kv[0], kv[1]); err == nil {
			t.Errorf("Set %s=%s accepted", kv[0], kv[1])
		}
	}
}

func TestNewPassphrase(t *testing.T) {
	p, err := NewPassphrase()
	if err != nil || len(p) != 32 || strings.Trim(p, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789") != "" {
		t.Fatalf("passphrase %q %v", p, err)
	}
}

func TestGet(t *testing.T) {
	c := valid()
	for key, want := range map[string]string{"port": "9000", "send.target_host": "10.0.0.2", "send.chroma444": "true"} {
		if got, err := Get(c, key); err != nil || got != want {
			t.Errorf("Get(%s) = %q, %v", key, got, err)
		}
	}
	for _, key := range []string{"nope", "send", "port.x"} {
		if _, err := Get(c, key); err == nil {
			t.Errorf("Get(%s) accepted", key)
		}
	}
}
