# Periscope: test guide

Periscope shows **device 1** (Windows) on **device n** (Linux). Device 1's screen is the background
layer, your own windows sit on top with transparency, and one hotkey saves a full-resolution PNG of
device 1 to the clipboard so you can paste it into GenAI tools.

```
 device 1 (Windows)                              device n (Linux)
 ffmpeg sender (no window)  ── encrypted SRT ──▶ d1 viewer: D1 feed + your apps on top
 keyboard/mouse A                                keyboard/mouse B
```

> **Disclosure:** use this with the people you pair with knowing about it. Tell your team or
> client that you sometimes follow the session from a second machine, and share this file if they
> want details. The sender stays quiet only so it doesn't clutter the shared screen.

---

## 0. Before you start

| Need | Check |
|---|---|
| Both machines on the same network | Device 1 can `ping <device-n-ip>` |
| Device n: Linux, **X11/Xorg session** for overlay features | `echo $XDG_SESSION_TYPE` → `x11` (on Wayland: log out, pick "Ubuntu/GNOME on Xorg" from the gear icon) |
| Device 1: Windows 10 21H2+ or Windows 11 | `winver` |
| Two input sets | One keyboard/mouse per device, or a multi-host Bluetooth set (see section 6) |

## Quick start (one command per device)

**Device n (Linux)** installs the dependencies, creates the config with a random passphrase,
opens the firewall port and starts the viewer:
```bash
curl -fsSL https://raw.githubusercontent.com/crl-sdn-vpz/periscope/main/linux/install.sh | bash
```
It ends by printing **one command for device 1** with this machine's IP, port and passphrase filled
in (`d1 pair` prints it again). Paste it into PowerShell on **device 1 (Windows)**. It installs
ffmpeg with winget, installs the scripts to `%LOCALAPPDATA%\periscope\app`, registers autostart and
starts the sender:
```powershell
powershell -NoProfile -ExecutionPolicy Bypass -Command "& ([scriptblock]::Create((irm https://raw.githubusercontent.com/crl-sdn-vpz/periscope/main/windows/install.ps1))) -TargetHost <ip> -Port 9000 -Passphrase '<passphrase>'"
```
Re-run either command to update. Your config is kept. Both installers take a version
(`PERISCOPE_REF=v0.2.0` before `bash` on Linux, `-Ref v0.2.0` on Windows).

On device n, `d1 menu` controls everything from one terminal (arrows + Enter, or one key per
action), and the GNOME hotkeys work without leaving what you're doing.

The sections below do the same steps by hand, which helps when something fails.

For a manual install, clone the repo on **both** machines
(`git clone https://github.com/crl-sdn-vpz/periscope`). Device 1 uses `windows/` and device n uses `linux/`.

---

## 1. Device n (Linux): install and self-test

```bash
cd periscope/linux                    # wherever you cloned it
./install.sh                          # --no-start: install only
```

The installer:
- installs `mpv socat wmctrl xdotool xprop xrandr xclip wl-clipboard ffmpeg`
- copies `d1` to `~/.local/bin`
- creates `~/.config/periscope/config` with a random passphrase
- opens UDP port 9000 if `ufw`/`firewalld` is active
- offers GNOME hotkeys
- prints the device 1 install command and starts the viewer (`d1 view`)

The printed command holds the three values device 1 needs (`TargetHost`, `Port`, `Passphrase`).
Run `d1 pair` to see it again.

### Self-test (no Windows needed)
```bash
d1 test          # opens the viewer and plays a moving test pattern with a clock
```
✅ You should see color bars and a ticking clock fullscreen. Leave it running and **open a
second terminal**:
```bash
d1 status        # → stream : receiving 1920x1080 @ 30 fps
d1 shot          # → prints ~/Pictures/periscope/d1-....png and copies the image
file ~/Pictures/periscope/latest.png   # → PNG image data, 1920 x 1080
```
Paste (Ctrl+V) into an image-aware app such as Claude or GIMP to confirm the clipboard works.
Then press Ctrl+C in the first terminal and run:
```bash
d1 stop
```

