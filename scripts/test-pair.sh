#!/usr/bin/env bash
# Hardware test of an unreleased build: run on the Linux viewer from a checkout.
# Builds the release files, serves them on the LAN, installs and starts the viewer with the real
# installer, then prints the one command to paste into PowerShell on the Windows sender.
# Keeps serving until Ctrl+C (the Windows install downloads from here).
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
HTTP_PORT=${1:-8000}
cd "$ROOT"

echo "==> Building release files"
if ! out=$(just dist 2>&1); then echo "$out"; exit 1; fi
cp install/install.sh install/install.ps1 dist/

ip=$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for (i = 1; i < NF; i++) if ($i == "src") print $(i + 1)}')
[[ -n $ip ]] || { echo "could not find this machine's LAN IP"; exit 1; }

python3 -m http.server "$HTTP_PORT" --bind 0.0.0.0 --directory dist >/dev/null 2>&1 &
srv=$!
trap 'kill "$srv" 2>/dev/null' EXIT
for _ in $(seq 1 40); do curl -fs "http://127.0.0.1:$HTTP_PORT/SHA256SUMS" >/dev/null && break; sleep 0.25; done

echo "==> Installing the viewer with install/install.sh"
PERISCOPE_DOWNLOAD_BASE="http://127.0.0.1:$HTTP_PORT" bash install/install.sh --role view --no-start
p="$HOME/.local/bin/periscope"
"$p" view
port=$("$p" config get port)
pass=$("$p" config get passphrase)

if command -v ufw >/dev/null && sudo ufw status 2>/dev/null | grep -q "Status: active"; then
    echo "==> ufw is active: allowing TCP $HTTP_PORT for the download (remove later: sudo ufw delete allow $HTTP_PORT/tcp)"
    sudo ufw allow "$HTTP_PORT/tcp" >/dev/null
fi

cat <<MSG

============================================================================================
On the Windows PC, open PowerShell (normal user, not admin) and paste this ONE line:

\$env:PERISCOPE_DOWNLOAD_BASE='http://$ip:$HTTP_PORT'; & ([scriptblock]::Create((irm http://$ip:$HTTP_PORT/install.ps1))) -Role send -TargetHost $ip -Port $port -Passphrase '$pass'

Within ~10 s this screen shows the Windows desktop. Keep this terminal open until the Windows
install says "Done", then press Ctrl+C here (the viewer keeps running).
============================================================================================
MSG
wait "$srv"
