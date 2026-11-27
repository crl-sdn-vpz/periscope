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
real Windows → Linux pair, the scripts above stay the default.** Once released:

```bash
# Viewing device (Linux): installs, starts the viewer, prints the command for the sending device
curl -fsSL https://raw.githubusercontent.com/crl-sdn-vpz/periscope/main/install/install.sh | bash
periscope pair      # Windows and Linux one-liners for the sending device, values filled in
periscope menu      # arrows + Enter, or one key per action
```

| v0.2 scripts | periscope |
|---|---|
| `d1 view / mode / peek / opacity / shot / status / stop / test / pair / menu / bind-gnome` | same subcommands: `periscope view`, `periscope mode cycle`, ... |
| `windows\sender.ps1`, `start.cmd`, `stop.ps1`, `status.ps1` | `periscope send`, `send --detach`, `send stop`, `send status` |
| `~/.config/periscope/config` + `periscope.config.ps1` | one JSON file per device: `periscope config init / get / set / show` |

Build it from source: `just build` (Go 1.24+) → `bin/periscope`.

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