---

## 2. Device 1 (Windows): install ffmpeg and configure

Open **PowerShell** (normal user, not admin):

```powershell
winget install Gyan.FFmpeg
```
**Close PowerShell and open a new one** so `ffmpeg` is on PATH, then check:
```powershell
ffmpeg -hide_banner -version | Select-Object -First 1     # version 6.x or 7.x
ffmpeg -hide_banner -filters   | Select-String ddagrab      # must print a line
ffmpeg -hide_banner -protocols | Select-String srt          # must print "srt"
```

Go to the folder and unblock the downloaded scripts:
```powershell
cd $HOME\periscope\windows      # wherever you cloned it
Get-ChildItem -Recurse | Unblock-File
notepad .\periscope.config.ps1
```
In the config, set the three values from step 1:
```powershell
TargetHost = "192.168.x.x"      # device n IP
Port       = 9000
Passphrase = "the-32-char-string-from-install.sh"
```
Optional: `Encoder = "nvenc"` if you have an NVIDIA GPU (lower CPU), and `Monitor = 1` to send your
second monitor instead.

---

## 3. First real test (sender in the foreground)

**Device n:**
```bash
d1 view
```
The screen turns black. That's expected while it waits for device 1.

**Device 1** (PowerShell, in the `windows` folder). Run the sender in the foreground first so
errors are easy to see:
```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\sender.ps1
```
In a **second** PowerShell window, watch the logs:
```powershell
Get-Content "$env:LOCALAPPDATA\periscope\sender.log" -Tail 20 -Wait
```
✅ Within about 5 s, device n shows device 1's screen.

**Device n**, in another terminal:
```bash
d1 status        # receiving 2560x1440 @ 30 fps (your monitor's resolution)
d1 shot          # full-res PNG of device 1 → clipboard
```

To stop the foreground test, press Ctrl+C in the sender window, then run:
```powershell
.\stop.ps1
```

---

## 4. Normal (hidden) mode on device 1

```powershell
.\start.cmd       # starts with no window; a console may flash for an instant
.\status.ps1      # Sender loop : running / Streaming : yes
.\stop.ps1        # stops everything
```

Start automatically at every logon (optional):
```powershell
powershell -ExecutionPolicy Bypass -File .\install-autostart.ps1
Start-ScheduledTask -TaskName PeriscopeSender          # start now without logging out
# remove later:  powershell -ExecutionPolicy Bypass -File .\uninstall-autostart.ps1
```

---

## 5. Test checklist on device n

Run these with the stream up and tick each one:

- [ ] **Overlay:** open a browser and a terminal. They appear on top of the D1 feed at 80% opacity.
  ```bash
  d1 mode overlay
  ```
- [ ] **Peek:** your windows fade to 12% so you can read D1. Run it again to restore them.
  ```bash
  d1 peek
  ```
- [ ] **Opacity steps:** cycles 0.90 → 0.75 → 0.55 → 0.35.
  ```bash
  d1 opacity cycle
  ```
- [ ] **Split:** D1 takes the left 60% of the screen. Try `split-right` too.
  ```bash
  d1 mode split-left
  ```
- [ ] **Full:** D1 is on top and fully opaque. Run `d1 mode overlay` to go back.
  ```bash
  d1 mode full
  ```
- [ ] **Screenshot to image clipboard:** `d1 shot`, then paste into Claude or ChatGPT.
- [ ] **Screenshot to path:** `d1 shot --path`, then paste the path into a CLI tool, e.g.
  `claude "what's wrong in ~/Pictures/periscope/latest.png"`
- [ ] **Hotkeys (GNOME):** Super+Shift+S (shot), Super+Shift+A (shot path), Super+Shift+P (peek),
      Super+Shift+M (layout), Super+Shift+O (opacity)
