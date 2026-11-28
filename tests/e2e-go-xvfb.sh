#!/usr/bin/env bash
# End-to-end test of the Go binary on a virtual X display: `periscope view` receives what
# `periscope send` (capture=test, the real argument builder) streams over encrypted SRT.
#
# Requires: go, xvfb-run, openbox, mpv (with srt), ffmpeg (with libsrt), wmctrl, xdotool, xprop,
#           xmessage, xfonts-base, ffprobe, xclip
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

# Xvfb has one monitor and ignores `xrandr --setmonitor`, so a shim reports two side by side
# (left = 0, primary; right = 1). Everything else goes to the real xrandr.
mkdir -p "$TMP/shim"
cat > "$TMP/shim/xrandr" <<'SHIM'
#!/usr/bin/env bash
if [[ "${1:-}" == --listactivemonitors ]]; then
    printf 'Monitors: 2\n 0: +*left 1280/339x1440/381+0+0  left\n 1: +right 1280/339x1440/381+1280+0  right\n'
    exit 0
fi
exec /usr/bin/xrandr "$@"
SHIM
chmod +x "$TMP/shim/xrandr"
export PATH="$TMP/shim:$PATH"
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
check "shot to clipboard returns"       timeout 5 env PERISCOPE_CONFIG="$VIEW_CFG" "$P" shot
check "clipboard holds the PNG"         bash -c "xclip -selection clipboard -t TARGETS -o | grep -q image/png"
check "shot --path returns"             timeout 5 env PERISCOPE_CONFIG="$VIEW_CFG" "$P" shot --path
[[ "$(png_size "$shot")" == "2560x1440" ]] && ok "screenshot is native res" || fail "screenshot res $(png_size "$shot")"
check "latest.png symlink"              test -L "$TMP/shots/latest.png"

echo "== overlay / opacity (feed on monitor 0 = left half)"
xmessage -geometry 300x100+50+50 app1 & sleep 1
xmessage -geometry 300x100+1500+100 right1 & sleep 1
view mode overlay >/dev/null
app=$(xdotool search --name '^xmessage$' | head -n1)
right=$(xdotool search --name '^xmessage$' | grep -v "^$app\$" | head -n1)
d1w=$(xdotool search --name '^PERISCOPE-VIEW$' | head -n1)
[[ "$(opacity_of "$app")" == "3435973836" ]] && ok "overlay window at 0.80" || fail "overlay opacity $(opacity_of "$app")"
[[ -z "$(opacity_of "$right")" ]] && ok "window on the other monitor not faded" || fail "right window faded $(opacity_of "$right")"
check "viewer window kept below"        bash -c "xprop -id $d1w _NET_WM_STATE | grep -q BELOW"
[[ "$(view peek)" == "peek on: opacity 0.12, 1 window(s) at 0.12" ]] && ok "peek fades and counts" || fail "peek: $(view opacity)"
[[ "$(opacity_of "$app")" == "$(( 0xffffffff * 12 / 100 ))" ]] && ok "peek opacity applied" || fail "peek opacity $(opacity_of "$app")"
[[ "$(view peek)" == "peek off: opacity 0.80, 1 window(s) at 0.80" ]] && ok "peek restores" || fail "peek restore"
[[ "$(view opacity cycle)" == opacity:\ 0.90* ]] && ok "opacity cycle" || fail "opacity cycle"
view opacity 0.8 >/dev/null
xmessage -geometry 300x100+400+400 app2 & sleep 1.5
new=$(xdotool search --name '^xmessage$' | grep -v -e "^$app\$" -e "^$right\$" | head -n1)
[[ -n "$(opacity_of "$new")" ]] && ok "new window auto-faded" || fail "new window not faded"
# GNOME puts every window on a non-primary monitor on all desktops (_NET_WM_DESKTOP = -1).
xprop -id "$new" -f _NET_WM_DESKTOP 32c -set _NET_WM_DESKTOP 4294967295; sleep 1.5
[[ "$(wmctrl -l | awk -v id="$(printf '0x%08x' "$new")" '$1 == id {print $2}')" == -1 ]] && \
    [[ -n "$(opacity_of "$new")" ]] && ok "all-desktops window on the feed is faded" || fail "all-desktops window: $(opacity_of "$new")"
check "windows explains each window"    bash -c "PERISCOPE_CONFIG='$VIEW_CFG' '$P' windows | grep -q 'faded to 0.80' && PERISCOPE_CONFIG='$VIEW_CFG' '$P' windows | grep -q 'solid: centered on another monitor'"
xdotool windowmove "$new" 1600 600; sleep 2
[[ -z "$(opacity_of "$new")" ]] && ok "window dragged off the feed un-fades" || fail "moved window still $(opacity_of "$new")"
xdotool windowmove "$new" 400 400; sleep 2
[[ -n "$(opacity_of "$new")" ]] && ok "window dragged back fades" || fail "moved-back window not faded"

