#!/usr/bin/env bash
# Installs the Periscope viewer (d1) on device n.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)

pkgs_apt="mpv socat wmctrl xdotool x11-utils x11-xserver-utils xclip wl-clipboard libnotify-bin ffmpeg"
pkgs_dnf="mpv socat wmctrl xdotool xprop xrandr xclip wl-clipboard libnotify ffmpeg"
pkgs_pac="mpv socat wmctrl xdotool xorg-xprop xorg-xrandr xclip wl-clipboard libnotify ffmpeg"

echo "==> Installing dependencies"
if   command -v apt-get >/dev/null; then sudo apt-get update -q && sudo apt-get install -y $pkgs_apt
elif command -v dnf     >/dev/null; then sudo dnf install -y $pkgs_dnf
elif command -v pacman  >/dev/null; then sudo pacman -S --needed --noconfirm $pkgs_pac
else echo "Unknown package manager; install manually: $pkgs_apt"; fi

protos=$(mpv --list-protocols 2>/dev/null || true)
if ! grep -qw 'srt://' <<<"$protos"; then
    echo "!! Your mpv build has no SRT support. Install mpv from your distro's newer repo or Flathub (io.mpv.Mpv)."
fi

echo "==> Installing d1 to ~/.local/bin"
mkdir -p "$HOME/.local/bin"
install -m 755 "$here/d1" "$HOME/.local/bin/d1"

cfg="${XDG_CONFIG_HOME:-$HOME/.config}/periscope/config"
if [[ ! -f "$cfg" ]]; then
    mkdir -p "$(dirname "$cfg")"
    # (|| true: tr gets SIGPIPE when head stops reading, which pipefail would treat as fatal)
    pass=$(LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom 2>/dev/null | head -c 32 || true)
    [[ ${#pass} -eq 32 ]] || { echo "!! could not generate a passphrase"; exit 1; }
    sed "s/^PASSPHRASE=.*/PASSPHRASE=$pass/" "$here/config.example" > "$cfg"
    chmod 600 "$cfg"
    echo "==> Created $cfg"
fi
pass=$(grep '^PASSPHRASE=' "$cfg" | cut -d= -f2-)
port=$(grep '^PORT=' "$cfg" | cut -d= -f2- || echo 9000)

if command -v ufw >/dev/null && sudo ufw status 2>/dev/null | grep -q "Status: active"; then
    echo "==> Opening UDP ${port:-9000} in ufw"; sudo ufw allow "${port:-9000}/udp"
fi
if command -v firewall-cmd >/dev/null && sudo firewall-cmd --state >/dev/null 2>&1; then
    sudo firewall-cmd --add-port="${port:-9000}/udp" --permanent && sudo firewall-cmd --reload
fi

[[ ":$PATH:" == *":$HOME/.local/bin:"* ]] || echo "!! Add ~/.local/bin to your PATH"

if [[ "${XDG_CURRENT_DESKTOP:-}" == *GNOME* ]]; then
    read -r -p "Register GNOME hotkeys (Super+Shift+S/A/P/M/O)? [Y/n] " a
    [[ ${a:-y} =~ ^[Yy]$ ]] && "$HOME/.local/bin/d1" bind-gnome
fi

echo
echo "Put these in windows\\periscope.config.ps1 on device 1:"
echo "    TargetHost = \"$(hostname -I 2>/dev/null | awk '{print $1}')\""
echo "    Port       = ${port:-9000}"
echo "    Passphrase = \"$pass\""
echo
echo "Then:  d1 test   (local test pattern)   |   d1 view   (wait for device 1)"
[[ "${XDG_SESSION_TYPE:-}" == "wayland" ]] && echo "Note: you're on Wayland. Log into an X11/Xorg session for window opacity and split placement (README > Wayland)."
true
