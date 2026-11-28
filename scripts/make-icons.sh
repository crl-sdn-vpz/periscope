#!/usr/bin/env bash
# Renders assets/icon.svg to the raster icons the installers use and README shows:
#   assets/icon-256.png (Linux launchers), assets/periscope.ico (Windows shortcuts: 16-256 px).
# Needs ffmpeg built with librsvg (Ubuntu's is) and python3 (stdlib only). Run: just icons
set -euo pipefail
cd "$(dirname "$0")/.."
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
for s in 16 24 32 48 64 128 256; do
    ffmpeg -hide_banner -loglevel error -y -i assets/icon.svg -vf "scale=${s}:${s}:flags=lanczos" \
        -frames:v 1 "$tmp/$s.png"
done
cp "$tmp/256.png" assets/icon-256.png
# ICO with PNG-compressed entries (supported since Windows Vista).
python3 - "$tmp" assets/periscope.ico <<'PY'
import struct, sys, pathlib
src, out = pathlib.Path(sys.argv[1]), sys.argv[2]
sizes = [16, 24, 32, 48, 64, 128, 256]
pngs = [(src / f"{s}.png").read_bytes() for s in sizes]
offset = 6 + 16 * len(sizes)
header = struct.pack("<HHH", 0, 1, len(sizes))
entries, data = b"", b""
for s, png in zip(sizes, pngs):
    dim = 0 if s == 256 else s  # 0 means 256 in ICO
    entries += struct.pack("<BBBBHHII", dim, dim, 0, 0, 1, 32, len(png), offset + len(data))
    data += png
pathlib.Path(out).write_bytes(header + entries + data)
PY
echo "icons: assets/icon-256.png assets/periscope.ico"
