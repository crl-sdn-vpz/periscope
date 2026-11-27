#!/usr/bin/env bash
# End-to-end test of the Go binary on a virtual X display: `periscope view` receives what
# `periscope send` (capture=test, the real argument builder) streams over encrypted SRT.
#
# Requires: go, xvfb-run, openbox, mpv (with srt), ffmpeg (with libsrt), wmctrl, xdotool, xprop,
#           xmessage, xfonts-base, ffprobe
# Usage:    tests/e2e-go-xvfb.sh            (wraps itself in xvfb-run)
set -uo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)

if [[ -z "${PERISCOPE_IN_XVFB:-}" ]]; then
    PERISCOPE_IN_XVFB=1 exec xvfb-run -a -s "-screen 0 2560x1440x24" "$0" "$@"
fi

TMP=$(mktemp -d /tmp/periscope-go-e2e.XXXX)
P="$TMP/periscope"
(cd "$ROOT" && go build -o "$P" ./cmd/periscope) || { echo "build failed"; exit 1; }
export HOME="$TMP/home" XDG_RUNTIME_DIR="$TMP/run" XDG_CONFIG_HOME="$TMP/home/.config" XDG_SESSION_TYPE=x11
mkdir -p "$HOME" "$XDG_RUNTIME_DIR"; chmod 700 "$XDG_RUNTIME_DIR"
STATE="$XDG_RUNTIME_DIR/periscope"
VIEW_CFG="$TMP/view.json" SEND_CFG="$TMP/send.json" BAD_CFG="$TMP/bad.json"
PASS=e2etestpassphrase42

view() { PERISCOPE_CONFIG="$VIEW_CFG" "$P" "$@"; }
send() { PERISCOPE_CONFIG="$SEND_CFG" "$P" "$@"; }

fails=0
ok()   { echo "  ok   $*"; }
fail() { echo "  FAIL $*"; fails=$((fails+1)); }
check() { local d=$1; shift; if "$@" >/dev/null 2>&1; then ok "$d"; else fail "$d"; fi; }
png_size() { ffprobe -v error -select_streams v -show_entries stream=width,height -of csv=p=0:s=x "$1"; }
opacity_of() { xprop -id "$1" _NET_WM_WINDOW_OPACITY 2>/dev/null | awk -F' = ' '/=/{print $2}'; }
wait_for() {  # wait_for <seconds> <command...>: retry until the command succeeds
    local n=$(( $1 * 4 )); shift
    for (( i = 0; i < n; i++ )); do "$@" >/dev/null 2>&1 && return 0; sleep 0.25; done
    return 1
}
receiving() { view status | grep -q "receiving $1"; }
# Anchored on the binary so the patterns can't match a shell whose command line quotes them.
MPV_RE='^mpv srt://'
FF_RE='^[^ ]*ffmpeg .*service_name=periscope'
export MPV_RE FF_RE

openbox >/dev/null 2>&1 & WM=$!
sleep 1

echo "== config"
check "view refuses a missing config"   bash -c "! PERISCOPE_CONFIG='$TMP/none.json' '$P' view"
view config init --passphrase "$PASS" --port 9138 >/dev/null
view config set view.shot_dir "$TMP/shots" >/dev/null
view config set view.overlay_exclude "" >/dev/null
check "config file is private"          bash -c "[[ \$(stat -c %a '$VIEW_CFG') == 600 ]]"
check "short passphrase refused"        bash -c "! PERISCOPE_CONFIG='$VIEW_CFG' '$P' config set passphrase short"
cp "$VIEW_CFG" "$SEND_CFG"
send config init --target-host 127.0.0.1 >/dev/null
send config set send.capture test >/dev/null
check "pair prints both installers"     bash -c "PERISCOPE_CONFIG='$VIEW_CFG' '$P' pair | grep -q 'install.ps1' && PERISCOPE_CONFIG='$VIEW_CFG' '$P' pair | grep -q 'install.sh'"

echo "== viewer"
check "view starts in the background"   view view
check "control socket answers"          bash -c "PERISCOPE_CONFIG='$VIEW_CFG' '$P' status | grep -q 'viewer : running (mode: overlay)'"
check "second view is a no-op"          bash -c "PERISCOPE_CONFIG='$VIEW_CFG' '$P' view | grep -q 'already running'"
check "unknown mode refused"            bash -c "! PERISCOPE_CONFIG='$VIEW_CFG' '$P' mode bogus"
check "waiting before a sender"         bash -c "PERISCOPE_CONFIG='$VIEW_CFG' '$P' status | grep -q 'waiting for the sender'"

echo "== stream 2560x1440 from periscope send"
check "sender detaches"                 send send --detach
wait_for 15 receiving 2560x1440 && ok "receiving 2560x1440" || fail "not receiving: $(view status | sed -n 2p)"
wait_for 10 bash -c "PERISCOPE_CONFIG='$SEND_CFG' '$P' send status | grep -q streaming" && ok "sender reports streaming" || fail "sender status: $(send send status)"
shot=$(view shot --no-clip 2>/dev/null)
check "screenshot written"              test -s "$shot"
[[ "$(png_size "$shot")" == "2560x1440" ]] && ok "screenshot is native res" || fail "screenshot res $(png_size "$shot")"
check "latest.png symlink"              test -L "$TMP/shots/latest.png"

