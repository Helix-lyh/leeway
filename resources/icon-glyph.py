#!/usr/bin/env python3
"""Emit the titlebar mark — the SVG path in internal/gui/icons.go.

    python3 resources/icon-glyph.py            # writes the path to stdout

## Why this traces the icon instead of drawing a spiral

The first version of this file drew its own spiral: the same start angle, a
chosen turn count, a chosen stroke weight. It was fitted by eye against the icon
and it never looked like it. At titlebar size the icon reads as a **solid
pinwheel with one slot cut through it**, and the hand-drawn glyph read as a thin
outlined "6" — because the number that decides which of those you see is how
thick the stroke is against the gap between turns, and picking that by eye
alongside two other knobs is how you end up two ratios away from the artwork.

So this does not draw anything. It takes `resources/icon.png`, keeps the ink,
and traces its outline — **the titlebar mark is the icon's own silhouette**, and
cannot drift from it. Change the icon and re-run this and the two agree again.

## The one judgement left: at what size to trace

Tracing at 16px gives an octagon. Tracing at 128px picks up every flying-white
hair, which at 16px is mud. 64px is where the mass is still whole and the tail is
still one stroke, so that is the default — the shape you get is what the icon
looks like at the size the titlebar actually draws it.

## Why a bitmap would not do

The obvious shortcut — put `icon.png` in the titlebar — puts a violet squircle
tile beside the wordmark, which is a second app icon, not a mark. A monochrome
bitmap would need its colour baked in, and MyGo's `ui.Image` cannot tint, so it
would take one file per theme. A path drawn with `fill="currentColor"` follows
the theme for free, and stays crisp at any size.
"""

import math
import pathlib
import sys

import numpy as np
from PIL import Image

HERE = pathlib.Path(__file__).resolve().parent
ICON = HERE / "icon.png"

VB = 24.0  # the viewBox the other icons use
TRACE_PX = 64  # where the mass is whole and the tail is still one stroke
EPS = 0.9  # Douglas-Peucker tolerance, in traced pixels
MIN_AREA = 1.5  # drop specks that are not part of the mark
FILL = 0.86  # how much of the viewBox the mark spans


def ink_mask(px):
    """The icon's ink as a boolean mask, at the size the titlebar draws it.

    Ink is white on the violet plate, so this measures how far a pixel is from
    the plate rather than how bright it is — otherwise the plate's own vertical
    ramp leaks into the mask at its light end.
    """
    a = np.asarray(Image.open(ICON).convert("RGBA").resize((px, px), Image.LANCZOS),
                   dtype=np.float64)
    alpha = a[:, :, 3] / 255.0
    lum = (0.2126 * a[:, :, 0] + 0.7152 * a[:, :, 1] + 0.0722 * a[:, :, 2]) / 255.0
    return (alpha > 0.5) & (lum > 0.55)


def _segments():
    """The marching-squares table, derived rather than typed out.

    A cell's four edges are crossed wherever their two corner bits differ; two
    crossings join into one segment, four (the saddles) into two. Writing this
    table by hand is how the first version of this file got it wrong.
    """
    edges = {(1, 2): (0.5, 0), (2, 4): (1, 0.5), (4, 8): (0.5, 1), (8, 1): (0, 0.5)}
    table = {}
    for code in range(16):
        crossed = [mid for (a, b), mid in edges.items()
                   if bool(code & a) != bool(code & b)]
        if len(crossed) == 2:
            table[code] = [(crossed[0], crossed[1])]
        elif len(crossed) == 4:
            # Saddle: pair the crossings so the two arcs do not cross, keeping
            # the set corners on one side.
            top, right, bottom, left = (0.5, 0), (1, 0.5), (0.5, 1), (0, 0.5)
            table[code] = ([(top, left), (right, bottom)] if code == 5
                           else [(top, right), (bottom, left)])
        else:
            table[code] = []
    return table


SEGMENTS = _segments()


def contours(mask):
    """Closed contours of a boolean mask, in pixel units."""
    h, w = mask.shape
    padded = np.zeros((h + 2, w + 2), dtype=bool)
    padded[1:-1, 1:-1] = mask

    edges = {}
    for y in range(h + 1):
        for x in range(w + 1):
            code = (int(padded[y, x]) | int(padded[y, x + 1]) << 1
                    | int(padded[y + 1, x + 1]) << 2 | int(padded[y + 1, x]) << 3)
            for (ax, ay), (bx, by) in SEGMENTS[code]:
                a, b = (x + ax - 1, y + ay - 1), (x + bx - 1, y + by - 1)
                edges.setdefault(a, []).append(b)
                edges.setdefault(b, []).append(a)

    out = []
    while edges:
        start = next(iter(edges))
        if not edges[start]:
            del edges[start]
            continue
        loop, cur, prev = [], start, None
        while True:
            loop.append(cur)
            nxt = [p for p in edges.get(cur, []) if p != prev]
            if not nxt:
                break
            step = nxt[0]
            edges[cur].remove(step)
            edges[step].remove(cur)
            if not edges[cur]:
                del edges[cur]
            prev, cur = cur, step
            if cur == start:
                break
        # Every pass consumes at least `start`, so the scan always terminates.
        edges.pop(start, None)
        if len(loop) > 3:
            out.append(loop)
    return out


def simplify(points, eps):
    """Douglas-Peucker. Run on an open polyline: the contour already ends next to
    where it starts, and closing it first makes the baseline zero-length, which
    collapses the whole thing to two points."""
    if len(points) < 3:
        return points
    (x1, y1), (x2, y2) = points[0], points[-1]
    dx, dy = x2 - x1, y2 - y1
    norm = math.hypot(dx, dy) or 1e-9
    worst, idx = 0.0, 0
    for i, (x, y) in enumerate(points[1:-1], 1):
        if abs(dy * (x - x1) - dx * (y - y1)) / norm > worst:
            worst, idx = abs(dy * (x - x1) - dx * (y - y1)) / norm, i
    if worst <= eps:
        return [points[0], points[-1]]
    return simplify(points[:idx + 1], eps)[:-1] + simplify(points[idx:], eps)


def area(loop):
    s = 0.0
    for i in range(len(loop)):
        x1, y1 = loop[i]
        x2, y2 = loop[(i + 1) % len(loop)]
        s += x1 * y2 - x2 * y1
    return abs(s) / 2


def build(px=TRACE_PX, eps=EPS, fill=FILL):
    loops = [l for l in (simplify(c, eps) for c in contours(ink_mask(px)))
             if len(l) > 3 and area(l) >= MIN_AREA]
    if not loops:
        raise SystemExit(f"no ink found in {ICON} at {px}px")

    # Fit the traced outline into `fill` of the viewBox and centre it.
    xs = [p[0] for l in loops for p in l]
    ys = [p[1] for l in loops for p in l]
    scale = VB * fill / max(max(xs) - min(xs), max(ys) - min(ys))
    cx, cy = (min(xs) + max(xs)) / 2, (min(ys) + max(ys)) / 2

    def f(v):
        return f"{v:.2f}".rstrip("0").rstrip(".")

    d = []
    for loop in loops:
        pts = [(VB / 2 + (x - cx) * scale, VB / 2 + (y - cy) * scale) for x, y in loop]
        d.append(f"M{f(pts[0][0])} {f(pts[0][1])}")
        d += [f"L{f(x)} {f(y)}" for x, y in pts[1:]]
        d.append("Z")

    return ('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" '
            f'fill="currentColor" fill-rule="evenodd"><path d="{"".join(d)}"/></svg>')


if __name__ == "__main__":
    print(build())