- [ ] **Reconnect:** run `.\stop.ps1` on device 1, then `.\start.cmd`. The feed returns by itself in a few seconds.
- [ ] **Latency:** open a clock with seconds on device 1 and compare. 100 to 200 ms is normal on a LAN.
- [ ] **Device 1 stays clean:** nothing new in the taskbar or tray, no notifications.

Crisp terminal text over the feed (recommended): use kitty with background-only transparency:
```bash
sudo apt install kitty
echo "include $HOME/periscope/linux/kitty-periscope.conf" >> ~/.config/kitty/kitty.conf
```

---

## 6. Input setup (pick one)

1. **Two keyboard+mouse sets** (recommended): one per device. No software and no lag.
2. **Multi-host Bluetooth** (Logitech Easy-Switch, Keychron…): channel 1 on device 1, channel 2 on
   device n, and you switch with one button.
3. **Input Leap** (one set, software): server on device n, client on device 1. Its client is
   visible on device 1.

---

## 7. Troubleshooting

| Symptom | Where to look / fix |
|---|---|
| Device n stays black | Device 1: `.\status.ps1`, read `ffmpeg.log`. Passphrases must match exactly. Device n: `ss -ulpn \| grep 9000` should list mpv. Check the firewall: `sudo ufw allow 9000/udp`. |
| `ffmpeg.exe not found` in sender.log | Open a new PowerShell after winget, or set `FfmpegPath` to the full path of `ffmpeg.exe` |
| `ddagrab` / `No such filter` error | Update ffmpeg (`winget upgrade Gyan.FFmpeg`) or set `Capture = "gdigrab"` |
| `Connection ... failed: Input/output error` | Wrong passphrase, wrong IP, or the viewer isn't running (`d1 view`) |
| Stuttering on Wi-Fi | `LatencyMs = 200` on device 1 **and** `LATENCY_MS=200` in the Linux config, or use Ethernet |
| High CPU on device 1 | `Encoder = "nvenc"` / `"qsv"` / `"amf"`, `Fps = 20`, or `MaxWidth = 1920` |
| Opacity/split don't work | You're on Wayland. Log into the Xorg session. |
| `d1: command not found` | Add `~/.local/bin` to PATH: `echo 'export PATH=$HOME/.local/bin:$PATH' >> ~/.bashrc` |

Logs:
- Device 1: `%LOCALAPPDATA%\periscope\sender.log`, `ffmpeg.log`
- Device n: `$XDG_RUNTIME_DIR/periscope/mpv.log`

---

## 8. Command reference

**Device n**
```
d1 menu                                         interactive control (arrows + Enter, or one key)
d1 view [overlay|split-left|split-right|full]   start the receiver
d1 mode <mode|cycle>                            change layout
d1 peek                                         fade / restore your windows
d1 opacity [0-1|cycle|reset]                    opacity of your windows
d1 shot [--path|--no-clip]                      full-res PNG of device 1
d1 status | d1 stop | d1 test | d1 pair | d1 bind-gnome
```
Config: `~/.config/periscope/config`

**Device 1** (in `%LOCALAPPDATA%\periscope\app` after the one-command install, or the `windows` folder)
```
.\install.ps1            install / update, configure, autostart and start (-TargetHost -Port -Passphrase -Ref)
.\start.cmd              start hidden
.\status.ps1             running? last log lines
.\stop.ps1               stop
.\install-autostart.ps1 / .\uninstall-autostart.ps1
```
Config: `periscope.config.ps1`

---

## 9. Preview: the `periscope` binary (any device to any device)

v0.3 replaces the two script sets with one Go binary, `periscope`, with roles instead of device
types ([ADR 0001](docs/adr/0001-go-cross-platform-core.md)). It does everything above, plus:
a local control socket instead of state files, `periscope pair` with ready-to-paste commands for
any sending device, and a sender that can also run on Linux (X11). ffmpeg and mpv stay the engines.

