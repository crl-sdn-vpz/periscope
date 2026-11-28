// Command periscope streams a screen from one device to another: `periscope send` on the device being
// shown, `periscope view` on the device watching it. See README.md.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/crl-sdn-vpz/periscope/internal/clipboard"
	"github.com/crl-sdn-vpz/periscope/internal/config"
	"github.com/crl-sdn-vpz/periscope/internal/control"
	"github.com/crl-sdn-vpz/periscope/internal/menu"
	"github.com/crl-sdn-vpz/periscope/internal/paths"
	"github.com/crl-sdn-vpz/periscope/internal/proc"
	"github.com/crl-sdn-vpz/periscope/internal/sender"
	"github.com/crl-sdn-vpz/periscope/internal/viewer"
	"github.com/crl-sdn-vpz/periscope/internal/x11"
)

// version is set at build time: -ldflags "-X main.version=v0.3.0".
var version = "dev"

const repo = "crl-sdn-vpz/periscope"

const usage = `periscope %s - stream a screen between devices

On the device being shown:
  periscope send                  run the sender (foreground; Ctrl+C stops it)
  periscope send --detach         run it in the background
  periscope send start|stop|toggle|status

On the device watching:
  periscope view [mode]           start the viewer in the background (modes: overlay, split-left, split-right, full)
  periscope menu                  interactive control: arrows + Enter, or one key per action
  periscope mode <mode|cycle>     change layout
  periscope screens               list monitors (the feed's is marked)
  periscope windows               each window and whether overlay fades it (and why not)
  periscope screen [n|next] [--default]   move the feed to monitor n; --default also saves it
  periscope peek                  toggle: fade your windows so the stream is readable
  periscope opacity [v|cycle|reset]
  periscope shot [--path|--no-clip]   full-res PNG of the stream -> clipboard (image by default)
  periscope status                viewer and local sender state
  periscope stop                  stop the viewer and make faded windows solid again (also after a crash)
  periscope toggle                start the viewer if it is stopped, stop it if it runs (for hotkeys/launchers)
  periscope test                  start the viewer and play a local test pattern into it
  periscope pair                  print the one-line install command for the sending device
  periscope bind-gnome            register GNOME hotkeys (Super+Shift+V/S/A/P/M/O/N)

Setup:
  periscope config init [--target-host H] [--port N] [--passphrase P]
  periscope config get <key> | set <key> <value>   e.g. set send.encoder nvenc
  periscope config show [--reveal] | path
  periscope version

Config: $PERISCOPE_CONFIG or %s
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	out, err := run(ctx, os.Args[1:])
	if out != "" {
		fmt.Println(out)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "periscope:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		args = []string{"help"}
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "send":
		return cmdSend(ctx, rest)
	case "view", "start":
		return cmdView(ctx, rest)
	case "mode", "peek", "opacity":
		return callViewer(cmd, rest...)
	case "screens":
		return cmdScreens()
	case "windows":
		return callViewer("windows")
	case "screen":
		return cmdScreen(rest)
	case "shot", "snap":
		return cmdShot(rest)
	case "status":
		return cmdStatus(), nil
	case "stop":
		return cmdStop()
	case "toggle":
		return cmdToggle(ctx)
	case "test":
		return cmdTest(ctx)
	case "pair":
		return cmdPair()
	case "menu":
		return "", cmdMenu()
	case "config":
		return cmdConfig(rest)
	case "bind-gnome":
		return cmdBindGnome()
	case "version", "-v", "--version":
		return version, nil
	case "help", "-h", "--help":
		p, _ := configPath()
		return fmt.Sprintf(usage, version, p), nil
	}
	return "", fmt.Errorf("unknown command %q (periscope help)", cmd)
}

// ---- config -----------------------------------------------------------------------

func configPath() (string, error) {
	if p := os.Getenv("PERISCOPE_CONFIG"); p != "" {
		return p, nil
	}
	return config.Path()
}

func loadConfig() (config.Config, error) {
	p, err := configPath()
	if err != nil {
		return config.Config{}, err
	}
	return config.Load(p)
}

func cmdConfig(args []string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("usage: periscope config init|get|set|show|path")
	}
	path, err := configPath()
	if err != nil {
		return "", err
	}
	switch args[0] {
	case "path":
		return path, nil
	case "init":
		fs := flag.NewFlagSet("config init", flag.ContinueOnError)
		host := fs.String("target-host", "", "LAN IP of the viewing device (sender side)")
		port := fs.Int("port", 0, "UDP port")
		pass := fs.String("passphrase", "", "shared passphrase (10-79 chars); generated if absent")
		if err := fs.Parse(args[1:]); err != nil {
			return "", err
		}
		c, err := config.Load(path)
		created := errors.Is(err, config.ErrNotFound)
		if err != nil && !created {
			return "", err
		}
		if created {
			if c.Passphrase, err = config.NewPassphrase(); err != nil {
				return "", err
			}
		}
		if *host != "" {
			c.Send.TargetHost = *host
		}
		if *port != 0 {
			c.Port = *port
		}
		if *pass != "" {
			c.Passphrase = *pass
		}
		if err := c.Validate(); err != nil {
			return "", err
		}
		if err := config.Save(path, c); err != nil {
			return "", err
		}
		if created {
			return "created " + path, nil
		}
		return "updated " + path, nil
	case "get":
		if len(args) != 2 {
			return "", errors.New("usage: periscope config get <key>")
		}
		c, err := config.Load(path)
		if err != nil {
			return "", err
		}
		return config.Get(c, args[1])
	case "set":
		if len(args) != 3 {
			return "", errors.New("usage: periscope config set <key> <value>")
		}
		c, err := config.Load(path)
		if err != nil {
			return "", err
		}
		if c, err = config.Set(c, args[1], args[2]); err != nil {
			return "", err
		}
		if err := config.Save(path, c); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s = %s", args[1], args[2]), nil
	case "show":
		c, err := config.Load(path)
		if err != nil {
			return "", err
		}
		if len(args) < 2 || args[1] != "--reveal" {
			c.Passphrase = strings.Repeat("*", len(c.Passphrase))
		}
		return config.Format(c)
	}
	return "", fmt.Errorf("unknown config command %q", args[0])
}

// ---- viewer -----------------------------------------------------------------------

func socket(name string) (string, error) {
	dir, err := paths.StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func callViewer(cmd string, args ...string) (string, error) {
	sock, err := socket(viewer.SocketName)
	if err != nil {
		return "", err
	}
	out, err := control.Call(sock, control.Request{Cmd: cmd, Args: args}, 15*time.Second)
	if errors.Is(err, control.ErrNotRunning) {
		return "", errors.New("the viewer is not running (run: periscope view)")
	}
	return out, err
}

func cmdView(ctx context.Context, args []string) (string, error) {
	fs := flag.NewFlagSet("view", flag.ContinueOnError)
	foreground := fs.Bool("foreground", false, "run in this terminal instead of the background")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	mode := fs.Arg(0)
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("viewing on %s arrives in milestone 3; use a Linux viewer for now", runtime.GOOS)
	}
	c, err := loadConfig()
	if err != nil {
		return "", err
	}
	if err := c.ValidateView(); err != nil {
		return "", err
	}
	dir, err := paths.StateDir()
	if err != nil {
		return "", err
	}
	if *foreground {
		return "", viewer.Run(ctx, c, dir, mode)
	}
	if _, err := callViewer("status"); err == nil {
		if mode == "" {
			return "the viewer is already running", nil
		}
		return callViewer("mode", mode)
	}
	if mpvPath := c.View.Mpv; mpvPath != "" {
		if _, err := exec.LookPath(mpvPath); err != nil {
			return "", fmt.Errorf("%s not found: install mpv (with SRT support)", mpvPath)
		}
	}
	daemonArgs := []string{"view", "--foreground"}
	if mode != "" {
		daemonArgs = append(daemonArgs, mode)
	}
	if err := spawn(dir, "viewer", viewer.SocketName, daemonArgs); err != nil {
		return "", err
	}
	return fmt.Sprintf("listening on srt://0.0.0.0:%d (waiting for the sender)", c.Port), nil
}

// spawn starts this binary detached with args and waits until it answers on its control socket.
// The daemon's early output goes to <name>.start.log, which is shown if it fails to come up.
func spawn(dir, name, sockName string, args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	startLog := filepath.Join(dir, name+".start.log")
	f, err := os.OpenFile(startLog, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	cmd := exec.Command(exe, args...)
	cmd.Stdout, cmd.Stderr = f, f
	proc.Detach(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	sock := filepath.Join(dir, sockName)
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); {
		if _, err := control.Call(sock, control.Request{Cmd: "status"}, 5*time.Second); err == nil {
			return nil
		}
		select {
		case <-exited:
			b, _ := os.ReadFile(startLog)
			return fmt.Errorf("the %s exited: %s", name, strings.TrimSpace(string(b)))
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("the %s did not come up within 15s (see %s)", name, startLog)
}

func cmdStop() (string, error) {
	out, err := callViewer("stop")
	if err == nil {
		// Return only once the viewer has exited, so a start right after (toggle, menu) can't race it.
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
			if _, serr := callViewer("status"); serr != nil {
				return out, nil
			}
		}
		return out, errors.New("the viewer did not exit within 5 s")
	}
	if !strings.Contains(err.Error(), "not running") {
		return out, err
	}
	// No viewer: still clean up windows a crashed or killed viewer may have left translucent.
	c, cerr := loadConfig()
	if cerr != nil && !errors.Is(cerr, config.ErrNotFound) {
		return "", cerr
	}
	n, rerr := viewer.RestoreWindows(c.View)
	switch {
	case rerr != nil:
		return "", fmt.Errorf("the viewer is not running; restoring windows failed: %w", rerr)
	case n > 0:
		return fmt.Sprintf("the viewer was not running; %d window(s) back to solid", n), nil
	}
	return "the viewer is not running", nil
}

func cmdToggle(ctx context.Context) (string, error) {
	var out, title string
	var err error
	if _, serr := callViewer("status"); serr == nil {
		out, err = cmdStop()
		title = "periscope viewer stopped"
	} else {
		out, err = cmdView(ctx, nil)
		title = "periscope viewer started"
	}
	if err != nil {
		notify("periscope", err.Error())
		return out, err
	}
	notify(title, out)
	return out, nil
}

func cmdScreens() (string, error) {
	ms, err := x11.Monitors()
	if err != nil {
		return "", fmt.Errorf("list monitors (needs X11 and xrandr): %w", err)
	}
	feed := -1
	if out, err := callViewer("screen", "index"); err == nil {
		feed, _ = strconv.Atoi(out)
	}
	var b strings.Builder
	for _, m := range ms {
		b.WriteString("  " + m.String())
		if m.Index == feed {
			b.WriteString("   <- feed")
		}
		b.WriteString("\n")
	}
	b.WriteString("Move the feed: periscope screen <n|next>   (add --default to keep it there next time)")
	return b.String(), nil
}

func cmdScreen(args []string) (string, error) {
	target, save := "", false
	for _, a := range args {
		switch {
		case a == "--default":
			save = true
		case target == "":
			target = a
		default:
			return "", errors.New("usage: periscope screen [n|next] [--default]")
		}
	}
	out, err := callViewer("screen", target)
	running := err == nil
	if err != nil && !(save && strings.Contains(err.Error(), "not running")) {
		return "", err
	}
	if !save {
		return out, nil
	}
	n := target
	if running {
		if n, err = callViewer("screen", "index"); err != nil {
			return out, err
		}
	} else if _, err := strconv.Atoi(n); err != nil {
		return "", errors.New("the viewer is not running: give a monitor number to save (periscope screens)")
	}
	if _, err := cmdConfig([]string{"set", "view.screen", n}); err != nil {
		return out, err
	}
	return strings.TrimSpace(out + "\nmonitor " + n + " saved as the default"), nil
}

func cmdShot(args []string) (string, error) {
	copyWhat := "image"
	if len(args) > 0 {
		switch args[0] {
		case "--path":
			copyWhat = "path"
		case "--no-clip":
			copyWhat = "none"
		default:
			return "", errors.New("usage: periscope shot [--path|--no-clip]")
		}
	}
	path, err := callViewer("shot")
	if err != nil {
		return "", err
	}
	switch copyWhat {
	case "image":
		err = clipboard.CopyPNG(path)
	case "path":
		err = clipboard.CopyText(path)
	}
	if err != nil {
		return path, fmt.Errorf("saved, but not copied: %w", err)
	}
	notify("periscope screenshot", filepath.Base(path)+" ("+copyWhat+" copied)")
	return path, nil
}

// notify shows a desktop notification when notify-send exists (best effort: the result is also printed).
func notify(title, body string) {
	if _, err := exec.LookPath("notify-send"); err == nil {
		_ = exec.Command("notify-send", "-a", "periscope", "-t", "1500", title, body).Run()
	}
}

func cmdStatus() string {
	var parts []string
	if out, err := callViewer("status"); err == nil {
		parts = append(parts, out)
	} else if errors.Is(err, control.ErrNotRunning) || strings.Contains(err.Error(), "not running") {
		parts = append(parts, "viewer : stopped")
	} else {
		parts = append(parts, "viewer : "+err.Error())
	}
	if out, err := callSender("status"); err == nil {
		parts = append(parts, out)
	}
	return strings.Join(parts, "\n")
}

// ---- sender -----------------------------------------------------------------------

func callSender(cmd string) (string, error) {
	sock, err := socket(sender.SocketName)
	if err != nil {
		return "", err
	}
	out, err := control.Call(sock, control.Request{Cmd: cmd}, 5*time.Second)
	if errors.Is(err, control.ErrNotRunning) {
		return "", errors.New("the sender is not running")
	}
	return out, err
}

func cmdSend(ctx context.Context, args []string) (string, error) {
	if len(args) > 0 {
		switch args[0] {
		case "status", "stop":
			return callSender(args[0])
		case "start":
			args = append([]string{"--detach"}, args[1:]...)
		case "toggle": // one shortcut/hotkey for both directions
			if _, err := callSender("status"); err == nil {
				return callSender("stop")
			}
			args = append([]string{"--detach"}, args[1:]...)
		}
	}
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	detach := fs.Bool("detach", false, "run in the background")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	c, err := loadConfig()
	if err != nil {
		return "", err
	}
	if err := c.ValidateSend(); err != nil {
		return "", err
	}
	dir, err := paths.StateDir()
	if err != nil {
		return "", err
	}
	if *detach {
		if err := spawn(dir, "sender", sender.SocketName, []string{"send"}); err != nil {
			return "", err
		}
		return fmt.Sprintf("sender running in the background, streaming to %s:%d (periscope send status)", c.Send.TargetHost, c.Port), nil
	}
	return "", sender.Run(ctx, c, dir, os.Stderr)
}

func cmdTest(ctx context.Context) (string, error) {
	if _, err := callViewer("status"); err != nil {
		out, err := cmdView(ctx, nil)
		if err != nil {
			return "", err
		}
		fmt.Println(out)
	}
	c, err := loadConfig()
	if err != nil {
		return "", err
	}
	c.Send.TargetHost, c.Send.Capture = "127.0.0.1", "test"
	dir, err := paths.StateDir()
	if err != nil {
		return "", err
	}
	fmt.Println("sending a 2560x1440 test pattern to the local viewer (Ctrl+C stops it)")
	return "", sender.Run(ctx, c, dir, io.Discard)
}

// ---- pairing ----------------------------------------------------------------------

// lanIP returns the address this machine uses to reach the network (no packet is sent).
func lanIP() string {
	c, err := net.Dial("udp", "192.0.2.1:9")
	if err != nil {
		return "<this-machine-LAN-IP>"
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).IP.String()
}

func cmdPair() (string, error) {
	c, err := loadConfig()
	if err != nil {
		return "", err
	}
	if err := c.Validate(); err != nil {
		return "", err
	}
	if strings.ContainsAny(c.Passphrase, `"'`) {
		return "", errors.New("the passphrase can't contain quotes for the pairing commands; run: periscope config set passphrase <new>")
	}
	ip := lanIP()
	base := "https://raw.githubusercontent.com/" + repo + "/main/install"
	return fmt.Sprintf(`To stream another device to this one, run one command on that device. It installs periscope
and ffmpeg, writes the config and starts sending here (%s:%d).

Windows (PowerShell or a command prompt):
  powershell -NoProfile -ExecutionPolicy Bypass -Command "& ([scriptblock]::Create((irm %s/install.ps1))) -Role send -TargetHost %s -Port %d -Passphrase '%s'"

Linux, X11 session (terminal):
  curl -fsSL %s/install.sh | bash -s -- --role send --target-host %s --port %d --passphrase '%s'

Show this again with:  periscope pair`, ip, c.Port, base, ip, c.Port, c.Passphrase, base, ip, c.Port, c.Passphrase), nil
}

