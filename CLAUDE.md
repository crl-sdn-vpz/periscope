# Periscope: project context

Screen streaming from a Windows host to a Linux viewer. The viewer adds layout modes,
overlay opacity control for local windows, and full-resolution frame capture to the clipboard.
Status: working POC (v0.1.0), tested end to end on Windows 11 → Ubuntu/GNOME.

## Architecture

```
windows/ (sender, PowerShell + ffmpeg)            linux/ (viewer, bash + mpv)
ddagrab (DXGI Desktop Duplication)                mpv  srt://0.0.0.0:PORT?mode=listener
 → hwdownload → bgra → [scale] → yuv444p          ├ JSON IPC socket ($XDG_RUNTIME_DIR/periscope/mpv.sock)
 → libx264 ultrafast/zerolatency CRF 18           ├ wmctrl/xprop for layout + _NET_WM_WINDOW_OPACITY
 → MPEG-TS → SRT caller (AES-128, passphrase) ──▶ └ screenshot-to-file … video → PNG → xclip/wl-copy
```

- **Transport:** SRT over UDP (default port 9000). Windows is the *caller* and Linux the *listener*,
  so start order doesn't matter: the sender retries with exponential backoff (2→15 s) and mpv
  re-listens via `--loop-playlist=inf`. A wrapper loop restarts mpv if it dies.
- **Codec:** H.264 4:4:4 by default, for legible colored text in captured frames. Hardware encoders
  (nvenc/qsv/amf) are selectable. qsv/amf force nv12 (4:2:0).
- **Latency:** ~100–200 ms on LAN. The knobs are `LatencyMs`/`LATENCY_MS` (SRT buffer, must match
  on both sides), mpv `--profile=low-latency --untimed --no-cache`, and `fflags=+nobuffer`.
- **Frame capture:** `screenshot-to-file <path> video` captures the decoded frame at source
  resolution, independent of window size. `latest.png` is a symlink to the newest capture.

## Layout

```
windows/
  periscope.config.ps1     hashtable $Config (TargetHost, Port, Passphrase, Monitor, Fps, Encoder,
                           Quality, Chroma444, MaxWidth, LatencyMs, Capture, FfmpegPath)
  sender.ps1               single-instance (named mutex), builds ffmpeg args, run/retry loop,
                           logs + rotation in %LOCALAPPDATA%\periscope\
  start.cmd                launches sender via `conhost --headless` (Win10 21H2+/Win11)
  stop.ps1 / status.ps1    stop via stop-file + kill ffmpeg matched by `service_name=periscope`
  install.ps1              one-command install (irm | scriptblock): downloads the repo zip, copies to
                           %LOCALAPPDATA%\periscope\app, sets TargetHost/Port/Passphrase, winget ffmpeg,
                           autostart + start. Keeps an existing config.
  install-autostart.ps1    per-user scheduled task at logon (Limited run level)
linux/
  d1                       single bash CLI: view|mode|peek|opacity|shot|status|stop|test|pair|bind-gnome
                           internal subcommands: _viewer-loop, _watch (xprop -spy on _NET_CLIENT_LIST)
  install.sh               deps (apt/dnf/pacman), installs to ~/.local/bin, generates config, firewall,
                           prints `d1 pair`, starts the viewer. Piped from curl it downloads the repo
                           tarball first (PERISCOPE_REF). Questions read /dev/tty, not stdin.
  config.example           sourced bash config → ~/.config/periscope/config (chmod 600)
  kitty-periscope.conf     background-only transparency for terminals
tests/
  e2e-xvfb.sh              full viewer test on Xvfb + openbox with an ffmpeg-simulated sender
README.md                  user-facing install/test guide for both machines
justfile                   lint | e2e | check
```

## Runtime state (Linux)
`$XDG_RUNTIME_DIR/periscope/`: `mpv.sock`, `viewer.pid`, `watcher.pid`, `mode`, `opacity`,
`peek` (stores the pre-peek opacity), `stop` (flag), `input.conf`, `mpv.log`.

## Conventions
- Bash: `set -euo pipefail`, must pass `shellcheck -S warning`. **Watch for SIGPIPE under pipefail.**
  `cmd | head` / `cmd | grep -q` can return 141 and kill the script. Use `|| true` or a here-string.
  This already caused one bug in install.sh.
- PowerShell: must stay compatible with **Windows PowerShell 5.1** (no `??`, ternary or `&&`).
  ffmpeg is launched with `Start-Process -ArgumentList <single string>`, so quote filter/URL args manually.
- mpv default key bindings are disabled (`--no-input-default-bindings`). Control is through IPC only.
- Window identity: mpv runs with a fixed `--title=PERISCOPE-D1` and is located with `xdotool search --name`.
- Keep the sender free of UI: no windows, tray icons or console output. Logs go to `%LOCALAPPDATA%\periscope`.

## Testing
```bash
just check                 # = just lint (shellcheck -S warning) + just e2e (tests/e2e-xvfb.sh)
                           # e2e deps: xvfb openbox mpv ffmpeg socat wmctrl xdotool x11-utils x11-apps
```
Windows scripts have no automated test. Verify them by hand with the foreground run
(`powershell -File sender.ps1`) and `status.ps1`. To sanity-check the generated ffmpeg args on
Linux, replace `ddagrab` with `testsrc2` and run the same filter/encoder chain.

## Platform notes
- X11: full feature set.
- Wayland: streaming and capture work. Per-window opacity and placement don't work, because
  external clients can't set them. Possible paths: a KWin script via D-Bus (KDE) or a small
  GNOME Shell extension exposing a D-Bus method that `d1` calls.

## Backlog
- `d1 shot --region`: select a rectangle on screen, map it to video coordinates (account for
  letterboxing), and crop from the full-res frame.
- Lossless snapshot channel: on-demand PNG from the sender, instead of a decoded video frame.
- Capture-card input source (`/dev/video0` via v4l2) as an alternative to SRT.
- Multiple sources on separate ports, `d1 view --source <name>`.
- Wayland opacity/placement (see Platform notes).
- Windows-side CI: a PowerShell parse check (`[Parser]::ParseFile`) plus Pester tests for `Build-Args`.