Status: it passes the same end-to-end checks as the scripts (Linux), and CI runs the Windows sender
and installer on a Windows runner. **Until it ships as a GitHub release and has been tested on a
real Windows → Linux pair (see "Test the preview" below), the scripts above stay the default.**
Once released:

```bash
# Viewing device (Linux): installs, starts the viewer, prints the command for the sending device
curl -fsSL https://raw.githubusercontent.com/crl-sdn-vpz/periscope/main/install/install.sh | bash
periscope pair      # Windows and Linux one-liners for the sending device, values filled in
periscope menu      # arrows + Enter, or one key per action
```

| v0.2 scripts | periscope |
|---|---|
| `d1 view / mode / peek / opacity / shot / status / stop / test / pair / menu / bind-gnome` | same subcommands: `periscope view`, `periscope mode cycle`, ... |
| (no equivalent) | `periscope screens`, `periscope screen <n\|next> [--default]`: pick the feed's monitor |
| `windows\start.cmd`, `stop.ps1`, logon autostart | Start menu > Periscope (`periscope send toggle/start/stop`); no autostart: it runs only when you start it |
| `d1 stop` | `periscope stop` (also cleans up after a crashed viewer), `periscope toggle`, Super+Shift+V |
| `windows\sender.ps1`, `start.cmd`, `stop.ps1`, `status.ps1` | `periscope send`, `send --detach`, `send stop`, `send status` |
| `~/.config/periscope/config` + `periscope.config.ps1` | one JSON file per device: `periscope config init / get / set / show` |

Build it from source: `just build` (Go 1.24+) → `bin/periscope`.

### Test the preview on your two machines (no release needed)

The Linux viewer builds the release files from a checkout and serves them on your LAN. Both
machines then install through the same installers a release uses. Any v0.2 viewer or sender
is stopped and replaced automatically.

**You need:** both machines on the same network; Linux in an **X11/Xorg session**; Windows 10 21H2+
or 11 with winget (it installs ffmpeg if missing).

**1. Linux viewer:** install the build tools once, then start the test:
```bash
sudo snap install go --classic          # Go 1.24+ (Ubuntu's apt Go is too old)
sudo apt install -y git just curl python3
git clone https://github.com/crl-sdn-vpz/periscope && cd periscope
just test-pair
```
It installs and starts the viewer (the screen goes black: that's the empty feed), then prints one
line like this, with your IP and passphrase filled in:
```powershell
$env:PERISCOPE_DOWNLOAD_BASE='http://192.168.1.50:8000'; & ([scriptblock]::Create((irm http://192.168.1.50:8000/install.ps1))) -Role send -TargetHost 192.168.1.50 -Port 9000 -Passphrase '...'
```
Leave that terminal open.

**2. Windows sender:** open **PowerShell** (normal user, not "Run as administrator") and paste the
line. Expected output: `Downloading`, `Installed`, `Installing ffmpeg` (first time only),
`Config`, `Creating Start menu shortcuts`, `Starting the sender`, then `sender : connecting ...` or
`sender : streaming ...`, and `Done.` Within about 10 s the Linux screen shows the Windows desktop.
You can close PowerShell now: the sender runs hidden and never needs a terminal. It does not start
by itself after a restart; see "Start and stop the sender on Windows" below.

**3. Linux:** press **Ctrl+C** in the `just test-pair` terminal. That only stops the download
server; the viewer keeps running.

**4. Checklist** (Linux commands are `periscope ...`; on Windows use
`& "$env:LOCALAPPDATA\periscope\bin\periscope.exe" ...`):
- [ ] Linux `periscope status` → `stream : receiving <Windows resolution> @ ~30 fps`
- [ ] Latency: open a clock with seconds on Windows and compare. 100 to 250 ms is normal on a LAN.
- [ ] `periscope shot`, then `file ~/Pictures/periscope/latest.png` → a PNG at the Windows resolution;
      Ctrl+V pastes the image into an app.
- [ ] `periscope menu`: try `o` `l` `r` `f` (layouts), `n` (next monitor), `p` (peek), `c` / `0`
      (opacity), `s` (shot), `q`. Each action answers below the menu; `q` / Esc / Ctrl+C always quit.
- [ ] Monitors: `periscope screens` lists them and marks the feed's; `periscope screen next` moves the
      feed; `periscope screen 1 --default` keeps it on monitor 1 from now on.
- [ ] With the feed on your **second** monitor (`periscope screen 1`), windows there fade too
      (`periscope windows` shows `faded to 0.80` for them).
- [ ] Overlay: your Linux windows **on the feed's monitor** turn translucent (windows on other monitors
      stay solid), also newly opened ones and ones you drag onto it. `p` (peek) makes them almost
      invisible and back; the reply says how many windows changed.
