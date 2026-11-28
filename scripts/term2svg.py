#!/usr/bin/env python3
"""Render a terminal capture (text with ANSI SGR colors) as an SVG "screenshot" for the docs.

Usage: term2svg.py <capture> <out.svg> [title]
Only SGR is interpreted (bold, dim, inverse, 8/256 colors); cursor moves are dropped, and for a
full-screen app (the menu) only the last frame (after the last cursor-home) is kept. Stdlib only.
"""
import html
import re
import sys

PALETTE16 = ["#1d2433", "#ff6b6b", "#5bd97a", "#f5c451", "#5fa8ff", "#c792ea", "#4fd6c8", "#d7dde8"]
BG, FG, DIM = "#0d1b2e", "#d7dde8", "#6c7a93"


def xterm256(n):
    if n < 8:
        return PALETTE16[n]
    if n < 16:
        return PALETTE16[n - 8]
    if n < 232:
        n -= 16
        steps = [0, 95, 135, 175, 215, 255]
        return "#%02x%02x%02x" % (steps[n // 36], steps[(n // 6) % 6], steps[n % 6])
    v = 8 + (n - 232) * 10
    return "#%02x%02x%02x" % (v, v, v)


def parse(text):
    text = text.split("\x1b[H")[-1]  # last frame of a full-screen app
    text = re.sub(r"\x1b\[\?[0-9;]*[a-zA-Z]", "", text)
    text = re.sub(r"\x1b\[[0-9;]*[ABCDEFGJKST]", "", text)
    text = text.replace("\r\n", "\n").replace("\r", "")
    lines, cur = [], []
    state = {"bold": False, "dim": False, "inv": False, "fg": None}
    tokens = re.split(r"(\x1b\[[0-9;]*m)", text)
    for tok in tokens:
        m = re.fullmatch(r"\x1b\[([0-9;]*)m", tok)
        if m:
            codes = [int(c) for c in m.group(1).split(";") if c] or [0]
            i = 0
            while i < len(codes):
                c = codes[i]
                if c == 0:
                    state = {"bold": False, "dim": False, "inv": False, "fg": None}
                elif c == 1:
                    state["bold"] = True
                elif c == 2:
                    state["dim"] = True
                elif c == 7:
                    state["inv"] = True
                elif 30 <= c <= 37:
                    state["fg"] = PALETTE16[c - 30]
                elif c == 38 and i + 2 < len(codes) and codes[i + 1] == 5:
                    state["fg"] = xterm256(codes[i + 2])
                    i += 2
                i += 1
            continue
        for ch in tok:
            if ch == "\n":
                lines.append(cur)
                cur = []
            else:
                cur.append((ch, dict(state)))
    lines.append(cur)
    while lines and not lines[-1]:
        lines.pop()
    return lines


def render(lines, title):
    cw, lh, pad, top = 8.43, 17, 18, 40
    cols = max((len(l) for l in lines), default=0)
    w = int(cols * cw + 2 * pad)
    h = int(len(lines) * lh + top + pad)
    out = [
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" viewBox="0 0 {w} {h}">',
        f'<rect width="{w}" height="{h}" rx="10" fill="{BG}"/>',
        '<circle cx="20" cy="20" r="6" fill="#ff5f57"/><circle cx="40" cy="20" r="6" fill="#febc2e"/>'
        '<circle cx="60" cy="20" r="6" fill="#28c840"/>',
        f'<text x="{w / 2}" y="24" fill="{DIM}" font-family="DejaVu Sans, Helvetica, Arial, sans-serif" '
        f'font-size="12" text-anchor="middle">{html.escape(title)}</text>',
        '<g font-family="DejaVu Sans Mono, Menlo, Consolas, monospace" font-size="14" xml:space="preserve">',
    ]
    for row, line in enumerate(lines):
        y = top + row * lh + 14
        col = 0
        while col < len(line):
            st = line[col][1]
            run = ""
            start = col
            while col < len(line) and line[col][1] == st:
                run += line[col][0]
                col += 1
            fg = st["fg"] or FG
            if st["dim"]:
                fg = DIM if st["fg"] is None else fg
            x = pad + start * cw
            if st["inv"]:
                out.append(f'<rect x="{x:.1f}" y="{y - 13}" width="{len(run) * cw:.1f}" height="{lh}" fill="{fg}"/>')
                fg = BG
            if run.strip():
                weight = ' font-weight="bold"' if st["bold"] else ""
                opacity = ' opacity="0.75"' if st["dim"] and st["fg"] is not None else ""
                out.append(f'<text x="{x:.1f}" y="{y}" fill="{fg}"{weight}{opacity}>{html.escape(run)}</text>')
    out.append("</g></svg>")
    return "\n".join(out)


if __name__ == "__main__":
    src, dst = sys.argv[1], sys.argv[2]
    title = sys.argv[3] if len(sys.argv) > 3 else ""
    with open(src, encoding="utf-8", errors="replace") as f:
        lines = parse(f.read())
    with open(dst, "w", encoding="utf-8") as f:
        f.write(render(lines, title))
