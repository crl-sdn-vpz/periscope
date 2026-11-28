#!/usr/bin/env bash
# Regenerates the README visuals from a real run, on two virtual displays:
#   :61 shows docs/img/src/remote-desktop.svg (a made-up desktop) and runs `periscope send` (x11grab)
#   :62 runs `periscope view` with ordinary local windows on top, captured in each mode.
# Terminal shots are real colored output converted to SVG by scripts/term2svg.py.
# Needs: Xvfb openbox xcompmgr (translucency needs a compositor) mpv ffmpeg (librsvg) xdotool wmctrl
#        xprop go python3.   Run: just screenshots   (VERSION=vX.Y.Z sets the version shown)
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
OUT="$ROOT/docs/img"
TMP=$(mktemp -d /tmp/periscope-shots.XXXX)
pids=()
cleanup() {
    v stop >/dev/null 2>&1 || true
    s send stop >/dev/null 2>&1 || true
    for p in "${pids[@]}"; do kill "$p" 2>/dev/null || true; done
    rm -rf "$TMP"
}
trap cleanup EXIT
mkdir -p "$OUT"

version=${VERSION:-v0.3.0}
(cd "$ROOT" && go build -ldflags "-X main.version=$version" -o "$TMP/periscope" ./cmd/periscope)
P="$TMP/periscope"

svg2png() { ffmpeg -hide_banner -loglevel error -y -i "$1" ${3:+-vf "scale=$3:-1:flags=lanczos"} -frames:v 1 "$2"; }
grab() { ffmpeg -hide_banner -loglevel error -y -f x11grab -draw_mouse 0 -video_size 1920x1080 -i :62 -frames:v 1 \
    -vf "scale=1440:-1:flags=lanczos" "$1"; }

svg2png "$ROOT/docs/img/src/remote-desktop.svg" "$TMP/remote.png"

Xvfb :61 -screen 0 1920x1080x24 -nolisten tcp & pids+=($!)
Xvfb :62 -screen 0 1920x1080x24 -nolisten tcp & pids+=($!)
sleep 1
DISPLAY=:61 mpv --really-quiet --fs --no-border --no-osc --cursor-autohide=always --image-display-duration=inf \
    "$TMP/remote.png" & pids+=($!)
DISPLAY=:62 openbox >/dev/null 2>&1 & pids+=($!)
DISPLAY=:62 xcompmgr >/dev/null 2>&1 & pids+=($!)
# The viewer's own desktop (seen in split mode) gets a wallpaper: a screen-sized window opened first
# and kept below, so the feed and the local windows all stack above it.
ffmpeg -hide_banner -loglevel error -y -f lavfi -i "color=c=0x1b2130:s=1920x1080" -frames:v 1 "$TMP/wall.png"
DISPLAY=:62 mpv --really-quiet --geometry=1920x1080+0+0 --no-border --no-osc --image-display-duration=inf \
    --title=Wallpaper "$TMP/wall.png" & pids+=($!)
sleep 1.5
DISPLAY=:62 wmctrl -r Wallpaper -b add,below
DISPLAY=:61 xsetroot -cursor_name left_ptr                  # an arrow, not X11's default "X"
DISPLAY=:61 xdotool mousemove 1919 1079                     # park the sender's pointer in a corner
sleep 1

for role in v s; do
    mkdir -p "$TMP/$role/run" "$TMP/$role/home"
    chmod 700 "$TMP/$role/run"
done
v() { env HOME="$TMP/v/home" XDG_RUNTIME_DIR="$TMP/v/run" XDG_SESSION_TYPE=x11 DISPLAY=:62 \
    PERISCOPE_CONFIG="$TMP/v/config.json" TERM=xterm-256color "$P" "$@"; }
s() { env HOME="$TMP/s/home" XDG_RUNTIME_DIR="$TMP/s/run" XDG_SESSION_TYPE=x11 DISPLAY=:61 \
    PERISCOPE_CONFIG="$TMP/s/config.json" "$P" "$@"; }

