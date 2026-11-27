#!/usr/bin/env bash
# End-to-end test of the Linux viewer (linux/d1) on a virtual X display.
# Simulates the Windows sender with ffmpeg using the same encoder settings as windows/sender.ps1.
#
# Requires: xvfb-run, openbox, mpv (with srt), ffmpeg (with libsrt), socat, wmctrl, xdotool, xprop, xmessage, xfonts-base
# Usage:    tests/e2e-xvfb.sh            (wraps itself in xvfb-run)
set -uo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
D1="$ROOT/linux/d1"

if [[ -z "${PERISCOPE_IN_XVFB:-}" ]]; then
    PERISCOPE_IN_XVFB=1 exec xvfb-run -a -s "-screen 0 2560x1440x24" "$0" "$@"
fi

TMP=$(mktemp -d /tmp/periscope-e2e.XXXX)
export HOME="$TMP/home" XDG_RUNTIME_DIR="$TMP/run" XDG_CONFIG_HOME="$TMP/home/.config" XDG_SESSION_TYPE=x11
mkdir -p "$XDG_CONFIG_HOME/periscope" "$XDG_RUNTIME_DIR"; chmod 700 "$XDG_RUNTIME_DIR"
PASS=e2etestpassphrase42
PORT=9137
cat > "$XDG_CONFIG_HOME/periscope/config" <<EOF
PORT=$PORT
PASSPHRASE=$PASS
SHOT_DIR=$TMP/shots
OVERLAY_EXCLUDE=""
EOF

fails=0
ok()   { echo "  ok   $*"; }
fail() { echo "  FAIL $*"; fails=$((fails+1)); }
check() { local d=$1; shift; if "$@" >/dev/null 2>&1; then ok "$d"; else fail "$d"; fi; }

send() {  # send <WxH> <seconds> [passphrase]
    ffmpeg -hide_banner -loglevel error -re -f lavfi -i "testsrc2=size=$1:rate=30" -vf format=yuv444p \
        -c:v libx264 -preset ultrafast -tune zerolatency -crf 18 -g 60 -bf 0 -an \
        -f mpegts -flush_packets 1 -muxdelay 0 -metadata service_name=periscope -t "$2" \
        "srt://127.0.0.1:$PORT?mode=caller&latency=80000&passphrase=${3:-$PASS}&pbkeylen=16&connect_timeout=3000"
}
png_size() { ffprobe -v error -select_streams v -show_entries stream=width,height -of csv=p=0:s=x "$1"; }
opacity_of() { xprop -id "$1" _NET_WM_WINDOW_OPACITY 2>/dev/null | awk -F' = ' '/=/{print $2}'; }

openbox >/dev/null 2>&1 & WM=$!
sleep 1
echo "== config checks"
check "empty passphrase refused" bash -c "XDG_CONFIG_HOME='$TMP/none' '$D1' view 2>&1 | grep -q 'must be 10-79'"
check "no viewer after refusal"  bash -c "! test -f '$XDG_RUNTIME_DIR/periscope/viewer.pid'"

echo "== viewer"
"$D1" view >/dev/null 2>&1
check "viewer pid alive"        test -f "$XDG_RUNTIME_DIR/periscope/viewer.pid"
check "ipc socket exists"       test -S "$XDG_RUNTIME_DIR/periscope/mpv.sock"
check "unknown mode refused"    bash -c "! '$D1' mode bogus"
check "unknown mode not saved"  bash -c "[[ \$(cat '$XDG_RUNTIME_DIR/periscope/mode') == overlay ]]"

echo "== stream 2560x1440"
send 2560x1440 12 & S1=$!
sleep 5
check "status reports 2560x1440" bash -c "'$D1' status | grep -q 'receiving 2560x1440'"
shot=$("$D1" shot --no-clip 2>/dev/null)
check "screenshot written"      test -s "$shot"
[[ "$(png_size "$shot")" == "2560x1440" ]] && ok "screenshot is native res" || fail "screenshot res $(png_size "$shot")"
check "latest.png symlink"      test -L "$TMP/shots/latest.png"

echo "== overlay / opacity"
xmessage -geometry 300x100+50+50 app1 & sleep 1
"$D1" mode overlay >/dev/null 2>&1
app=$(xdotool search --name '^xmessage$' | head -n1)
d1w=$(xdotool search --name '^PERISCOPE-D1$' | head -n1)
[[ "$(opacity_of "$app")" == "3435973836" ]] && ok "overlay window at 0.80" || fail "overlay opacity $(opacity_of "$app")"
check "D1 window kept below"    bash -c "xprop -id $d1w _NET_WM_STATE | grep -q BELOW"
"$D1" peek >/dev/null 2>&1
[[ "$(cat "$XDG_RUNTIME_DIR/periscope/opacity")" == "0.12" ]] && ok "peek fades" || fail "peek"
"$D1" peek >/dev/null 2>&1
[[ "$(cat "$XDG_RUNTIME_DIR/periscope/opacity")" == "0.80" ]] && ok "peek restores" || fail "peek restore"
xmessage -geometry 300x100+400+400 app2 & sleep 1.5
new=$(xdotool search --name '^xmessage$' | grep -v "^$app\$" | head -n1)
[[ -n "$(opacity_of "$new")" ]] && ok "new window auto-faded" || fail "new window not faded"

echo "== split"
"$D1" mode split-left >/dev/null 2>&1; sleep 0.5
w=$(xdotool getwindowgeometry "$d1w" | awk '/Geometry/{split($2,a,"x"); print a[1]}')
(( w >= 1500 && w <= 1560 )) && ok "split width ~60% ($w)" || fail "split width $w"
"$D1" mode overlay >/dev/null 2>&1

echo "== reconnect at new resolution"
wait "$S1" 2>/dev/null
send 1920x1080 8 & S2=$!
sleep 5
check "reconnected 1920x1080"   bash -c "'$D1' status | grep -q 'receiving 1920x1080'"
shot=$("$D1" shot --no-clip 2>/dev/null)
[[ "$(png_size "$shot")" == "1920x1080" ]] && ok "second screenshot res" || fail "second screenshot $(png_size "$shot")"
wait "$S2" 2>/dev/null

echo "== wrong passphrase rejected"
if send 640x360 2 wrongpassphrase000 2>/dev/null; then fail "wrong passphrase accepted"; else ok "wrong passphrase rejected"; fi

echo "== stop"
"$D1" stop >/dev/null 2>&1; sleep 0.5
check "viewer stopped"          bash -c "! test -f '$XDG_RUNTIME_DIR/periscope/viewer.pid'"
[[ -z "$(opacity_of "$app")" ]] && ok "opacity restored" || fail "opacity not restored"

echo "== hung viewer (IPC socket that never answers)"
socat "UNIX-LISTEN:$XDG_RUNTIME_DIR/periscope/mpv.sock,fork" SYSTEM:'sleep 10' 2>/dev/null & HUNG=$!
sleep 60 & echo $! > "$XDG_RUNTIME_DIR/periscope/viewer.pid"
sleep 0.5
check "status fails loudly"     bash -c "'$D1' status 2>&1 | grep -q 'did not answer'"
check "stop still cleans up"    bash -c "'$D1' stop && ! test -f '$XDG_RUNTIME_DIR/periscope/viewer.pid'"
kill "$HUNG" 2>/dev/null

kill "$WM" 2>/dev/null
echo
if (( fails == 0 )); then echo "ALL PASSED"; else echo "$fails FAILED (logs: $XDG_RUNTIME_DIR/periscope/mpv.log)"; fi
exit $(( fails > 0 ))