- [ ] Optional: `periscope bind-gnome`, then Super+Shift+S / P / M / O.
- [ ] Windows `periscope.exe send status` → `sender : streaming to <linux-ip>:9000 for ...`
- [ ] Windows shows no window, console or tray icon; Task Manager lists `periscope.exe` and `ffmpeg.exe`.
- [ ] Windows `periscope.exe send stop` → Linux status says `waiting for the sender`, and `ffmpeg.exe`
      is gone from Task Manager.
- [ ] Windows Start menu > **Periscope - Start or stop sender** stops it; again starts it.
      **Periscope - Status** shows the state. See "Start and stop the sender on Windows".
- [ ] Windows `Start-ScheduledTask -TaskName PeriscopeSender` → the stream is back within ~10 s.
- [ ] Linux `periscope stop`, then `periscope view` → the stream comes back on its own.
- [ ] Restart Windows → the sender does **not** start by itself; Start menu > Periscope - Start sender does.
- [ ] Linux `periscope stop` → every faded window is solid again (the reply says how many).
- [ ] Linux `periscope bind-gnome`, then Super+Shift+V stops / starts the viewer (see "Control the viewer on Linux").

**If something fails,** send these:
```bash
periscope version; periscope status
tail -n 40 "$XDG_RUNTIME_DIR/periscope/viewer.log" "$XDG_RUNTIME_DIR/periscope/mpv.log"
```
```powershell
$p = "$env:LOCALAPPDATA\periscope"; & "$p\bin\periscope.exe" send status
Get-Content "$p\sender.log" -Tail 30; Get-Content "$p\ffmpeg.log" -Tail 30
```

| Problem | Fix |
|---|---|
| Windows: `irm` can't connect | Wrong IP (the script uses the IP of Linux's default route), or a firewall blocks TCP 8000. The script opens it in ufw; remove that later with `sudo ufw delete allow 8000/tcp`. |
| Windows Defender / SmartScreen flags `periscope.exe` | The preview build is unsigned. Allow it, or build it yourself on Windows with `go build ./cmd/periscope`. |
| `sender.log` shows `ddagrab` / `No such filter` | Old ffmpeg: `winget upgrade Gyan.FFmpeg`, or `periscope.exe config set send.capture gdigrab`. |
| Linux stays on `waiting for the sender` | UDP 9000 is blocked on Linux (`sudo ufw allow 9000/udp`), or the sender points at another IP (`periscope.exe config get send.target_host`). |
| Overlay/split do nothing | You're on Wayland. Log into the Xorg session. |
| Windows stay translucent after stopping | `periscope stop` (works without a running viewer). If it persists, send the output of `periscope windows` (start the viewer first) |
| Opacity/peek seem to do nothing | Run `periscope windows`: it lists every window and whether it is faded, solid (on another monitor) or skipped (and why). Fading only touches windows centered on the feed's monitor, in overlay mode. Terminals in `view.overlay_exclude` (kitty, Alacritty, wezterm) keep their own transparency. |

### Start and stop the sender on Windows

After the install, nothing needs a terminal, and nothing runs until you start it: the sender
does **not** start at logon. While it runs it is hidden: no window, no taskbar button, no tray icon.