v config init --passphrase demo-passphrase-2026 --port 9000 >/dev/null
v config set view.shot_dir /home/you/Pictures/periscope >/dev/null   # only displayed: no shot is taken
cp "$TMP/v/config.json" "$TMP/s/config.json"
s config init --target-host 127.0.0.1 >/dev/null
s config set send.capture x11grab >/dev/null

v view >/dev/null
s send --detach >/dev/null
for _ in $(seq 1 60); do v status | grep -q receiving && break; sleep 0.5; done
v status | grep -q receiving || { echo "no stream"; v logs; exit 1; }
for _ in $(seq 1 20); do v status | grep -q fps && break; sleep 0.5; done

# term <name> <title> <cols> <rows> <command...>: capture a command's colored output as docs/img/<name>.svg
term() {
    local name=$1 title=$2 cols=$3 rows=$4; shift 4
    local q; printf -v q '%q ' "$@"
    SHELL=/bin/bash script -qec "stty cols $cols rows $rows; $q" /dev/null < /dev/null > "$TMP/$name.ansi" || true
    sed -i "s/$lan/192.168.1.50/g" "$TMP/$name.ansi"   # don't publish this machine's address
    python3 "$ROOT/scripts/term2svg.py" "$TMP/$name.ansi" "$OUT/$name.svg" "$title"
}
lan=$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for (i = 1; i < NF; i++) if ($i == "src") print $(i + 1)}')
export -f v
export P TMP

# The menu is full-screen: feed it a q after it has drawn the live status.
{ sleep 3; printf q; } | SHELL=/bin/bash script -qec "stty cols 98 rows 42; v menu" /dev/null > "$TMP/menu.ansi" || true
python3 "$ROOT/scripts/term2svg.py" "$TMP/menu.ansi" "$OUT/menu.svg" "periscope menu"
term status "periscope status" 90 20 v status
term pair "periscope pair" 170 20 v pair

# Local windows on the viewer's desktop: the menu and status "terminals" (images of the real output).
svg2png "$OUT/menu.svg" "$TMP/menu.png" 730
svg2png "$OUT/status.svg" "$TMP/status.png" 600
DISPLAY=:62 mpv --really-quiet --no-border --no-osc --image-display-duration=inf --title=Terminal \
    --geometry=+1110+80 "$TMP/menu.png" & pids+=($!)
DISPLAY=:62 mpv --really-quiet --no-border --no-osc --image-display-duration=inf --title=Status \
    --geometry=+120+700 "$TMP/status.png" & pids+=($!)
sleep 2

v mode overlay >/dev/null; sleep 1.5
grab "$OUT/overlay.png"
v peek >/dev/null; sleep 1.5
grab "$OUT/peek.png"
v peek >/dev/null
v mode split-left >/dev/null; sleep 1
DISPLAY=:62 xdotool search --name '^Terminal$' windowmove 1172 60 2>/dev/null || true
DISPLAY=:62 xdotool search --name '^Status$' windowmove 1230 720 2>/dev/null || true
sleep 1
grab "$OUT/split.png"

# Hero: the sender's screen next to the viewer's overlay.
font=/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf
ffmpeg -hide_banner -loglevel error -y -i "$TMP/remote.png" -i "$OUT/overlay.png" -filter_complex "\
[0]scale=880:-1,pad=920:620:20:70:color=0x0d1b2e,drawtext=fontfile=$font:text='Windows PC  ·  periscope send':fontcolor=0xd7dde8:fontsize=26:x=20:y=24[a];\
[1]scale=880:-1,pad=920:620:20:70:color=0x0d1b2e,drawtext=fontfile=$font:text='Linux desktop  ·  periscope view (overlay)':fontcolor=0x46e3d0:fontsize=26:x=20:y=24[b];\
[a][b]hstack=inputs=2,pad=iw+20:ih+20:0:0:color=0x0d1b2e" -frames:v 1 "$OUT/hero.png"

echo "docs/img: $(cd "$OUT" && ls ./*.png ./*.svg | tr '\n' ' ')"
