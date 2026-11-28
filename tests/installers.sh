#!/usr/bin/env bash
# Installs the viewer with install/install.sh from locally built release files (as `curl | bash`
# would), checks what it put where, then runs `periscope uninstall --yes` and checks it is all gone.
# Package installs go through a fake sudo. Usage: tests/installers.sh   (needs go, python3, curl)
set -uo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
TMP=$(mktemp -d /tmp/periscope-inst.XXXX)
srv=""
trap '[[ -n $srv ]] && kill "$srv" 2>/dev/null; rm -rf "$TMP"' EXIT

fails=0
ok()   { echo "  ok   $*"; }
fail() { echo "  FAIL $*"; fails=$((fails+1)); }
check() { local d=$1; shift; if "$@" >/dev/null 2>&1; then ok "$d"; else fail "$d"; fi; }

mkdir -p "$TMP/dist" "$TMP/shim" "$TMP/home" "$TMP/run"
chmod 700 "$TMP/run"
(cd "$ROOT" && CGO_ENABLED=0 go build -o "$TMP/dist/periscope_linux_amd64" ./cmd/periscope) || { echo "build failed"; exit 1; }
cp "$ROOT/assets/icon-256.png" "$TMP/dist/periscope.png"
(cd "$TMP/dist" && sha256sum periscope_linux_amd64 > SHA256SUMS)
printf '#!/bin/sh\nexit 0\n' > "$TMP/shim/sudo"; chmod +x "$TMP/shim/sudo"
port=$(( 20000 + RANDOM % 20000 ))
python3 -m http.server "$port" --bind 127.0.0.1 --directory "$TMP/dist" >/dev/null 2>&1 & srv=$!
for _ in $(seq 1 40); do curl -fs "http://127.0.0.1:$port/SHA256SUMS" >/dev/null && break; sleep 0.25; done

export HOME="$TMP/home" XDG_RUNTIME_DIR="$TMP/run" PATH="$TMP/shim:$PATH" PERISCOPE_DOWNLOAD_BASE="http://127.0.0.1:$port"
unset XDG_CONFIG_HOME XDG_DATA_HOME DISPLAY WAYLAND_DISPLAY XDG_CURRENT_DESKTOP PERISCOPE_CONFIG
P="$HOME/.local/bin/periscope"
apps="$HOME/.local/share/applications"

echo "== install (view role, piped like curl | bash)"
bash -s -- --no-start < "$ROOT/install/install.sh" > "$TMP/install.log" 2>&1
check "installer succeeds"              grep -q "Installed. Start with: periscope view" "$TMP/install.log"
check "binary installed"                test -x "$P"
check "config written, private"         bash -c "[[ \$(stat -c %a '$HOME/.config/periscope/config.json') == 600 ]]"
check "launchers installed"             test -f "$apps/periscope-toggle.desktop" -a -f "$apps/periscope-menu.desktop" -a -f "$apps/periscope-uninstall.desktop"
check "icon installed and used"         bash -c "test -f '$HOME/.local/share/icons/hicolor/256x256/apps/periscope.png' && grep -q '^Icon=periscope$' '$apps/periscope-toggle.desktop'"
"$P" status >/dev/null 2>&1   # creates the runtime dir, as any use would

echo "== uninstall"
check "uninstall without a terminal needs --yes" bash -c "! '$P' uninstall < /dev/null"
check "binary still there after refusal" test -x "$P"
"$P" uninstall --yes > "$TMP/uninstall.log" 2>&1
check "uninstall succeeds"              grep -q "periscope is uninstalled" "$TMP/uninstall.log"
check "binary removed"                  test ! -e "$P"
check "config removed"                  test ! -e "$HOME/.config/periscope"
check "launchers and icon removed"      bash -c "! ls '$apps'/periscope-* 2>/dev/null | grep -q . && test ! -e '$HOME/.local/share/icons/hicolor/256x256/apps/periscope.png'"
check "runtime files removed"           test ! -e "$XDG_RUNTIME_DIR/periscope"
check "packages kept, noted"            grep -q "Kept: mpv, ffmpeg" "$TMP/uninstall.log"

echo "== reinstall keeps working; --keep-config keeps the passphrase"
bash "$ROOT/install/install.sh" --no-start >/dev/null 2>&1
pass=$("$P" config get passphrase)
"$P" uninstall --yes --keep-config >/dev/null 2>&1
check "config kept"                     test -f "$HOME/.config/periscope/config.json"
bash "$ROOT/install/install.sh" --no-start >/dev/null 2>&1
[[ "$("$P" config get passphrase)" == "$pass" ]] && ok "same passphrase after reinstall" || fail "passphrase changed"
"$P" uninstall --yes >/dev/null 2>&1

echo
if (( fails == 0 )); then echo "ALL PASSED"; else echo "$fails FAILED (logs: $TMP)"; trap - EXIT; fi
exit $(( fails > 0 ))