echo "== monitors"
check "screens lists both"              bash -c "PERISCOPE_CONFIG='$VIEW_CFG' '$P' screens | grep -q '1: right'"
check "screens marks the feed"          bash -c "PERISCOPE_CONFIG='$VIEW_CFG' '$P' screens | grep -q 'left.*<- feed'"
check "screen 1 moves the feed"         bash -c "PERISCOPE_CONFIG='$VIEW_CFG' '$P' screen 1 | grep -q 'on monitor 1 (right)'"
[[ "$(opacity_of "$right")" == "3435973836" && -z "$(opacity_of "$app")" ]] && ok "fading follows the feed" \
    || fail "after screen 1: right=$(opacity_of "$right") left=$(opacity_of "$app")"
check "status shows the monitor"        bash -c "PERISCOPE_CONFIG='$VIEW_CFG' '$P' status | grep -q 'screen : 1: right'"
check "screen next wraps to 0"          bash -c "PERISCOPE_CONFIG='$VIEW_CFG' '$P' screen next | grep -q 'on monitor 0 (left)'"
check "bad monitor refused"             bash -c "! PERISCOPE_CONFIG='$VIEW_CFG' '$P' screen 5"
view screen 1 --default >/dev/null
[[ "$(view config get view.screen)" == 1 ]] && ok "screen --default saves it" || fail "view.screen = $(view config get view.screen)"
view screen 0 --default >/dev/null

echo "== split"
view mode split-left >/dev/null; sleep 0.5
geo() { xdotool getwindowgeometry "$d1w" | awk '/Position/{split($2,p,","); x=p[1]} /Geometry/{split($2,a,"x"); w=a[1]} END{print x, w}'; }
read -r x w < <(geo)
(( x >= 0 && x <= 10 && w >= 740 && w <= 790 )) && ok "split-left: 60% of monitor 0 ($x,$w)" || fail "split-left geometry $x,$w"
[[ -z "$(opacity_of "$app")" ]] && ok "split restores opacity" || fail "split opacity $(opacity_of "$app")"
check "opacity refused outside overlay" bash -c "! PERISCOPE_CONFIG='$VIEW_CFG' '$P' peek"
view screen 1 >/dev/null; view mode split-right >/dev/null; sleep 0.5
read -r x w < <(geo)
(( x >= 1770 && x <= 1810 && w >= 740 && w <= 790 )) && ok "split-right on monitor 1 ($x,$w)" || fail "split-right geometry $x,$w"
view screen 0 >/dev/null
check "mode cycle"                      bash -c "PERISCOPE_CONFIG='$VIEW_CFG' '$P' mode cycle | grep -q 'mode: overlay'"
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

echo "== send toggle"
check "toggle starts the sender"        bash -c "PERISCOPE_CONFIG='$SEND_CFG' '$P' send toggle && PERISCOPE_CONFIG='$SEND_CFG' '$P' send status"
check "toggle stops it"                 bash -c "PERISCOPE_CONFIG='$SEND_CFG' '$P' send toggle; sleep 1; ! PERISCOPE_CONFIG='$SEND_CFG' '$P' send status"

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
[[ "$(opacity_of "$app")" == "3435973836" ]] || view mode overlay >/dev/null
check "stop restores before replying"   bash -c "PERISCOPE_CONFIG='$VIEW_CFG' '$P' stop | grep -q 'back to solid'"
wait_for 5 bash -c '! pgrep -f "$MPV_RE"' && ok "mpv ended with the viewer" || fail "mpv still running"
[[ -z "$(opacity_of "$app")" ]] && ok "opacity restored" || fail "opacity not restored"
check "status says stopped"             bash -c "PERISCOPE_CONFIG='$VIEW_CFG' '$P' status | grep -q 'viewer : stopped'"

echo "== crashed viewer: stop still cleans up"
view view overlay >/dev/null; sleep 1
[[ -n "$(opacity_of "$app")" ]] || view mode overlay >/dev/null
pkill -9 -f '^[^ ]*periscope view --foreground'
wait_for 5 bash -c '! pgrep -f "$MPV_RE"'
[[ -n "$(opacity_of "$app")" ]] && ok "window left faded by the crash" || fail "crash setup: window not faded"
check "stop cleans up without a viewer" bash -c "PERISCOPE_CONFIG='$VIEW_CFG' '$P' stop | grep -q 'was not running; .* back to solid'"
[[ -z "$(opacity_of "$app")" ]] && ok "opacity restored after a crash" || fail "still $(opacity_of "$app")"

echo "== toggle"
check "toggle starts the viewer"        bash -c "PERISCOPE_CONFIG='$VIEW_CFG' '$P' toggle && PERISCOPE_CONFIG='$VIEW_CFG' '$P' status | grep -q 'viewer : running'"
check "toggle stops it"                 bash -c "PERISCOPE_CONFIG='$VIEW_CFG' '$P' toggle && PERISCOPE_CONFIG='$VIEW_CFG' '$P' status | grep -q 'viewer : stopped'"

kill "$WM" 2>/dev/null
echo
if (( fails == 0 )); then echo "ALL PASSED"; else echo "$fails FAILED (logs: $STATE)"; fi
exit $(( fails > 0 ))