// ---- menu -------------------------------------------------------------------------

func cmdMenu() error {
	act := func(cmd string, args ...string) func() (string, error) {
		return func() (string, error) { return callViewer(cmd, args...) }
	}
	items := []menu.Item{
		{Key: 'v', Label: "Start viewer", Run: func() (string, error) { return cmdView(context.Background(), nil) }},
		{Key: 'o', Label: "Overlay mode", Run: act("mode", "overlay")},
		{Key: 'l', Label: "Split left", Run: act("mode", "split-left")},
		{Key: 'r', Label: "Split right", Run: act("mode", "split-right")},
		{Key: 'f', Label: "Full screen", Run: act("mode", "full")},
		{Key: 'n', Label: "Feed to next monitor", Run: act("screen", "next")},
		{Key: 'd', Label: "Use this monitor by default", Run: func() (string, error) { return cmdScreen([]string{"--default"}) }},
		{Key: 'p', Label: "Peek (toggle)", Run: act("peek")},
		{Key: 'c', Label: "Cycle opacity", Run: act("opacity", "cycle")},
		{Key: '0', Label: "Reset opacity", Run: act("opacity", "reset")},
		{Key: 's', Label: "Screenshot to clipboard", Run: func() (string, error) { return cmdShot(nil) }},
		{Key: 'a', Label: "Screenshot, copy path", Run: func() (string, error) { return cmdShot([]string{"--path"}) }},
		{Key: 'w', Label: "Pair a sending device", Run: cmdPair},
		{Key: 'x', Label: "Stop viewer", Run: cmdStop},
		{Key: 'q', Label: "Quit menu"},
	}
	return menu.Run("periscope "+version+" · viewer", items, cmdStatus, 2*time.Second)
}

