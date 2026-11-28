# Changelog

All notable changes to periscope. Versions follow [semantic versioning](https://semver.org/);
the release notes on GitHub are this file's section for the version.

## [v0.3.0] - 2026-11-28

The first release of `periscope`, a single binary that replaces the v0.2 scripts.

### Added
- **One binary, two roles:** `periscope send` (Windows, Linux X11) and `periscope view` (Linux), driving
  ffmpeg and mpv. Encrypted SRT streaming with H.264 4:4:4 and about 100–250 ms of latency on a LAN.
- **Viewer layouts:** overlay (your windows translucent over the feed), split left/right and full screen.
  Also peek, opacity steps, and a feed monitor you can choose and save (`periscope screens`, `screen`).
- **Screenshots** at the sender's full resolution, to the clipboard (`periscope shot`).
- **Control:** a terminal menu (`periscope menu`), GNOME hotkeys (`periscope bind-gnome`), app launchers
  on Linux, Start menu entries on Windows, `periscope toggle`, `status`, `windows`, `logs`.
- **Setup:** one-command installers for both sides, `periscope pair` to print the second command, and
  `periscope uninstall` (also one click from the app grid or Start menu).
- **Security:** enforced encryption, the passphrase kept out of process lists and logs on the viewer, a
  private state directory, a firewall rule for the local subnet only, verified downloads. See
  [SECURITY.md](SECURITY.md).

### Changed
- The sender runs only when you start it: no logon autostart, no hotkey, no tray icon.

## [v0.2.0] - 2026-11-27

The bash viewer (`linux/d1`) and PowerShell sender (`windows/`), now legacy:
see [docs/v0.2-scripts.md](docs/v0.2-scripts.md).
