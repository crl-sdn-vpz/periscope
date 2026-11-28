# Security

## Reporting a problem

Please report security issues privately, not in a public issue: on GitHub, **Security → Report a
vulnerability** in this repository. Include the version (`periscope version`), both operating
systems, and how to reproduce the problem. You'll get an answer within a week.

Supported: the latest release of the `periscope` binary. The legacy v0.2 scripts (`linux/d1`,
`windows/*.ps1`) don't get fixes and lack several of the protections below. Use the binary.

## What periscope protects, and how

Periscope sends a picture of one computer's screen to another on your network. The assets are
that picture, the shared passphrase, and the two computers themselves.

| Area | Protection |
|---|---|
| **Stream** | SRT's built-in AES-128 encryption, keyed from the shared passphrase (PBKDF2). Encryption is *enforced*: a peer without the passphrase, or with a different one, is refused, never served in clear. |
| **Who can send to the viewer** | Only a sender with the passphrase. The default passphrase is 32 random letters and digits; your own must be 10–79 characters. |
| **Network exposure** | The viewer listens on UDP (default 9000). The installer opens that port in ufw/firewalld for the **local subnet only**, not for any address. The viewer never sends the screen anywhere; only senders send. |
| **Passphrase at rest** | The config file is readable only by your user (mode 0600 on Linux; your private `%APPDATA%` on Windows). `periscope config show` masks it. |
| **Passphrase in use** | The viewer gives it to mpv through a private options file (mode 0600, deleted on stop), not on mpv's command line. Logs and status messages are redacted. |
| **Local control** | The viewer and sender take commands on Unix sockets in a per-user directory (`$XDG_RUNTIME_DIR/periscope`, `%LOCALAPPDATA%\periscope`) that periscope checks is private: not a symlink, owned by you, mode 0700. mpv's own control socket lives there too. |
| **Decoding untrusted data** | mpv runs with `--no-config --load-scripts=no --ytdl=no`, so your personal mpv settings and scripts don't run inside the process that decodes network video. |
| **Screenshots** | Saved at mode 0600 (new screenshot folders are 0700). |
| **Downloads** | The installers download over HTTPS from GitHub Releases and check the binary and the icons against the release's `SHA256SUMS` before installing. |
| **Uninstall** | It only deletes absolute paths it owns: folders named `periscope`, or the exact config file `PERISCOPE_CONFIG` names, never the folder around it. With `HOME`/`LOCALAPPDATA` unset it refuses to run. |
| **Builds** | A supported Go toolchain (see `go.mod`), two dependencies (`golang.org/x/sys`, `golang.org/x/term`), `govulncheck` in CI. |

## Known limits

- **Linux senders: the passphrase is in ffmpeg's command line**, which other local users of that
  computer can see (`ps`). ffmpeg has no other way to receive it. On a shared Linux machine, use
  a dedicated passphrase and change it (`periscope config set passphrase …` on both sides) if you
  suspect it leaked. On Windows, only you and administrators can read another process's command line.
- **The pairing command contains the passphrase**, so it ends up in the shell history of whoever
  pastes it (bash history, PowerShell's PSReadLine history). Delete that line if the computer is
  shared.
- **The passphrase is the only credential.** Anyone who knows it and can reach the viewer's UDP
  port can show it a picture. They can't make the viewer send anything or run anything, but mpv
  and ffmpeg do decode what they send, so keep those up to date with your package manager or winget.
- **Releases are not code-signed yet.** The checksums protect the download in transit and from
  corruption. They come from the same release, so they don't prove who built it. Windows
  SmartScreen may warn for that reason.
- **The sender has no on-screen indicator, by request.** It only runs when started from the Start
  menu or a terminal, it appears in Task Manager, and it never starts at logon. Use it only on
  computers you own or have permission to watch.
- **`just test-pair` (a development tool) serves the build over plain HTTP** on your LAN. An
  attacker on the same network could swap the download. Use it on networks you trust.
- The local control sockets trust any process running as your user. That's the usual boundary:
  such a process could already read your files.