| To | Do this |
|---|---|
| Start it | Start menu → type **Periscope** → **Periscope - Start sender** |
| Stop it | Start menu → **Periscope - Stop sender** |
| One entry for both | **Periscope - Start or stop sender** (starts it if stopped, stops it if running) |
| See whether it runs | **Periscope - Status** (opens a small window: `streaming to ...`, `waiting for the viewer ...` or `the sender is not running`; close it when done) |
| Keep them handy | Right-click a Periscope entry → **Pin to Start** or **Pin to taskbar** |

The start/stop entries show nothing when they act. Check with **Periscope - Status**, or on Linux:
when the sender stops, the feed freezes on the last frame and `periscope status` says
`waiting for the sender`. There is deliberately no hotkey (Ctrl+Alt+Shift+P, for one, opens
Kaspersky's on-screen keyboard). To add your own: right-click the entry → Open file location →
Properties → Shortcut key.

From a terminal it's the same: `periscope send start`, `periscope send stop`, `periscope send toggle`,
`periscope send status`.

### Control the viewer on Linux

| To | Hotkey (after `periscope bind-gnome`) | Launcher / command |
|---|---|---|
| Start or stop the viewer | **Super+Shift+V** | app grid: **Periscope: start or stop viewer** · `periscope toggle` |
| Open the control menu | | app grid: **Periscope menu** · `periscope menu` |
| Screenshot → clipboard (image) | **Super+Shift+S** | `periscope shot` · menu `s` |
| Screenshot → clipboard (file path) | **Super+Shift+A** | `periscope shot --path` · menu `a` |
| Peek (fade your windows almost fully, again to restore) | **Super+Shift+P** | `periscope peek` · menu `p` |
| Next layout (overlay → split-left → full) | **Super+Shift+M** | `periscope mode <overlay\|split-left\|split-right\|full>` · menu `o` `l` `r` `f` |
| Next overlay opacity (0.90 → 0.75 → 0.55 → 0.35) | **Super+Shift+O** | `periscope opacity <0-1\|cycle\|reset>` · menu `c` / `0` |
| Feed to the next monitor | **Super+Shift+N** | `periscope screen <n\|next> [--default]` · menu `n` / `d` |

- `periscope bind-gnome` registers the hotkeys (GNOME). Change them in Settings → Keyboard →
  Keyboard Shortcuts → Custom Shortcuts (entries named `periscope: ...`). Other desktops: bind the
  commands in the right column.
- The app-grid launchers come with `install/install.sh` (and `just test-pair`). Right-click →
  **Pin to Dash** keeps them in the dock.
- Stopping (any of the ways above) makes every faded window solid again before it returns. If a
  window ever stays translucent (say, after a crash), `periscope stop` cleans it up even when no
  viewer runs.
- The menu's own keys are listed at its bottom: arrows/j/k + Enter, or one key per action; q/Esc quits.

**Go back to v0.2:** Windows `periscope.exe send stop`, then
`Unregister-ScheduledTask -TaskName PeriscopeSender -Confirm:$false`; Linux `periscope stop`. Then
follow the Quick start at the top.

---

## 10. Development

- Technical context for contributors and Claude Code: [`CLAUDE.md`](CLAUDE.md)
- Needs Go 1.24+ and `just`. End-to-end tests run on a virtual display:
  ```bash
  sudo apt install xvfb openbox mpv ffmpeg socat wmctrl xdotool x11-utils x11-apps xfonts-base shellcheck
  just check      # lint (gofmt, go vet for linux/windows/darwin, shellcheck) + unit tests + both e2e suites
  just dist       # release binaries + SHA256SUMS in dist/
  ```
- CI (`.github/workflows/ci.yml`) runs the same on Ubuntu, and on Windows it parses every `.ps1` in
  Windows PowerShell 5.1, runs `install/install.ps1` end to end and streams into an SRT listener.
- Don't commit a filled-in `windows/periscope.config.ps1`, because it holds your passphrase.

## License

[MIT](LICENSE)
