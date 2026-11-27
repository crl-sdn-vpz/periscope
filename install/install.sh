#!/usr/bin/env bash
# Installs the periscope binary from GitHub Releases plus the engine it drives (mpv to view,
# ffmpeg to send), writes the config and starts the role.
#
#   View (default):  curl -fsSL https://raw.githubusercontent.com/crl-sdn-vpz/periscope/main/install/install.sh | bash
#   Send:            ... | bash -s -- --role send --target-host <viewer-ip> --port 9000 --passphrase '<passphrase>'
#                    (`periscope pair` on the viewer prints this line with the values filled in)
#
# Options: --role view|send   --version vX.Y.Z (default: latest)   --no-start
#          --target-host H --port N --passphrase P   (written to the config)
# PERISCOPE_DOWNLOAD_BASE replaces the release URL (used by tests).
set -euo pipefail

REPO="crl-sdn-vpz/periscope"
role=view version=latest start=1
init_args=()
die() { echo "install: $*" >&2; exit 1; }

while (( $# )); do
    case $1 in
        --role)        role=${2:?}; shift ;;
        --version)     version=${2:?}; shift ;;
        --no-start)    start=0 ;;
        --target-host|--port|--passphrase) init_args+=("$1" "${2:?$1 needs a value}"); shift ;;
        *) die "unknown option $1" ;;
    esac
    shift
done
[[ $role == view || $role == send ]] || die "--role must be view or send"

case "$(uname -s)/$(uname -m)" in
    Linux/x86_64)          asset=periscope_linux_amd64 ;;
    Linux/aarch64|Linux/arm64) asset=periscope_linux_arm64 ;;
    *) die "$(uname -s)/$(uname -m) isn't supported yet (macOS arrives in milestone 3)" ;;
esac

base=${PERISCOPE_DOWNLOAD_BASE:-}
if [[ -z $base ]]; then
    if [[ $version == latest ]]; then base="https://github.com/$REPO/releases/latest/download"
    else base="https://github.com/$REPO/releases/download/$version"; fi
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
echo "==> Downloading $asset ($version)"
curl -fsSL "$base/$asset" -o "$tmp/$asset"
curl -fsSL "$base/SHA256SUMS" -o "$tmp/SHA256SUMS"
sum=$(grep " $asset\$" "$tmp/SHA256SUMS" || true)
[[ -n $sum ]] || die "SHA256SUMS has no entry for $asset"
(cd "$tmp" && sha256sum -c --quiet - <<<"$sum") || die "checksum mismatch for $asset"

mkdir -p "$HOME/.local/bin"
if [[ -x "$HOME/.local/bin/periscope" ]]; then   # an update: stop the old binary's roles first
    "$HOME/.local/bin/periscope" stop >/dev/null 2>&1 || true
    "$HOME/.local/bin/periscope" send stop >/dev/null 2>&1 || true
fi
install -m 755 "$tmp/$asset" "$HOME/.local/bin/periscope"
p="$HOME/.local/bin/periscope"
echo "==> Installed $("$p" version) to $p"

if [[ $role == view ]]; then
    apt="mpv wmctrl xdotool x11-utils x11-xserver-utils xclip wl-clipboard libnotify-bin"
    dnf="mpv wmctrl xdotool xprop xrandr xclip wl-clipboard libnotify"
    pac="mpv wmctrl xdotool xorg-xprop xorg-xrandr xclip wl-clipboard libnotify"
else
    apt="ffmpeg" dnf="ffmpeg" pac="ffmpeg"
fi
echo "==> Installing dependencies"
if   command -v apt-get >/dev/null; then sudo apt-get update -q && sudo apt-get install -y $apt
elif command -v dnf     >/dev/null; then sudo dnf install -y $dnf
elif command -v pacman  >/dev/null; then sudo pacman -S --needed --noconfirm $pac
else echo "!! Unknown package manager; install: $apt"; fi

if [[ $role == view ]]; then
    protos=$(mpv --list-protocols 2>/dev/null || true)
    grep -qw 'srt://' <<<"$protos" || echo "!! Your mpv has no SRT support. Install a newer mpv (distro backports or Flathub io.mpv.Mpv)."
fi

echo "==> Config"
"$p" config init "${init_args[@]}"
port=$("$p" config get port)

if [[ $role == view ]]; then
    if command -v ufw >/dev/null && sudo ufw status 2>/dev/null | grep -q "Status: active"; then
        echo "==> Opening UDP $port in ufw"; sudo ufw allow "$port/udp"
    fi
    if command -v firewall-cmd >/dev/null && sudo firewall-cmd --state >/dev/null 2>&1; then
        sudo firewall-cmd --add-port="$port/udp" --permanent && sudo firewall-cmd --reload
    fi
    # stdin is this script when piped from curl, so the question goes to the terminal.
    if [[ "${XDG_CURRENT_DESKTOP:-}" == *GNOME* ]] && { : </dev/tty; } 2>/dev/null; then
        read -r -p "Register GNOME hotkeys (Super+Shift+S/A/P/M/O)? [Y/n] " a </dev/tty
        [[ ${a:-y} =~ ^[Yy]$ ]] && "$p" bind-gnome
    fi
fi

[[ ":$PATH:" == *":$HOME/.local/bin:"* ]] || echo "!! Add ~/.local/bin to your PATH"
echo

if (( ! start )); then
    echo "Installed. Start with: periscope $role"
elif [[ -z "${DISPLAY:-}${WAYLAND_DISPLAY:-}" ]]; then
    echo "No graphical session here; start later with: periscope $role"
elif [[ $role == view ]]; then
    "$p" view
    "$p" pair
    echo
    echo "Control it with: periscope menu"
else
    "$p" send --detach
    echo "Check it with: periscope send status"
fi