// ---- GNOME hotkeys ----------------------------------------------------------------

func cmdBindGnome() (string, error) {
	if _, err := exec.LookPath("gsettings"); err != nil {
		return "", errors.New("gsettings not found (not GNOME?)")
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	const schema = "org.gnome.settings-daemon.plugins.media-keys"
	const base = "/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings"
	binds := []struct{ id, name, args, key string }{
		{"periscope-toggle", "periscope: start or stop the viewer", "toggle", "<Super><Shift>v"},
		{"periscope-shot", "periscope: screenshot to clipboard", "shot", "<Super><Shift>s"},
		{"periscope-shotpath", "periscope: screenshot, copy path", "shot --path", "<Super><Shift>a"},
		{"periscope-peek", "periscope: peek (fade my windows)", "peek", "<Super><Shift>p"},
		{"periscope-mode", "periscope: cycle layout", "mode cycle", "<Super><Shift>m"},
		{"periscope-opacity", "periscope: cycle overlay opacity", "opacity cycle", "<Super><Shift>o"},
		{"periscope-screen", "periscope: feed to next monitor", "screen next", "<Super><Shift>n"},
	}
	cur, err := exec.Command("gsettings", "get", schema, "custom-keybindings").Output()
	if err != nil {
		return "", fmt.Errorf("read GNOME keybindings: %w", err)
	}
	list := strings.TrimSpace(string(cur))
	if list == "@as []" {
		list = "[]"
	}
	var out strings.Builder
	for _, b := range binds {
		path := base + "/" + b.id + "/"
		if !strings.Contains(list, "'"+path+"'") {
			if list == "[]" {
				list = "['" + path + "']"
			} else {
				list = strings.TrimSuffix(list, "]") + ", '" + path + "']"
			}
		}
		for _, kv := range [][2]string{{"name", b.name}, {"command", exe + " " + b.args}, {"binding", b.key}} {
			if err := exec.Command("gsettings", "set", schema+".custom-keybinding:"+path, kv[0], kv[1]).Run(); err != nil {
				return out.String(), fmt.Errorf("set %s %s: %w", b.id, kv[0], err)
			}
		}
		fmt.Fprintf(&out, "  %-22s %s\n", b.key, b.name)
	}
	if err := exec.Command("gsettings", "set", schema, "custom-keybindings", list).Run(); err != nil {
		return out.String(), fmt.Errorf("save GNOME keybindings: %w", err)
	}
	return out.String() + "GNOME shortcuts registered", nil
}