echo "== overlay / opacity"
xmessage -geometry 300x100+50+50 app1 & sleep 1
view mode overlay >/dev/null
app=$(xdotool search --name '^xmessage$' | head -n1)
d1w=$(xdotool search --name '^PERISCOPE-VIEW$' | head -n1)
[[ "$(opacity_of "$app")" == "3435973836" ]] && ok "overlay window at 0.80" || fail "overlay opacity $(opacity_of "$app")"
check "viewer window kept below"        bash -c "xprop -id $d1w _NET_WM_STATE | grep -q BELOW"
[[ "$(view peek)" == "peek on: opacity 0.12" ]] && ok "peek fades" || fail "peek"
[[ "$(view peek)" == "peek off: opacity 0.80" ]] && ok "peek restores" || fail "peek restore"
[[ "$(view opacity cycle)" == "opacity: 0.90" ]] && ok "opacity cycle" || fail "opacity cycle"
view opacity 0.8 >/dev/null
xmessage -geometry 300x100+400+400 app2 & sleep 1.5
new=$(xdotool search --name '^xmessage$' | grep -v "^$app\$" | head -n1)
[[ -n "$(opacity_of "$new")" ]] && ok "new window auto-faded" || fail "new window not faded"

echo "== split"
view mode split-left >/dev/null; sleep 0.5
w=$(xdotool getwindowgeometry "$d1w" | awk '/Geometry/{split($2,a,"x"); print a[1]}')
(( w >= 1500 && w <= 1560 )) && ok "split width ~60% ($w)" || fail "split width $w"
[[ -z "$(opacity_of "$app")" ]] && ok "split restores opacity" || fail "split opacity $(opacity_of "$app")"
check "mode cycle"                      bash -c "PERISCOPE_CONFIG='$VIEW_CFG' '$P' mode cycle | grep -q 'mode: full'"
view mode overlay >/dev/null

echo "== reconnect at a new resolution (max_width scaling)"
check "sender stops"                    send send stop
wait_for 5 bash -c '! pgrep -f "$FF_RE"' && ok "ffmpeg ended with the sender" || fail "ffmpeg still running"
send config set send.max_width 1920 >/dev/null
send send --detach >/dev/null
wait_for 15 receiving 1920x1080 && ok "reconnected 1920x1080" || fail "not 1920x1080: $(view status | sed -n 2p)"
shot=$(view shot --no-clip 2>/dev/null)
[[ "$(png_size "$shot")" == "1920x1080" ]] && ok "second screenshot res" || fail "second screenshot $(png_size "$shot")"
send send stop >/dev/null; wait_for 5 bash -c '! pgrep -f "$FF_RE"'

echo "== mpv crash is recovered"
pkill -f "$MPV_RE"
sleep 0.5
wait_for 10 bash -c "PERISCOPE_CONFIG='$VIEW_CFG' '$P' status | grep -q 'waiting for the sender'" && ok "mpv restarted" || fail "mpv not restarted: $(view status | sed -n 2p)"

echo "== hung mpv"
mpvpid=$(pgrep -f "$MPV_RE" | head -n1)
kill -STOP "$mpvpid"
start=$(date +%s)
out=$(view status 2>&1)
(( $(date +%s) - start <= 6 )) && ok "status returns fast" || fail "status took $(( $(date +%s) - start ))s"
grep -q "not answering" <<<"$out" && ok "status reports a hung mpv" || fail "status: $out"
kill -CONT "$mpvpid"

echo "== wrong passphrase rejected"
cp "$SEND_CFG" "$BAD_CFG"
PERISCOPE_CONFIG="$BAD_CFG" "$P" config set passphrase wrongpassphrase000 >/dev/null
PERISCOPE_CONFIG="$BAD_CFG" "$P" send --detach >/dev/null
sleep 6
view status | grep -q "waiting for the sender" && ok "viewer did not accept it" || fail "viewer: $(view status | sed -n 2p)"
PERISCOPE_CONFIG="$BAD_CFG" "$P" send status | grep -q "waiting for the viewer" && ok "sender keeps retrying" || fail "sender: $(PERISCOPE_CONFIG="$BAD_CFG" "$P" send status)"
PERISCOPE_CONFIG="$BAD_CFG" "$P" send stop >/dev/null

echo "== stop"
check "viewer stops"                    view stop
wait_for 5 bash -c '! pgrep -f "$MPV_RE"' && ok "mpv ended with the viewer" || fail "mpv still running"
[[ -z "$(opacity_of "$app")" ]] && ok "opacity restored" || fail "opacity not restored"
check "status says stopped"             bash -c "PERISCOPE_CONFIG='$VIEW_CFG' '$P' status | grep -q 'viewer : stopped'"

kill "$WM" 2>/dev/null
echo
if (( fails == 0 )); then echo "ALL PASSED"; else echo "$fails FAILED (logs: $STATE)"; fi
exit $(( fails > 0 ))
