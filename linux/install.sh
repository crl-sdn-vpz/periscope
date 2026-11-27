#!/usr/bin/env bash
# Installs and starts the Periscope viewer (d1) on device n.
#
# From a checkout:  linux/install.sh [--no-start]
# One command:      curl -fsSL https://raw.githubusercontent.com/crl-sdn-vpz/periscope/main/linux/install.sh | bash
#                   (add  -s -- --no-start  to only install; PERISCOPE_REF=<tag|branch> pins a version)
set -euo pipefail

REPO="crl-sdn-vpz/periscope"
start=1
for arg in "$@"; do
    case $arg in
        --no-start) start=0 ;;
        *) echo "usage: install.sh [--no-start]" >&2; exit 2 ;;
    esac
done

# Piped into bash there are no sibling files: fetch the repo and run the installer from it.
src=${BASH_SOURCE[0]:-}
if [[ -z "$src" || ! -f "$(dirname "$src")/d1" ]]; then
    ref=${PERISCOPE_REF:-main}
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT
    echo "==> Downloading periscope ($ref)"
    curl -fsSL "https://github.com/$REPO/archive/$ref.tar.gz" | tar -xz -C "$tmp" --strip-components=1
    bash "$tmp/linux/install.sh" "$@"
    exit
fi
here=$(cd "$(dirname "$src")" && pwd)

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
d1="$HOME/.local/bin/d1"

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
port=$(grep '^PORT=' "$cfg" | cut -d= -f2- || true)

if command -v ufw >/dev/null && sudo ufw status 2>/dev/null | grep -q "Status: active"; then
    echo "==> Opening UDP ${port:-9000} in ufw"; sudo ufw allow "${port:-9000}/udp"
fi
if command -v firewall-cmd >/dev/null && sudo firewall-cmd --state >/dev/null 2>&1; then
    sudo firewall-cmd --add-port="${port:-9000}/udp" --permanent && sudo firewall-cmd --reload
fi

[[ ":$PATH:" == *":$HOME/.local/bin:"* ]] || echo "!! Add ~/.local/bin to your PATH"

# stdin is the script itself when piped from curl, so questions go to the terminal.
if [[ "${XDG_CURRENT_DESKTOP:-}" == *GNOME* ]] && { : </dev/tty; } 2>/dev/null; then
    read -r -p "Register GNOME hotkeys (Super+Shift+S/A/P/M/O)? [Y/n] " a </dev/tty
    [[ ${a:-y} =~ ^[Yy]$ ]] && "$d1" bind-gnome
fi

echo
"$d1" pair
echo
[[ "${XDG_SESSION_TYPE:-}" == "wayland" ]] && echo "Note: you're on Wayland. Log into an X11/Xorg session for window opacity and split placement (README > Wayland)."

if (( start )) && [[ -n "${DISPLAY:-}${WAYLAND_DISPLAY:-}" ]]; then
    "$d1" view
    echo "Viewer started and waiting for device 1.  Control it with:  d1 menu"
else
    echo "Start the viewer with:  d1 view     (or d1 test for a local test pattern; d1 menu to control it)"
fi
