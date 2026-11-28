<!-- For contributors: how to test a change on a real Windows -> Linux pair before it is released. -->
# Hardware test of a checkout (no release needed)

> `just test-pair` serves the build over plain HTTP on your LAN, unauthenticated: someone on the same
> network could swap the download. Use it only on a network you trust.

The Linux viewer builds the release files from a checkout and serves them on your LAN. Both
machines then install through the same installers a release uses. Any v0.2 viewer or sender
is stopped and replaced automatically.

**You need:** both machines on the same network; Linux in an **X11/Xorg session**; Windows 10 21H2+
or 11 with winget (it installs ffmpeg if missing).

**1. Linux viewer:** install the build tools once, then start the test:
```bash
sudo snap install go --classic          # Go 1.26+ (Ubuntu's apt Go is too old)
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
by itself after a restart; see [the README](../README.md#on-the-computer-being-shown-windows).

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
      **Periscope - Status** shows the state. See [Windows](../README.md#on-the-computer-being-shown-windows).
- [ ] Windows `Start-ScheduledTask -TaskName PeriscopeSender` → the stream is back within ~10 s.
- [ ] Linux `periscope stop`, then `periscope view` → the stream comes back on its own.
- [ ] Restart Windows → the sender does **not** start by itself; Start menu > Periscope - Start sender does.
- [ ] Linux `periscope stop` → every faded window is solid again (the reply says how many).
- [ ] Linux `periscope bind-gnome`, then Super+Shift+V stops / starts the viewer (see [the README](../README.md#shortcuts-and-commands)).

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
