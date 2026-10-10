#!/usr/bin/env python3
"""Build resources/icon.png from resources/icon-brush.png.

    python3 resources/icon.py          # needs Pillow and numpy

## What the mark is

A brush stroke wound into a spiral: solid where the quota is spent, coming apart
into hairs and thrown sparks where it is not — 尾, the tail, which is what leeway
means: the length you have not had to spend yet.

Why a spiral and not a quota bar. A quota bar is a line, and an icon that redraws
the app's own progress bar is an icon of a UI component, which is exactly what
Apple's HIG says not to ship ("prefer illustrations to photos and avoid
replicating UI components"). Winding the line up says the half of the product a
bar cannot: quota comes round again, it resets, it cycles. A progress bar is a
line, so this is not a change of subject — it is the same element, wound up.

## Where each half comes from

The brush surface — flying white, stray hairs, the sparks thrown off the tip —
is a render from an image model. Hand-written code draws a stroke that is
*shaped* like a dry brush but never actually *behaves* like one: the streaks sit
in a comb, the hairs in a fan. That texture is what `icon-brush.png` supplies,
and it is the one thing worth taking from a model.

Everything else stays in code, because it is the part a model does not do
reliably: the plate, the squircle, the violet ramp, and above all the placement.
`icon-brush.png` is white ink on flat black, which is what makes luminance a
usable mask.

## Placement: solved, not measured

The tempting approach is to measure some "centre" of the mark and move that to
the middle of the canvas. It does not work, and each definition fails its own
way. The **bounding box** centre tracks the mark's reach, so one hair trailing to
a corner drags the body away from the middle. The **centre of gravity** tracks
the mark's weight, and a long faint tail carries enough weight to pull the body
off the other way. The **eye of the spiral** is not a hole at all: a spiral is
open, so flooding in from the border runs straight through it.

What the eye reads is the **reach** — a mark whose sparks sweep far to the upper
right looks off-centre even when its weight is dead centre. So the centre is not
measured, it is solved for: over every candidate position, find the worst pixel,
the one that reaches furthest once the shape of the icon body is accounted for,
and choose the position that makes that worst pixel as close as possible. That is
the Chebyshev centre of the mark in the squircle's own norm, and it balances the
reach and maximises the size in the same step — which is why the scale falls out
of the same number instead of being guessed separately.

## Geometry

macOS: a 1024 canvas with an 824 superellipse body centred in it, corner
exponent 5 (Apple's squircle, not a rounded rectangle), transparent margins.
MyGo's `pngToICNS` only resizes — it does no masking — so the squircle has to be
baked in here.

Everything runs at 4x and is scaled down, which is what keeps the edges clean.
"""

import math
import pathlib

import numpy as np
from PIL import Image, ImageDraw

HERE = pathlib.Path(__file__).parent
SOURCE = HERE / "icon-brush.png"
TARGET = HERE / "icon.png"

S = 4096  # working canvas, 4x the 1024 the bundle wants
CANVAS = 1024
BODY = round(824 / 1024 * S)
N = 5.0  # superellipse exponent
VIOLET_TOP = (124, 92, 255)
VIOLET_BOTTOM = (74, 34, 217)
WHITE = (255, 255, 255)

# Thresholds for turning the render's luminance into an ink mask.
#
# The floor sits above the model's JPEG ringing: set it too low and the faint
# halo of compression artefacts around the mark is lifted along with the ink,
# which makes the result look hazy and scattered rather than drawn.
INK_FLOOR = 0.30
INK_CEIL = 0.85
# Ink fainter than this does not count when solving the placement. Where the
# brush ran out, the hair tip fades to almost nothing; shrinking the whole mark
# so that an invisible tip stays inside the shape costs far more than letting it
# fade out at the edge.
VISIBLE = 40
# How much of the icon body the mark may fill.
FILL = 0.90
# A high percentile rather than the strict maximum, so one stray speck cannot
# decide the layout for everything else. The few pixels this lets overhang are
# the faintest ones, and the clip below trims them.
ROBUST = 99.5


def superellipse(canvas, body, n, steps=8192):
    """A centred squircle mask, as Apple draws an icon body."""
    a, c = body / 2, canvas / 2
    pts = []
    for i in range(steps):
        t = 2 * math.pi * i / steps
        ct, st = math.cos(t), math.sin(t)
        pts.append((c + a * math.copysign(abs(ct) ** (2 / n), ct),
                    c + a * math.copysign(abs(st) ** (2 / n), st)))
    mask = Image.new("L", (canvas, canvas), 0)
    ImageDraw.Draw(mask).polygon(pts, fill=255)
    return mask


def vertical_gradient(size, top, bottom):
    grad = Image.new("RGB", (1, size))
    px = grad.load()
    for y in range(size):
        f = y / (size - 1)
        px[0, y] = tuple(round(top[i] + (bottom[i] - top[i]) * f) for i in range(3))
    return grad.resize((size, size), Image.BILINEAR)


