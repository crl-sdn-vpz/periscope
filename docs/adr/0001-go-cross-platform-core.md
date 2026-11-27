# 0001. A Go binary for any-device-to-any-device streaming

Date: 2026-11-27 / Status: proposed

Context: v0.2 has a Windows-only sender (PowerShell) and a Linux-only viewer (bash). The owner wants
any device to send and any device to view, whatever the OS, with control from the viewing side and a
one-command install. The POC shows the hard parts already work as external engines: ffmpeg for
capture and encode, SRT with AES for transport, mpv for low-latency decode, display and full-res
frame capture. What doesn't port is the glue: two shells, per-OS window control and state kept in
loose files.

Options considered:
- Keep growing bash + PowerShell, adding a macOS shell: three copies of every feature, and no testing
  across them.
- Python with uv: the owner's stack, but every device needs Python and uv first, and Windows window
  control means ctypes.
- Rust: single binary, but slower to build in and fewer people know it.
- **Go (chosen, owner's call):** one static binary per OS/arch (`CGO_ENABLED=0` cross-compiles from
  any machine), Win32 calls through `golang.org/x/sys/windows` without cgo, and a good stdlib for TLS,
  JSON and processes.

Decision (to confirm):
- **One binary, `periscope`, with roles instead of device types.** `periscope send` (capture → ffmpeg
  → SRT caller) and `periscope view` (SRT listener → mpv) run on any supported OS. One machine can do
  both. `d1`/`sender.ps1` stay until the Go version matches them, then become thin wrappers or go away.
- **Engines stay external:** ffmpeg and mpv, installed by the platform package manager (apt/dnf/pacman,
  winget, brew). periscope builds their arguments, supervises them (restart, backoff) and talks to mpv
  over its JSON IPC. One argument builder per platform, unit-tested with golden files. That also closes
  the "e2e simulates the sender by hand" gap.
- **Platform adapters behind two interfaces:**
  - `Capture` builds the input. Windows: ddagrab, then gdigrab. Linux X11: x11grab. macOS: avfoundation.
    Linux Wayland needs PipeWire through the screencast portal (open question below).
  - `Windows` handles placement, below/on-top and opacity of other windows. X11: EWMH, first by running
    `wmctrl`/`xprop` as today, later over the X protocol. Windows: `SetLayeredWindowAttributes` on other
    top-level windows. macOS: other apps' opacity can't be set, so only split/full there.
    Wayland: compositor extensions (GNOME/KDE), as in the README.
- **Control:**
  - *Local:* `periscope view` serves a control socket (Unix socket, or a named pipe on Windows). The
    CLI, the menu and hotkeys are all clients of it, which replaces the pid/mode/opacity/peek files
    in `$XDG_RUNTIME_DIR`.
  - *Remote:* the sender runs a small control endpoint over TLS 1.3 (stdlib). Pairing pins its
    self-signed certificate fingerprint, so the viewer can start/stop the stream, switch monitor,
    change fps or quality, and ask for a lossless PNG (the backlog's snapshot channel) without a new
    secret. The SRT passphrase still encrypts the video.
- **Pairing:** `periscope pair` on the viewer prints one line or a QR code with host, port, passphrase
  and the viewer's fingerprint. The sender's one-command install takes it. Optional mDNS discovery
  comes later.
- **UI:**
  - First a terminal menu like `d1 menu`, over the control socket (`golang.org/x/term`, no TUI framework).
  - Global hotkeys: Windows registers them with `RegisterHotKey`. Linux keeps DE shortcuts that call
    the CLI, because Wayland gives no global hotkeys to apps.
  - A tray icon comes later: the common Go tray libraries need cgo on Linux and macOS, which breaks
    `CGO_ENABLED=0`. That gets its own ADR.
- **Distribution:** GitHub Releases with binaries for `windows/amd64`, `linux/amd64`, `linux/arm64`,
  `darwin/arm64` and `darwin/amd64`, plus `SHA256SUMS`. `install.sh` and `install.ps1` download the
  right binary, check its checksum, install the engines, write the config and start the role. Building
  a release is a `just` recipe (`go build` matrix + `gh release`); GoReleaser only if that grows.
- **Config:** one JSON file per device, `periscope config` to view and edit it, file mode 600 (user-only
  ACL on Windows). Its fields match today's config keys.

Consequences:
- Adds a Go toolchain (1.24+) for development; users only get a binary.
- New dependencies to approve one by one: `golang.org/x/sys`, `golang.org/x/term`. Nothing else in M1.
- CI on GitHub Actions (free for public repos): `go vet`, `go test`, cross-compile, the Xvfb e2e on
  Ubuntu, a smoke test on Windows.
- Two implementations exist until M1 is done; the e2e test takes the viewer command as a parameter so
  both run against it.

Milestones:
1. **M1, parity:** `periscope send` (Windows) and `periscope view` (Linux X11) do what v0.2 does, with
   the control socket, menu, `pair` and the release-based installers.
2. **M2, remote control:** TLS control channel, pinned pairing, viewer-driven start/stop/monitor,
   lossless snapshot.
3. **M3, more platforms:** send from Linux X11 and macOS, view on Windows, then macOS (split/full).
4. **M4, Wayland and tray:** portal capture, compositor-specific opacity, tray (own ADR).

Open questions:
- Wayland capture: ffmpeg has no PipeWire screencast input I can rely on. Options are GStreamer's
  `pipewiresrc` as a second engine or a small portal client feeding ffmpeg. Needs a spike.
- Windows viewer opacity: `SetLayeredWindowAttributes` on other processes' windows works for most
  Win32 apps but may not for UWP or elevated windows. Needs a spike in M3.
