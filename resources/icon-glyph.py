#!/usr/bin/env python3
"""Emit the titlebar mark — the SVG path in internal/gui/icons.go.

    python3 resources/icon-glyph.py            # the chosen 0.86 / 3 / 0.95
    python3 resources/icon-glyph.py 0.92 3.6 1.10    # a heavier one

Args: how much of the 24-unit box the mark fills, stroke width, turns.

## Why this is not just resources/icon.png scaled down

The window draws the app's mark beside the wordmark at **16px**, smaller than the
icon ever gets in the Dock, and a spiral loses its shape from the inside out as it
shrinks. What survives at that size is the *opening* in the middle, so the glyph
turns 0.95 times where the icon turns 1.10 — fewer turns, a bigger hole. The
icon's whole spread of trailing hairs becomes noise at 16px and is cut to one
short flick; the flick stays because without it the mark is just a "6", and the
tail is the half of the name that means anything.

Size and weight were not guessed. They were picked by rendering candidates
through MyGo's own rasteriser and looking at them: a preview rendered by anything
else — PIL, a browser — disagrees about downsampling, antialiasing and placement,
and it disagreed enough here to pick the wrong numbers. **Judge a glyph in the
engine that will draw it.**

## Output

The path is a Catmull-Rom spline through the spiral turned into cubic Béziers, so
it is a handful of curves rather than a few hundred `L` points. The mark is
centred in its box and the script refuses to emit a path that would be clipped by
the viewBox — an earlier version quietly multiplied the hairs by the radius scale
and produced a mark 24.6 units tall in a 24-unit box.
"""

import math
import sys

# The spiral resources/icon.py winds, in its own 1024-space units.
R0, R1, START = 102, 307, -math.pi / 2

VB = 24.0  # the viewBox the other icons use
SEG = 12  # cubic segments; enough that no facet shows at 16px


def build(fill, stroke, turns):
    half = stroke / 2
    # Radius scale so that the outer reach plus half the stroke spans `fill`.
    rs = (VB * fill / 2 - half) / R1

    def raw(t):
        r = (R0 + (R1 - R0) * t) * rs
        a = START + 2 * math.pi * turns * t
        return (r * math.cos(a), r * math.sin(a))

    P = [raw(i / (SEG * 4)) for i in range(SEG * 4 + 1)]
    knots = [P[i * 4] for i in range(SEG + 1)]

    # The flick: hairs off the outer end, along the direction of travel, splayed
    # either side of it.
    r, a = R1 * rs, START + 2 * math.pi * turns
    k = (R1 - R0) / (2 * math.pi * turns) * rs  # dr/da, the tangent
    vx, vy = k * math.cos(a) - r * math.sin(a), k * math.sin(a) + r * math.cos(a)
    nrm = math.hypot(vx, vy)
    vx, vy = vx / nrm, vy / nrm
    x0, y0 = raw(1.0)
    hairs = []
    for splay, ln in ((0.42, 1.9), (-0.30, 1.25)):
        b = a + math.pi / 2 * splay
        hairs.append(((x0, y0), (x0 + (vx + math.cos(b) * 0.9) * ln,
                                y0 + (vy + math.sin(b) * 0.9) * ln)))

    # Centre the whole mark, stroke included, in the box.
    xs = [p[0] for p in P] + [h[1][0] for h in hairs]
    ys = [p[1] for p in P] + [h[1][1] for h in hairs]
    dx = VB / 2 - (min(xs) + max(xs)) / 2
    dy = VB / 2 - (min(ys) + max(ys)) / 2
    knots = [(x + dx, y + dy) for x, y in knots]
    hairs = [((a0 + dx, b0 + dy), (a1 + dx, b1 + dy)) for (a0, b0), (a1, b1) in hairs]

    def f(v):
        return f"{v:.2f}".rstrip("0").rstrip(".")

    d = [f"M{f(knots[0][0])} {f(knots[0][1])}"]
    for i in range(SEG):
        p0 = knots[i - 1] if i else knots[0]
        p1, p2 = knots[i], knots[i + 1]
        p3 = knots[i + 2] if i + 2 < len(knots) else knots[-1]
        c1 = (p1[0] + (p2[0] - p0[0]) / 6, p1[1] + (p2[1] - p0[1]) / 6)
        c2 = (p2[0] - (p3[0] - p1[0]) / 6, p2[1] - (p3[1] - p1[1]) / 6)
        d.append(f"C{f(c1[0])} {f(c1[1])} {f(c2[0])} {f(c2[1])} {f(p2[0])} {f(p2[1])}")
    for (a0, b0), (a1, b1) in hairs:
        d.append(f"M{f(a0)} {f(b0)}L{f(a1)} {f(b1)}")

    lo_x, hi_x = min(xs) + dx - half, max(xs) + dx + half
    lo_y, hi_y = min(ys) + dy - half, max(ys) + dy + half
    if lo_x < 0 or lo_y < 0 or hi_x > VB or hi_y > VB:
        raise SystemExit(f"glyph leaves the {VB:g} box: "
                         f"x {lo_x:.2f}..{hi_x:.2f} y {lo_y:.2f}..{hi_y:.2f}")

    return ('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" '
            f'stroke="currentColor" stroke-width="{stroke}" stroke-linecap="round" '
            f'stroke-linejoin="round"><path d="{"".join(d)}"/></svg>')


if __name__ == "__main__":
    fill = float(sys.argv[1]) if len(sys.argv) > 1 else 0.86
    stroke = float(sys.argv[2]) if len(sys.argv) > 2 else 3.0
    turns = float(sys.argv[3]) if len(sys.argv) > 3 else 0.95
    print(build(fill, stroke, turns))