def plate():
    """The icon body: one clean vertical ramp, no glow and no bevel."""
    img = vertical_gradient(S, VIOLET_TOP, VIOLET_BOTTOM).convert("RGBA")
    out = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    out.paste(img, (0, 0), superellipse(S, BODY, N))
    return out


def squircle_radius(angles):
    """How far the icon body reaches at a given polar angle.

    The body is a superellipse |x/a|^5 + |y/a|^5 = 1. Parametrised as
    x = a|cos t|^0.4, y = a|sin t|^0.4, the polar angle satisfies
    tan(theta) = |tan t|^0.4, so inverting gives |tan t| = |tan(theta)|^2.5.
    Getting that exponent wrong is easy and quiet: it is exact on the axes and at
    45 degrees, where the numbers are memorable, and wrong by tens of pixels
    everywhere between them.
    """
    a = BODY / 2 / S * CANVAS
    t = np.arctan(np.abs(np.tan(angles)) ** 2.5)
    return np.hypot(a * np.abs(np.cos(t)) ** 0.4, a * np.abs(np.sin(t)) ** 0.4)


def ink_ramp(lum):
    """Luminance to ink, as a ramp rather than a cut.

    Deliberately soft. Where the brush ran dry there is genuinely less ink, and
    letting that through as thinner paint is what a dry brush looks like; a hard
    threshold would delete the flying white, which is the whole reason to use a
    render here at all.
    """
    card = float(np.median(np.concatenate([lum[0], lum[-1], lum[:, 0], lum[:, -1]])))
    span = max(1e-6, INK_CEIL - INK_FLOOR)
    return np.clip((lum - card - INK_FLOOR) / span * 255.0, 0, 255)


def worst_reach(cx, cy, xs, ys):
    """The furthest any visible pixel reaches, in units of the icon body.

    Measuring against the body's own radius rather than plain distance is what
    lets the flat sides of the squircle take the ink that the corners cannot.
    """
    d = np.maximum(np.hypot(xs - cx, ys - cy), 1e-6)
    ang = np.arctan2(ys - cy, xs - cx)
    return float(np.percentile(d / squircle_radius(ang), ROBUST))


def solve_placement(xs, ys):
    """The position that best balances how far the mark reaches.

    The objective is a maximum over an unknown pixel, so it is not smooth and
    there is no gradient worth following. A grid that shrinks cannot get stuck
    the way a descent can.
    """
    keep = max(1, len(xs) // 20000)
    sx, sy = xs[::keep], ys[::keep]

    cx, cy = float(xs.mean()), float(ys.mean())
    best = worst_reach(cx, cy, sx, sy)
    for step in (12.0, 3.0, 1.0, 0.25):
        while True:
            improved = False
            for ax in (cx - step, cx, cx + step):
                for ay in (cy - step, cy, cy + step):
                    v = worst_reach(ax, ay, sx, sy)
                    if v < best - 1e-9:
                        best, cx, cy, improved = v, ax, ay, True
            if not improved:
                break
    return cx, cy, FILL / best


def mark():
    """The ink, already placed: a 1024 alpha mask the size of the canvas."""
    rgb = np.asarray(Image.open(SOURCE).convert("RGB"), dtype=np.float64)
    lum = (0.2126 * rgb[:, :, 0] + 0.7152 * rgb[:, :, 1] + 0.0722 * rgb[:, :, 2]) / 255.0
    ramp = ink_ramp(lum)

    ys, xs = np.nonzero(ramp >= VISIBLE)
    xs, ys = xs.astype(np.float64), ys.astype(np.float64)
    cx, cy, scale = solve_placement(xs, ys)

    h, w = ramp.shape
    placed = Image.new("L", (CANVAS, CANVAS), 0)
    scaled = Image.fromarray(ramp.astype(np.uint8), "L").resize(
        (max(1, round(w * scale)), max(1, round(h * scale))), Image.LANCZOS)
    placed.paste(scaled,
                 (round(CANVAS / 2 - cx * scale), round(CANVAS / 2 - cy * scale)))

    # Clip to the body. The placement solve lets the faintest few pixels
    # overhang its own threshold, and alpha_composite keeps the ink's own alpha
    # wherever the plate is transparent — so without this, stray hairs land as
    # white specks outside the rounded body.
    return Image.composite(placed, Image.new("L", (CANVAS, CANVAS), 0),
                           superellipse(CANVAS, round(BODY / S * CANVAS), N))


def main():
    ink = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    ink.paste(WHITE, (0, 0), mark().resize((S, S), Image.LANCZOS))
    art = plate()
    art.alpha_composite(ink)
    art.resize((CANVAS, CANVAS), Image.LANCZOS).save(TARGET)


if __name__ == "__main__":
    main()
